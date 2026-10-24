// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"
)

const claimCandidateRetryDelay = 30 * time.Second
const (
	submitCandidatePage = iota
	acceptCandidatePage
	repairCandidatePage
	candidatePageCount
)

// The publisher owns its traversal and short-lived candidate window. It stores
// no copy of the sender's current request or historical transaction outcomes.
type claimPublisher struct {
	repository                                            iclaimerRepository
	lookup                                                claimLookup
	backend                                               txsender.Backend
	sender                                                *txsender.Sender[claimAction]
	signer                                                common.Address
	observationPolicy                                     model.DefaultBlock
	Logger                                                *slog.Logger
	interval                                              time.Duration
	cooldowns                                             map[claimAction]time.Time
	stagingWarnings                                       map[claimAction]time.Time
	pages                                                 [candidatePageCount]repository.ClaimCursor
	failure                                               *atomic.Pointer[error]
	candidates                                            []claimCandidate
	prepared                                              bool
	lastChoice                                            claimAction
	lastPending                                           claimAction
	lastLive                                              txsender.BlockRef
	lastNonce                                             uint64
	hadPending                                            bool
	headChangedAt, nonceChangedAt, warnedAt, hardWarnedAt time.Time
	deferredWarnedAt, planningWarnedAt                    time.Time
	planningOnly                                          bool // the last pass failed before choosing work
}

func (s *claimPublisher) run(ctx context.Context) error {
	interval := s.interval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		var err error
		if cause := s.failure.Load(); cause != nil {
			err = *cause
		} else {
			err = s.tick(ctx)
			if txsender.HardFailure(err) {
				s.failure.CompareAndSwap(nil, &err)
			}
		}
		if txsender.ShutdownOnly(ctx, err, txsender.ErrLimit) || ctx.Err() != nil && err == nil {
			return ctx.Err()
		}
		if txsender.HardFailure(err) {
			if time.Since(s.hardWarnedAt) >= claimWarningInterval {
				s.Logger.Error("Claim publication stopped; operator action required", "error", err)
				s.hardWarnedAt = time.Now()
			}
		} else if err != nil && !txsender.WaitOnly(err, txsender.ErrLimit) {
			warnedAt := &s.deferredWarnedAt
			if s.planningOnly {
				warnedAt = &s.planningWarnedAt
			}
			s.reportPublicationError(ctx, warnedAt, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *claimPublisher) tick(ctx context.Context) error {
	if cause := s.failure.Load(); cause != nil {
		return *cause
	}
	s.prepared, s.candidates, s.planningOnly = false, nil, false
	defer func() { s.candidates = nil }()
	now := time.Now()
	for action, until := range s.cooldowns {
		if !now.Before(until) {
			delete(s.cooldowns, action)
		}
	}
	for epoch, until := range s.stagingWarnings {
		if !now.Before(until) {
			delete(s.stagingWarnings, epoch)
		}
	}
	var result error
	// Only preparation failures fall through within this pass. A POST may
	// already have reached the network and must end candidate selection.
	for range claimPageSize {
		s.lastChoice = claimAction{}
		report, err := s.sender.PublishNext(ctx, s)
		if action := s.reportAction(report); err != nil && action != (claimAction{}) {
			kind := claimKindName(action.Kind)
			if report.Canceling {
				kind = "cancel obsolete " + kind
			}
			err = fmt.Errorf("application %s, action %s, epoch %d, nonce %d: %w",
				action.Address, kind, action.Epoch, report.Nonce, err)
		}
		s.reportStalls(report, err)
		s.reportLimit(report, err)
		retainedChoice := report.Current != nil && !report.Current.Neutral && report.Current.Action == s.lastChoice
		if err == nil || report.Attempted || retainedChoice || txsender.PassInvalid(err) || ctx.Err() != nil {
			return errors.Join(result, err)
		}
		if s.lastChoice == (claimAction{}) {
			s.planningOnly = result == nil && report.Current == nil
			return errors.Join(result, err)
		}
		s.cooldowns[s.lastChoice] = time.Now().Add(claimCandidateRetryDelay)
		result = errors.Join(result, err)
	}
	return result
}

func (s *claimPublisher) Next(
	ctx context.Context, block txsender.BlockRef, current *txsender.Current[claimAction],
) (txsender.Decision[claimAction], error) {
	if current != nil && !current.Neutral {
		candidate, state, err := s.currentDecision(ctx, block, current.Action)
		if err != nil {
			return txsender.Wait[claimAction]("current claim read failed"), err
		}
		if state == txsender.ProtocolWait {
			return txsender.Wait[claimAction]("claim is not mature"), nil
		}
		if state == txsender.Useful {
			s.lastChoice = candidate.action
			return txsender.Publish(candidate.action, s.decorate(candidate)), nil
		}
	}
	if !s.prepared {
		s.prepared = true
		if err := s.prepareCandidates(ctx, block); err != nil {
			if current == nil {
				return txsender.Wait[claimAction]("candidate reads failed"), err
			}
			// Current obsolescence was established independently above.
			s.reportPublicationError(ctx, &s.planningWarnedAt,
				fmt.Errorf("other claim work could not be read before cancellation: %w", err))
		}
	}
	if len(s.candidates) != 0 {
		candidate := s.candidates[0]
		s.candidates = s.candidates[1:]
		s.lastChoice = candidate.action
		return txsender.Publish(candidate.action, s.decorate(candidate)), nil
	}
	if current != nil {
		return txsender.Cancel[claimAction]("claim action is no longer useful"), nil
	}
	return txsender.Wait[claimAction]("no useful claim work"), nil
}

func (s *claimPublisher) Recheck(ctx context.Context, block txsender.BlockRef, action claimAction) (txsender.Eligibility, error) {
	_, state, err := s.currentDecision(ctx, block, action)
	return state, err
}

func (s *claimPublisher) currentDecision(
	ctx context.Context, block txsender.BlockRef, action claimAction,
) (claimCandidate, txsender.Eligibility, error) {
	work := repository.ClaimWork{Application: &model.Application{ID: action.ApplicationID, IApplicationAddress: action.Address},
		Epoch: &model.Epoch{Index: action.Epoch}}
	candidate, state, err := s.decide(ctx, block, work, true)
	var mismatch *claimSignerMismatch
	if errors.As(err, &mismatch) {
		if !time.Now().Before(s.cooldowns[action]) {
			s.Logger.Warn("Held claim signer authorization changed; replacing obsolete request", "address", action.Address,
				"epoch", action.Epoch, "action", claimKindName(action.Kind), "signer", s.signer, "error", err)
			s.cooldowns[action] = time.Now().Add(claimCandidateRetryDelay)
		}
		return claimCandidate{}, txsender.Obsolete, nil
	}
	if err == nil && ((state == txsender.Useful && candidate.action != action) ||
		(state == txsender.ProtocolWait && isClaimSubmission(action.Kind) && candidate.action.Kind == acceptClaimKind)) {
		state = txsender.Obsolete
	}
	return candidate, state, err
}

func (s *claimPublisher) decorate(candidate claimCandidate) txsender.Request {
	work := repository.ClaimWork{Application: candidate.app, Epoch: candidate.epoch}
	if isClaimSubmission(candidate.action.Kind) && candidate.epoch.Status != model.EpochStatus_ClaimComputed &&
		!time.Now().Before(s.cooldowns[claimPlanningRefusal(work)]) {
		s.Logger.Warn("Re-publishing recorded claim absent from the current chain view", "application", candidate.app.Name,
			"address", candidate.app.IApplicationAddress, "epoch", candidate.epoch.Index, "stored_status", candidate.epoch.Status)
		s.cooldowns[claimPlanningRefusal(work)] = time.Now().Add(claimCandidateRetryDelay)
	}
	request := candidate.request
	request.Logger = s.Logger.With("application", candidate.app.Name, "address", candidate.app.IApplicationAddress,
		"action", claimKindName(candidate.action.Kind), "epoch", candidate.epoch.Index)
	return request
}

// Staging warnings have their own per-epoch clock: a different refusal must
// not hide this cause, and a diagnostic must not delay an otherwise useful action.
func (s *claimPublisher) warnStagingContradiction(app *model.Application, epoch *model.Epoch, latest uint64) {
	key := claimAction{ApplicationID: app.ID, Address: app.IApplicationAddress, Epoch: epoch.Index}
	now := time.Now()
	if now.Before(s.stagingWarnings[key]) {
		return
	}
	if s.stagingWarnings == nil {
		s.stagingWarnings = make(map[claimAction]time.Time)
	}
	s.Logger.Warn("Claim publication waiting: recorded staging contradicts latest view",
		"application", app.Name, "address", app.IApplicationAddress, "epoch", epoch.Index,
		"staged_at_block", *epoch.StagedAtBlock, "latest_block", latest, "observation_policy", s.observationPolicy)
	s.stagingWarnings[key] = now.Add(claimWarningInterval)
}

func (s *claimPublisher) prepareCandidates(ctx context.Context, block txsender.BlockRef) error {
	var result error
	// Each source reserves later-row capacity. All rows in these windows are
	// inspected before choosing: a useful oldest row cannot hide its tail.
	for _, source := range []struct {
		kind int
		size uint64
	}{
		{acceptCandidatePage, 6}, {repairCandidatePage, 5}, {submitCandidatePage, 5},
	} {
		query := s.repository.SelectClaimsToSubmitPage
		if source.kind == acceptCandidatePage {
			query = s.repository.SelectClaimsToAcceptPage
		}
		if source.kind == repairCandidatePage {
			query = s.repository.SelectClaimsToStagePage
		}
		s.pages[source.kind].PublishableOnly = true
		page, err := query(ctx, s.pages[source.kind], source.size)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		s.pages[source.kind].AfterApplicationID = page.ApplicationID
		for _, work := range page.Work {
			s.pages[source.kind].Advance(work, page)
			candidate, state, err := s.decide(ctx, block, work, false)
			if err != nil {
				var mismatch *claimSignerMismatch
				if errors.As(err, &mismatch) || errors.Is(err, txsender.ErrUnsupported) {
					if time.Now().Before(s.cooldowns[claimPlanningRefusal(work)]) {
						continue
					}
					s.cooldowns[claimPlanningRefusal(work)] = time.Now().Add(claimCandidateRetryDelay)
					s.Logger.Warn("Claim candidate refused; retrying after cooldown", "application", work.Application.Name,
						"address", work.Application.IApplicationAddress, "epoch", work.Epoch.Index, "signer", s.signer, "error", err)
				}
				result = errors.Join(result, fmt.Errorf("application %s (%s), epoch %d: %w",
					work.Application.Name, work.Application.IApplicationAddress, work.Epoch.Index, err))
				continue
			}
			if state != txsender.Useful || time.Now().Before(s.cooldowns[candidate.action]) {
				continue
			}
			s.candidates = append(s.candidates, candidate)
		}
	}
	slices.SortStableFunc(s.candidates, func(a, b claimCandidate) int {
		if a.action.Kind == acceptClaimKind && b.action.Kind != acceptClaimKind {
			return -1
		}
		if b.action.Kind == acceptClaimKind && a.action.Kind != acceptClaimKind {
			return 1
		}
		return 0
	})
	// A failed unrelated read does not discard candidates established useful.
	if len(s.candidates) != 0 {
		if result != nil {
			s.reportPublicationError(ctx, &s.planningWarnedAt, result)
		}
		return nil
	}
	return result
}

// A zero-kind key throttles a planning refusal before immutable calldata can be
// selected. It is local to the service; it is never a sender action identity.
func claimPlanningRefusal(work repository.ClaimWork) claimAction {
	return claimAction{ApplicationID: work.Application.ID, Address: work.Application.IApplicationAddress, Epoch: work.Epoch.Index}
}

func (s *claimPublisher) reportAction(report txsender.Report[claimAction]) claimAction {
	if s.lastChoice != (claimAction{}) {
		return s.lastChoice
	}
	if report.Current != nil {
		return report.Current.Action
	}
	return claimAction{}
}

func (s *claimPublisher) reportLimit(report txsender.Report[claimAction], err error) {
	action := s.reportAction(report)
	if !errors.Is(err, txsender.ErrLimit) || action == (claimAction{}) {
		return
	}
	if time.Now().Before(s.cooldowns[action]) {
		return
	}
	kind := claimKindName(action.Kind)
	if report.Canceling {
		kind = "cancel obsolete " + kind
	}
	s.Logger.Warn("Claim publication waiting for funding or a policy limit", "address", action.Address,
		"action", kind, "epoch", action.Epoch, "signer", s.signer, "nonce", report.Nonce, "error", err)
	s.cooldowns[action] = time.Now().Add(claimCandidateRetryDelay)
}

func (s *claimPublisher) reportStalls(report txsender.Report[claimAction], err error) {
	if report.Block == (txsender.BlockRef{}) {
		return
	}
	now := time.Now()
	if report.Current == nil && s.lastChoice != (claimAction{}) && s.lastLive != (txsender.BlockRef{}) &&
		report.Nonce > s.lastNonce && (errors.Is(err, txsender.ErrStale) || report.Class == txsender.NonceLow) &&
		now.Sub(s.warnedAt) >= claimWarningInterval {
		s.Logger.Warn("Claim publication lost a race while signer nonce advanced; inspect other signer users",
			"address", s.lastChoice.Address, "action", claimKindName(s.lastChoice.Kind), "epoch", s.lastChoice.Epoch,
			"signer", s.signer, "previous_nonce", s.lastNonce, "nonce", report.Nonce, "error", err)
		s.warnedAt = now
	}
	if report.Attempted && report.Current != nil && s.hadPending && report.Nonce > s.lastNonce &&
		report.Current.Action == s.lastPending && now.Sub(s.warnedAt) >= claimWarningInterval {
		action := report.Current.Action
		s.Logger.Warn("Claim action selected again after signer nonce advanced; inspect execution and other signer users",
			"address", action.Address,
			"action", claimKindName(action.Kind), "epoch", action.Epoch, "signer", s.signer, "nonce", report.Nonce)
		s.warnedAt = now
	}
	if s.headChangedAt.IsZero() || s.lastLive != report.Block {
		s.headChangedAt = now
	}
	pending := report.Current != nil
	if pending {
		s.lastPending = report.Current.Action
	}
	if !pending || !s.hadPending || s.nonceChangedAt.IsZero() || s.lastNonce != report.Nonce {
		s.nonceChangedAt = now
	}
	s.hadPending = pending
	s.lastLive, s.lastNonce = report.Block, report.Nonce
	if now.Sub(s.warnedAt) < claimWarningInterval {
		return
	}
	logger := s.Logger.With("signer", s.signer, "nonce", report.Nonce)
	if report.Attempted {
		logger = logger.With("reply", report.Class)
	}
	if pending {
		action := report.Current.Action
		logger = logger.With("address", action.Address, "action", claimKindName(action.Kind), "epoch", action.Epoch)
	}
	if now.Sub(s.headChangedAt) >= claimWarningInterval {
		logger.Warn("Claim publication head has not advanced", "block", report.Block.Number, "hash", report.Block.Hash)
		s.warnedAt = now
	} else if pending && now.Sub(s.nonceChangedAt) >= claimWarningInterval {
		logger.Warn("Claim publication nonce has not advanced", "block", report.Block.Number)
		s.warnedAt = now
	}
}

func (s *claimPublisher) reportPublicationError(ctx context.Context, warnedAt *time.Time, err error) {
	if txsender.ShutdownOnly(ctx, err, txsender.ErrLimit) {
		return
	}
	if time.Since(*warnedAt) < claimWarningInterval {
		s.Logger.Debug("Claim publication deferred", "error", err)
		return
	}
	s.Logger.Warn("Claim publication deferred", "error", err)
	*warnedAt = time.Now()
}

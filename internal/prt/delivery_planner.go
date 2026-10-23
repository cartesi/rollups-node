// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
)

// tick may skip a refused candidate, but ends at the first POST. The bound also
// prevents a failing application from making one publisher pass unbounded.
func (s *delivery) tick(ctx context.Context) (err error) {
	defer func() {
		if txsender.HardFailure(err) {
			s.stopPublication(err)
			s.reportHardPublicationFailure(err)
		}
	}()
	if s.publicationStopped() {
		return s.publicationFailure()
	}
	if s.sender == nil {
		return errors.New("PRT submission requires a transaction sender")
	}
	for key, until := range s.candidateCooldown {
		if !time.Now().Before(until) {
			delete(s.candidateCooldown, key)
		}
	}
	var refused error
	pass := &deliveryPass{delivery: s}
	for range publicationCandidateLimit {
		pass.selected = nil
		report, publishErr := s.sender.PublishNext(ctx, pass)
		s.recordLiveProgress(report, pass.selected, publishErr)
		if publishErr == nil {
			return refused
		}
		refusedAction := pass.selected
		if refusedAction == nil && report.Current != nil {
			refusedAction = &report.Current.Action
		}
		if refusedAction != nil {
			key := *refusedAction
			if !errors.Is(ctx.Err(), context.Canceled) &&
				(!txsender.WaitOnly(publishErr) || errors.Is(publishErr, txsender.ErrLimit)) {
				s.warnPublicationRefusal(key, report, publishErr)
			}
			kind := actionName(key.Kind)
			if report.Canceling {
				kind = "cancel obsolete " + kind
			}
			publishErr = fmt.Errorf("PRT application %s, action %s, epoch %d, nonce %d: %w",
				key.Application, kind, key.Epoch, report.Nonce, publishErr)
		}
		if report.Attempted || pass.selected == nil || txsender.PassInvalid(publishErr) ||
			ctx.Err() != nil {
			return errors.Join(refused, publishErr)
		}
		s.candidateCooldown[*pass.selected] = time.Now().Add(s.pollInterval)
		refused = errors.Join(refused, publishErr)
	}
	return refused
}

// A pass discovers one bounded candidate window. Each entry gets at most one
// preparation attempt, even when a slow pass outlasts the refusal cooldown.
// Requests remain subject to the sender's exact simulation and fresh Recheck.
type deliveryPass struct {
	*delivery
	planned       bool
	candidates    []plannedAction
	planErr       error
	selected      *actionID
	heldAttempted bool
}

// Next owns selection. The current action is checked before optional backlog
// reads; a failed observation pass cannot stop a still-useful current request.
func (s *deliveryPass) Next(ctx context.Context, block txsender.BlockRef,
	current *txsender.Current[actionID]) (txsender.Decision[actionID], error) {
	var held *plannedAction
	var heldErr error
	var heldEligibility txsender.Eligibility
	if current != nil && !current.Neutral {
		held, heldEligibility, heldErr = s.decide(ctx, block, current.Action)
		if heldErr != nil {
			return txsender.Decision[actionID]{}, heldErr
		}
	}
	if !s.planned {
		s.planned = true
		facts, err := s.observationReference(ctx)
		switch {
		case err != nil:
			s.planErr = err
			if held != nil && s.planningWarningDue(actionID{}) {
				s.Logger.Warn("PRT fresh-work observation is unavailable; maintaining current work",
					"block", block.Number, "error", err)
			}
		case facts.Number > block.Number:
			s.planErr = txsender.ErrInconclusive
		default:
			s.candidates, s.planErr = s.delivery.candidates(ctx, block, facts)
		}
	}
	if txsender.PassInvalid(s.planErr) {
		return txsender.Decision[actionID]{}, s.planErr
	}
	for len(s.candidates) != 0 {
		candidate := s.candidates[0]
		s.candidates = s.candidates[1:]
		if time.Now().Before(s.candidateCooldown[candidate.Key]) {
			continue
		}
		// Useful held work keeps its nonce unless a verified join has an earlier
		// deadline. Unknown held evidence does not itself authorize substitution.
		if current != nil && !current.Neutral && heldEligibility != txsender.Obsolete {
			if candidate.Key == current.Action || candidate.Deadline == 0 ||
				current.Action.Deadline != 0 && candidate.Deadline >= current.Action.Deadline {
				continue
			}
		}
		s.selected = new(candidate.Key)
		return txsender.Publish(candidate.Key, candidate.Request), nil
	}
	if held != nil && heldEligibility == txsender.Useful {
		if s.heldAttempted || time.Now().Before(s.candidateCooldown[held.Key]) {
			return txsender.Wait[actionID]("current candidate is cooling down after a refusal"), nil
		}
		s.heldAttempted = true
		s.selected = new(held.Key)
		return txsender.Publish(held.Key, held.Request), nil
	}
	if current != nil && !current.Neutral {
		if heldEligibility == txsender.Obsolete {
			return txsender.Cancel[actionID]("current PRT action is no longer useful"), nil
		}
		return txsender.Wait[actionID]("current PRT action waits for its protocol window"), nil
	}
	if current != nil && current.Neutral {
		return txsender.Cancel[actionID]("continue cancellation of obsolete PRT action"), nil
	}
	if s.planErr != nil {
		return txsender.Decision[actionID]{}, s.planErr
	}
	return txsender.Wait[actionID]("no useful PRT action"), nil
}

// Recheck repeats exactly the selected domain decision. It does not advance a
// page, choose another candidate, or consult a second pending-request cache.
func (s *delivery) Recheck(ctx context.Context, block txsender.BlockRef, action actionID) (txsender.Eligibility, error) {
	_, eligibility, err := s.decide(ctx, block, action)
	return eligibility, err
}

func (s *delivery) decide(ctx context.Context, block txsender.BlockRef, action actionID,
) (*plannedAction, txsender.Eligibility, error) {
	app, err := s.repository.GetApplication(ctx, action.Application.Hex())
	if errors.Is(err, repository.ErrNotFound) || err == nil && (app == nil || app.ID != action.ApplicationID) {
		return nil, txsender.Obsolete, nil
	}
	if err != nil {
		return nil, txsender.ProtocolWait, err
	}
	var call *plannedAction
	if action.Kind == recoverBond {
		call, err = s.recoveryCall(withLiveBlock(ctx, block.Number, block.Hash), app, action.Epoch, action.Target, block, s.observedFacts)
	} else {
		call, err = s.planEpochAction(ctx, app, action.Kind, block, s.observedFacts, &action)
	}
	if errors.Is(err, errProtocolWait) {
		return nil, txsender.ProtocolWait, nil
	}
	if errors.Is(err, txsender.ErrIneligible) {
		return nil, txsender.Obsolete, nil
	}
	if err != nil {
		return nil, txsender.ProtocolWait, err
	}
	if call == nil || call.Key != action {
		return nil, txsender.Obsolete, nil
	}
	return call, txsender.Useful, nil
}

var errProtocolWait = errors.New("PRT action waits for its protocol window")

func (s *delivery) candidates(ctx context.Context, block, facts txsender.BlockRef) ([]plannedAction, error) {
	var calls []plannedAction
	var errs []error
	for _, kind := range []actionKind{joinTournament, recoverBond, acceptTournament, stageTournament} {
		apps, err := s.nextDeliveryApplications(ctx, kind)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, app := range apps {
			if err := ctx.Err(); err != nil {
				return calls, errors.Join(append(errs, err)...)
			}
			call, planErr := s.planAction(ctx, app, kind, block, facts)
			s.warnJoinPlanningRefusal(app, block, planErr)
			s.warnPlanningPause(app, kind, block, planErr)
			if planErr != nil && !errors.Is(planErr, txsender.ErrIneligible) {
				errs = append(errs, fmt.Errorf("planning PRT application %s, action %s: %w", app.Name, actionName(kind), planErr))
			}
			if txsender.PassInvalid(planErr) {
				return calls, errors.Join(errs...)
			}
			if call != nil && planErr == nil {
				calls = append(calls, *call)
			}
		}
	}
	slices.SortStableFunc(calls, func(a, b plannedAction) int {
		if a.Deadline != b.Deadline {
			if a.Deadline == 0 {
				return 1
			}
			if b.Deadline == 0 || a.Deadline < b.Deadline {
				return -1
			}
			return 1
		}
		return int(a.Key.Kind) - int(b.Key.Kind)
	})
	return calls, errors.Join(errs...)
}

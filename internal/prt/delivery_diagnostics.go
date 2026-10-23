// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
)

const (
	publicationWarningInterval = 30 * time.Second
	frozenLiveHeadInterval     = time.Minute
	stalledNonceBlocks         = 6
)

type publicationDiagnostics struct {
	head                                                   txsender.BlockRef
	headSince                                              time.Time
	nonce, nonceAtBlock                                    uint64
	hadNonce                                               bool
	warnedHead, warnedNonce, warnedJoin, warnedHardFailure time.Time
	lastReply                                              txsender.ReplyClass
	joinRefusals                                           map[int64]joinRefusalWarning
	planningWarnings                                       map[actionID]time.Time
	lastPublishedAction                                    actionID
	lastPublishedNonce                                     uint64
	hadPublication                                         bool
	warnedRepeatedAction                                   time.Time
	warnedPublication                                      time.Time
}

func actionName(kind actionKind) string {
	switch kind {
	case noAction:
		return "cancel obsolete transaction"
	case joinTournament:
		return "join tournament"
	case stageTournament:
		return "stage tournament result"
	case acceptTournament:
		return "accept tournament result"
	case recoverBond:
		return "recover bond"
	default:
		return fmt.Sprintf("unknown action %d", kind)
	}
}

func warningDue(last time.Time) bool { return time.Since(last) >= publicationWarningInterval }

func (s *delivery) recordLiveProgress(report txsender.Report[actionID], selected *actionID, err error) {
	if report.Block == (txsender.BlockRef{}) {
		return
	}
	d := &s.diagnostics
	block := report.Block
	if report.Current == nil && selected != nil && d.head != (txsender.BlockRef{}) && report.Nonce > d.nonce &&
		(errors.Is(err, txsender.ErrStale) || report.Class == txsender.NonceLow) && warningDue(d.warnedNonce) {
		s.Logger.Warn("PRT publication lost a race while signer nonce advanced; inspect other signer users",
			"address", selected.Application, "action", actionName(selected.Kind), "epoch", selected.Epoch,
			"signer", s.txOptsFactory.From(), "previous_nonce", d.nonce, "nonce", report.Nonce, "error", err)
		d.warnedNonce = time.Now()
	}
	if d.head != block {
		d.head, d.headSince = block, time.Now()
	} else if time.Since(d.headSince) >= frozenLiveHeadInterval && warningDue(d.warnedHead) {
		s.Logger.Warn("PRT latest block has not advanced; publication may miss a deadline", "block", block.Number)
		d.warnedHead = time.Now()
	}
	if report.Attempted {
		d.lastReply = report.Class
		if report.Current != nil && !report.Current.Neutral &&
			(report.Class == txsender.Accepted || report.Class == txsender.Known || report.Class == txsender.Uncertain) {
			action := report.Current.Action
			if d.hadPublication && report.Nonce > d.lastPublishedNonce && action == d.lastPublishedAction &&
				warningDue(d.warnedRepeatedAction) {
				s.Logger.Warn("PRT action is still required after the signer nonce advanced; inspect execution and other signer users",
					"address", action.Application, "action", actionName(action.Kind), "epoch", action.Epoch,
					"previous_nonce", d.lastPublishedNonce, "nonce", report.Nonce, "block", block.Number)
				d.warnedRepeatedAction = time.Now()
			}
			d.lastPublishedAction, d.lastPublishedNonce, d.hadPublication = action, report.Nonce, true
		}
	}
	if report.Current == nil {
		d.nonce = report.Nonce
		d.hadNonce = false
		return
	}
	action := report.Current.Action
	if !d.hadNonce || d.nonce != report.Nonce || block.Number < d.nonceAtBlock {
		d.nonce, d.nonceAtBlock, d.hadNonce = report.Nonce, block.Number, true
	}
	if block.Number-d.nonceAtBlock >= stalledNonceBlocks && warningDue(d.warnedNonce) {
		s.Logger.Warn("PRT signer nonce is stalled", "nonce", report.Nonce, "address", action.Application,
			"action", actionName(action.Kind), "epoch", action.Epoch, "blocks_held", block.Number-d.nonceAtBlock, "last_reply", d.lastReply)
		d.warnedNonce = time.Now()
	}
	if !report.Current.Neutral && action.Kind == joinTournament && action.Deadline > block.Number &&
		action.Deadline-block.Number <= joinWarningMargin && warningDue(d.warnedJoin) {
		s.Logger.Warn("PRT join remains pending near its deadline", "address", action.Application,
			"epoch", action.Epoch, "nonce", report.Nonce, "blocks_remaining", action.Deadline-block.Number, "last_reply", d.lastReply)
		d.warnedJoin = time.Now()
	}
}

type consensusRefusal struct {
	key    actionID
	reason error
}

func (e *consensusRefusal) Error() string { return e.reason.Error() }
func (e *consensusRefusal) Unwrap() error { return e.reason }

func (s *delivery) planningWarningDue(key actionID) bool {
	if s.diagnostics.planningWarnings == nil {
		s.diagnostics.planningWarnings = make(map[actionID]time.Time)
	}
	for action, last := range s.diagnostics.planningWarnings {
		if time.Since(last) >= 2*publicationWarningInterval {
			delete(s.diagnostics.planningWarnings, action)
		}
	}
	if !warningDue(s.diagnostics.planningWarnings[key]) {
		return false
	}
	s.diagnostics.planningWarnings[key] = time.Now()
	return true
}

func (s *delivery) warnPlanningPause(app *model.Application, kind actionKind, block txsender.BlockRef, err error) {
	if err == nil || txsender.HardFailure(err) {
		return
	}
	var join *joinRefusal
	if errors.As(err, &join) {
		return // warnJoinPlanningRefusal supplies its tournament and deadline context.
	}
	var refusal *consensusRefusal
	if errors.As(err, &refusal) {
		if !s.planningWarningDue(refusal.key) {
			return
		}
		s.Logger.Warn("PRT cannot publish consensus action with the registered contracts",
			"application", app.Name, "address", app.IApplicationAddress, "epoch", refusal.key.Epoch,
			"action", actionName(kind), "block", block.Number, "reason", refusal.reason)
		return
	}
	if errors.Is(err, txsender.ErrIneligible) || errors.Is(err, errProtocolWait) || txsender.WaitOnly(err) {
		return
	}
	if s.planningWarningDue(actionID{ApplicationID: app.ID, Application: app.IApplicationAddress, Kind: kind}) {
		s.Logger.Warn("PRT application action could not be planned; other applications can continue",
			"application", app.Name, "address", app.IApplicationAddress, "action", actionName(kind),
			"block", block.Number, "error", err)
	}
}

func (s *delivery) warnPublicationRefusal(action actionID, report txsender.Report[actionID], err error) {
	if !s.planningWarningDue(action) {
		return
	}
	kind := actionName(action.Kind)
	if report.Canceling {
		kind = "cancel obsolete " + kind
	}
	s.Logger.Warn("PRT transaction could not publish; planning will retry", "address", action.Application,
		"action", kind, "epoch", action.Epoch, "nonce", report.Nonce,
		"block", report.Block.Number, "deadline", action.Deadline, "error", err)
}

func (s *delivery) reportPublicationError(ctx context.Context, err error) {
	if txsender.ShutdownOnly(ctx, err, repository.ErrTournamentIntervalStale, txsender.ErrLimit) {
		return
	}
	if !warningDue(s.diagnostics.warnedPublication) {
		s.Logger.Debug("PRT publication deferred", "error", err)
		return
	}
	s.Logger.Warn("PRT publication deferred", "error", err)
	s.diagnostics.warnedPublication = time.Now()
}

func (s *delivery) reportHardPublicationFailure(err error) {
	if warningDue(s.diagnostics.warnedHardFailure) {
		s.Logger.Error("PRT publication stopped by a protected-state contradiction; operator intervention required", "error", err)
		s.diagnostics.warnedHardFailure = time.Now()
	}
}

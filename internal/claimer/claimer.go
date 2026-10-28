// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package claimer drives the on-chain claim lifecycle for Authority and
// Quorum consensus apps.
//
// Tick observes claims and events at the configured block policy. A separate publisher
// reads committed rows and maintains its signer without waiting for Tick.
// The Claimer owns its signer independently of the PRT service.
//
// The in-DB lifecycle mirrors the v3 IConsensus contract:
//
//	CLAIM_COMPUTED -> CLAIM_SUBMITTED -> CLAIM_STAGED -> CLAIM_ACCEPTED
//
// There are also shortcut transitions for recovery after restart or for reader
// mode catching up from chain state:
//
//	CLAIM_COMPUTED -> CLAIM_STAGED
//	CLAIM_COMPUTED -> CLAIM_ACCEPTED
//
// In Quorum, an epoch can also move to CLAIM_REJECTED when another validator's
// claim wins before our claim reaches CLAIM_STAGED. This transition is coupled
// with the application becoming DIVERGED: the local claim did not win, and
// the app must stop until the operator or guardian handles the divergence.
//
// Other divergence paths do not necessarily rewrite the epoch to
// CLAIM_REJECTED. For example, Authority divergence and any divergence after
// our local epoch is CLAIM_STAGED are app-level failures. In those cases the
// epoch may keep its current status, but the application becomes DIVERGED
// with a reason that says where the mismatch was found: submit, stage, or
// accept.
//
// Foreclosed apps can also move remaining pre-foreclosure claim work to
// CLAIM_FORECLOSED. This is used after read-only reconciliation has checked
// that the claim was not already STAGED or ACCEPTED on chain. The app itself
// stays enabled for L1 observation with foreclose_block set and normally keeps
// status OK. If it was already DIVERGED because of a divergence, EVM reader
// preserves that status while still recording foreclose_block.
//
// PRT (DaveConsensus) stages and accepts tournament results through the PRT
// service. The claimer queries exclude PRT apps.
package claimer

import (
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"math/big"
	"time"
)

func (s *Service) Tick(ctx context.Context) (reschedule bool, err error) {
	defer func() {
		if txsender.HardFailure(err) {
			cause := err
			s.failure.CompareAndSwap(nil, &cause)
			if time.Since(s.observationHardWarnedAt) < claimWarningInterval {
				err = nil
			} else {
				s.observationHardWarnedAt = time.Now()
			}
		}
		if txsender.ShutdownOnly(ctx, err, txsender.ErrLimit) {
			reschedule, err = false, nil
		} else if txsender.WaitOnly(err) {
			s.Logger.Debug("Claim observation deferred", "error", err)
			err = nil
		}
	}()
	block, err := s.blockchain.getDefaultBlockNumber(ctx)
	if err != nil {
		return false, err
	}
	s.consensusAddressChecks = map[consensusAddressCheckKey]error{}
	defer func() { s.consensusAddressChecks = nil }()
	for kind := claimSubmitPage; kind <= claimAcceptPage; kind++ {
		count, stageErr := s.observeClaimStage(ctx, kind, block)
		reschedule = reschedule || count > 0
		err = errors.Join(err, stageErr)
		if txsender.HardFailure(stageErr) || ctx.Err() != nil {
			break
		}
	}
	return reschedule, err
}

func (s *Service) observeClaimStage(ctx context.Context, kind int, block *big.Int) (int, error) {
	page, err := s.selectClaimPage(ctx, kind)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range page.Work {
		epochs := map[int64]*model.Epoch{item.Application.ID: item.Epoch}
		apps := map[int64]*model.Application{item.Application.ID: item.Application}
		var progressed int
		var observeErr error
		switch kind {
		case claimSubmitPage:
			progressed, observeErr = s.submitClaimsAndUpdateDatabase(ctx, epochs, apps, block)
		case claimStagePage:
			progressed, observeErr = s.stageClaimsAndUpdateDatabase(ctx, epochs, apps, block)
		default:
			accepted, acceptErr := s.acceptClaimsAndUpdateDatabase(ctx, epochs, apps, block)
			issued, issueErr := s.observeStagedClaims(ctx, epochs, apps, block)
			progressed, observeErr = accepted+issued, errors.Join(acceptErr, issueErr)
		}
		s.pageCursors[kind].Advance(item, page)
		count += progressed
		err = errors.Join(err, observeErr)
		if txsender.HardFailure(observeErr) || ctx.Err() != nil {
			break
		}
	}
	if kind == claimAcceptPage {
		foreclosed, listErr := s.listEnabledForeclosedNonPRTApps(ctx)
		err = errors.Join(err, listErr, s.processForeclosedApps(ctx, foreclosed, block.Uint64()))
	}
	return count, err
}

func (s *Service) Ready() bool {
	return s.failure.Load() == nil && (!s.submissionEnabled || s.publicationReady.Load())
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package claimer drives the on-chain claim lifecycle for Authority and
// Quorum consensus apps.
//
// Each Tick runs the claim stages in order: submit, stage, send accept,
// confirm accept, and accept by event. The code takes new DB snapshots between
// groups of stages so later stages see the rows updated by earlier stages.
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
// PRT (DaveConsensus) uses a different path. PRT epochs go directly from
// CLAIM_COMPUTED to CLAIM_ACCEPTED through tournament resolution. They never
// reach CLAIM_STAGED, and the claimer queries exclude PRT apps.
package claimer

import (
	"context"
	"errors"
)

func (s *Service) Tick(ctx context.Context) (bool, error) {
	// Use the same finalized block number for all chain reads in this tick.
	// This is one RPC per tick even when there is no DB work. The call is
	// cheap, and Tick already runs on a polling interval.
	defaultBlockNumber, err := s.blockchain.getDefaultBlockNumber(ctx)
	if err != nil {
		// During shutdown, the parent context is canceled and RPC/DB calls
		// return context.Canceled. Ignore only that normal shutdown case. Other
		// errors, such as deadline exceeded, must still be returned.
		if errors.Is(err, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "stage", "getDefaultBlockNumber", "error", err)
			return false, nil
		}
		return false, err
	}
	s.consensusAddressChecks = map[consensusAddressCheckKey]error{}
	defer func() {
		s.consensusAddressChecks = nil
	}()

	// Each stage reads its input after the previous stage finishes. This lets
	// stage 2 see rows that stage 1 just moved to SUBMITTED. If all reads ran
	// first, one chain update could take several ticks to reach STAGED.

	// Stage 1: submit. COMPUTED -> SUBMITTED, or directly to STAGED when the
	// transaction receipt already contains ClaimStaged.
	prevSubmittedOrStaged, computedEpochs, computedApps, errComputed := s.repository.SelectClaimsToSubmitPerApp(ctx)
	if errComputed != nil {
		if errors.Is(errComputed, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "stage", "SelectClaimsToSubmitPerApp", "error", errComputed)
			return false, nil
		}
		return false, errComputed
	}
	submitted, err := s.submitClaimsAndUpdateDatabase(ctx, prevSubmittedOrStaged, computedEpochs, computedApps, defaultBlockNumber)

	// Stage 2: stage. SUBMITTED -> STAGED. This read sees stage 1 updates.
	prevAcceptedForSubmitted, submittedEpochs, submittedApps, errSubmitted := s.repository.SelectClaimsToStagePerApp(ctx)
	if errSubmitted != nil {
		if errors.Is(errSubmitted, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "stage", "SelectClaimsToStagePerApp", "error", errSubmitted)
			return false, nil
		}
		return false, errors.Join(err, errSubmitted)
	}
	staged, stageErr := s.stageClaimsAndUpdateDatabase(ctx, prevAcceptedForSubmitted, submittedEpochs, submittedApps, defaultBlockNumber)
	err = errors.Join(err, stageErr)

	// Stages 3, 4, and 5: accept. STAGED -> ACCEPTED by our own transaction,
	// another party's event, or a getClaim read before we send acceptClaim.
	// This read sees stage 1 and stage 2 updates.
	prevAcceptedForStaged, stagedEpochs, stagedApps, errStaged := s.repository.SelectClaimsToAcceptPerApp(ctx)
	if errStaged != nil {
		if errors.Is(errStaged, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "stage", "SelectClaimsToAcceptPerApp", "error", errStaged)
			return false, nil // TODO:[maia] should we discard the potential database update errors from calls above?
		}
		return false, errors.Join(err, errStaged)
	}

	// Foreclosed apps still need some read-only claim work. A claim accepted
	// before foreclosure must be copied from chain into the local DB. The
	// read-only steps inside the normal stages do that:
	// findClaimSubmittedEvent, getClaim, and findClaimStagedEvent.
	//
	// The submitClaim and acceptClaim paths skip new broadcasts when
	// foreclose_block is set, so this does not spend gas on transactions that
	// must revert.
	//
	// The query below adds foreclosed apps that no longer have pending claim
	// work, so operators can still see drain and reconciliation progress. Once
	// drained, the app remains enabled for L1 observation with foreclose_block
	// set.
	foreclosed, listErr := s.listEnabledForeclosedNonPRTApps(ctx)
	err = errors.Join(err, listErr)

	// Finish the accept side of the lifecycle. First send acceptClaim for
	// staged epochs that are ready. Then check acceptClaim transactions sent in
	// previous ticks. Finally, scan for ClaimAccepted events from any party.
	issuedAccepts, issueErr := s.acceptStagedClaimsAndIssueAcceptTx(ctx, stagedEpochs, stagedApps, defaultBlockNumber)
	err = errors.Join(err, issueErr)

	confirmedAccepts, confirmErr := s.checkAcceptsInFlight(ctx, stagedEpochs, stagedApps, defaultBlockNumber)
	err = errors.Join(err, confirmErr)

	accepted, acceptErr := s.acceptClaimsAndUpdateDatabase(ctx, prevAcceptedForStaged, stagedEpochs, stagedApps, defaultBlockNumber)
	err = errors.Join(err, acceptErr)

	// Keep logging foreclosed apps until all pre-foreclosure work is done.
	// After that, processForeclosedApps has nothing else to change.
	forecloseErr := s.processForeclosedApps(ctx, foreclosed)
	err = errors.Join(err, forecloseErr)

	s.cleanupOrphanedInFlight(computedApps, stagedApps, stagedEpochs)

	s.Logger.Debug("Processed claims for epochs",
		"computed", len(computedEpochs),
		"submitted", len(submittedEpochs),
		"staged", len(stagedEpochs),
	)

	// Signal reschedule whenever pipeline progress was made, even with errors.
	if submitted > 0 || staged > 0 || issuedAccepts > 0 || confirmedAccepts > 0 || accepted > 0 {
		return true, err
	}
	return false, err
}

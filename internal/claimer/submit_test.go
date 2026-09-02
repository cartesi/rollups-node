// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSubmitFirstClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.HexToHash("0x10"), nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(m.claimsInFlight))
	assert.Equal(t, 1, transitions, "submitting a claim counts as a transition")
}

// withForeclosed returns a copy of app with ForecloseBlock / ForecloseTransaction
// populated, matching the in-memory state evmreader leaves behind after
// checkForForeclosure has run on a foreclosed application.

func TestSubmitClaimForeclosesUnstagedForeclosedApp(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := withForeclosed(makeApplication(), 35)
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent, currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()
	// CRITICAL: no submitClaimToBlockchain expectation — testify reports
	// an unexpected call if the guard fails.

	computedEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), computedEpochs, makeApplicationMap(app), endBlock)

	assert.NoError(t, err, "foreclosing an impossible claim is not an error")
	assert.Equal(t, 1, transitions, "CLAIM_FORECLOSED is a local status transition")
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
	assert.Equal(t, 0, len(m.claimsInFlight),
		"no claim should enter the in-flight set for a foreclosed app")
}

func TestSubmitClaimForeclosesUnstagedForeclosedAppWhenSubmissionDisabled(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionEnabled = false
	endBlock := big.NewInt(40)
	app := withForeclosed(makeApplication(), 35)
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent, currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
	assert.Equal(t, 0, len(m.claimsInFlight))
}

// TestSubmitClaimForecloseMidFlight verifies the transition behaviour: a
// healthy app whose claim is broadcast on tick 1 must STOP broadcasting on
// tick 2 once the in-memory ForecloseBlock has been populated (by evmreader
// observing the on-chain Foreclosure event between ticks). The first
// claim's in-flight tracking is preserved — that broadcast already
// happened; it's the *next* epoch's broadcast that must be suppressed.
//
// Two-tick scenario:
//  1. Tick 1: app.ForecloseBlock == 0; epoch N broadcast fires.
//  2. Between ticks: evmreader observes Foreclosure; the in-memory app's
//     ForecloseBlock is set to a value < epoch N+1's LastBlock.
//  3. Tick 2: epoch N+1 in the computedEpochs work-map. The pre-submit
//     reconciliation reads still run (mirroring any pre-foreclosure
//     ACCEPTED state into the local DB), but the broadcast must be
//     SKIPPED so we don't burn gas on a guaranteed ApplicationForeclosed
//     revert.
func TestSubmitClaimForecloseMidFlight(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	epochN := makeComputedEpoch(app, 3)
	epochNPlus1 := makeComputedEpoch(app, 4)
	var prevEvent, currEvent *iconsensus.IConsensusClaimSubmitted

	// --- Tick 1 — healthy app; broadcast fires for epoch N.
	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, epochN, epochN.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epochN, epochN.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, epochN, endBlock)
	tick1TxHash := common.HexToHash("0xa1")
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, epochN).
		Return(tick1TxHash, nil).Once()

	transitions1, errs1 := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), makeEpochMap(epochN), makeApplicationMap(app), endBlock)
	require.Empty(t, errs1)
	require.Equal(t, 1, transitions1, "tick 1: broadcast counts as a transition")
	require.Len(t, m.claimsInFlight, 1, "tick 1: claim enters in-flight set")

	// --- Between ticks — evmreader observes Foreclosure and sets the marker;
	// the in-flight tick-1 receipt resolves successfully. Receipt processing
	// is orthogonal to what this test pins (the broadcast guard on the next
	// epoch); short-circuit it by clearing the in-flight entry directly.
	app.ForecloseBlock = 35
	tick2TxHash := common.HexToHash("0xcafe")
	app.ForecloseTransaction = &tick2TxHash
	delete(m.claimsInFlight, app.ID)

	// --- Tick 2 — foreclosed app + a new computed epoch. Reconciliation
	// runs (pre-foreclosure on-chain state must still mirror to the local
	// DB), but the broadcast is SKIPPED because app.ForecloseBlock != 0.
	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, epochNPlus1, epochNPlus1.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, epochNPlus1, epochNPlus1.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, epochNPlus1, endBlock)
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epochNPlus1.Index).
		Return(nil).Once()
	// CRITICAL: no second submitClaimToBlockchain expectation registered.
	// testify reports an unexpected call if the broadcast guard fails to
	// see the now-populated ForecloseBlock.

	transitions2, errs2 := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), makeEpochMap(epochNPlus1), makeApplicationMap(app), endBlock)
	require.Empty(t, errs2, "foreclosing an impossible claim is not an error")
	assert.Equal(t, 1, transitions2, "tick 2: claim becomes CLAIM_FORECLOSED")
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, epochNPlus1.Status)
	assert.Empty(t, m.claimsInFlight,
		"tick 2: no new in-flight entry — the broadcast guard fires before submit")
}

// TestSubmitClaimReconcilesAcceptedForForeclosedApp verifies the
// counterpoint to the broadcast-guard test: the read-only
// reconciliation path MUST still run for foreclosed apps so that
// pre-foreclosure on-chain-accepted epochs are mirrored to the local DB.
// Without this, a new node bootstrapped against an already-foreclosed
// application would leave its last successful epoch stuck at
// CLAIM_COMPUTED — diverging from chain reality.
func TestSubmitClaimReconcilesAcceptedForForeclosedApp(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := withForeclosed(makeApplication(), 35)
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent, currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	// Chain returns ACCEPTED (status 2) — the reconcile-before-submit
	// path mirrors this to the local DB and skips broadcast entirely.
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusAccepted, currEpoch, 0), nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index, mock.Anything).
		Return(nil).Once()

	computedEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), computedEpochs, makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions, "ACCEPTED reconciliation counts as a transition")
	assert.Equal(t, 0, len(m.claimsInFlight))
}

func TestSubmitClaimReconcilesStagedBeforeBroadcast(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent, currEvent *iconsensus.IConsensusClaimSubmitted
	stagedAt := currEpoch.LastBlock + 2

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	r.On("UpdateEpochReconciledStaged", mock.Anything, app.ID, currEpoch.Index, stagedAt).
		Return(nil).Once()

	computedEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), computedEpochs, makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions, "STAGED reconciliation counts as a transition")
	assert.Empty(t, computedEpochs, "reconciled epoch must leave the computed work map")
	assert.Equal(t, 0, len(m.claimsInFlight), "reconciled staged claim must not be submitted again")
}

func TestReconcileBeforeSubmitAcceptedOutputsMismatchSetsDiverged(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent, currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	claim := makeClaimStatus(claimStatusAccepted, currEpoch, 0)
	claim.StagedOutputsMerkleRoot = common.HexToHash("0xdeadbeef")
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(claim, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, 0, transitions)
	assert.Equal(t, 0, len(m.claimsInFlight))
}

func TestSubmitClaimWithAntecessor(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil
	prevEvent := makeSubmittedEvent(app, prevEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.HexToHash("0x10"), nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(m.claimsInFlight))
	assert.Equal(t, 1, transitions, "submitting a claim counts as a transition")
}

func TestSubmitClaimWithAcceptedAntecessorWithoutClaimTransactionHash(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	prevEpoch.ClaimTransactionHash = nil
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEventWithTxHash(app, prevEpoch, common.HexToHash("0x20"))
	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{prevEvent, currEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(common.HexToHash("0x10"), nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	require.NoError(t, err)
	assert.Len(t, m.claimsInFlight, 1)
	assert.Equal(t, 1, transitions, "accepted predecessor with unknown tx hash must not block submission")
}

func TestSkipSubmitClaimWithStagedAntecessor(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeStagedEpoch(app, 1, 25)
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEvent(app, prevEpoch)
	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 0, transitions, "staged predecessor must block newer claim submission")
}

func TestSkipSubmitFirstClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionEnabled = false
	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 0, transitions, "no transition when submission is disabled")
}

func TestSkipSubmitClaimWithAntecessor(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionEnabled = false
	endBlock := big.NewInt(40)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEvent(app, prevEpoch)
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, len(m.claimsInFlight), 0)
}

func TestUpdateFirstClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, prevEvent, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.TxHash).
		Return(nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 1, transitions, "finding on-chain event counts as a transition")
}

func TestUpdateClaimWithAntecessor(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEvent(app, prevEpoch)
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.TxHash).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, len(m.claimsInFlight), 0)
}

func TestQuorumSubmittedEventsIgnoresForeignDifferentOutputsAndUpdatesMatchingEvent(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	foreignEvent := makeSubmittedEventWithRoots(
		app,
		currEpoch,
		common.HexToHash("0xf001"),
		common.HexToHash("0xf002"),
	)
	foreignEvent.Raw.TxHash = common.HexToHash("0xf003")
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent, currEvent}, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.TxHash).
		Return(nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 1, transitions, "matching later event counts as a transition")
}

func TestQuorumDifferentOutputSubmittedEventStillSubmitsLocalClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	foreignEvent := makeSubmittedEventWithRoots(
		app,
		currEpoch,
		common.HexToHash("0xf001"),
		common.HexToHash("0xf002"),
	)
	txHash := common.HexToHash("0x10")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(txHash, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, txHash, m.claimsInFlight[app.ID].txHash)
	assert.Equal(t, 1, transitions)
}

func TestQuorumForeignMatchingSubmittedEventStillSubmitsLocalClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	foreignEvent := makeSubmittedEvent(app, currEpoch)
	foreignEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000002")
	txHash := common.HexToHash("0x10")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(txHash, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, txHash, m.claimsInFlight[app.ID].txHash)
	assert.Equal(t, 1, transitions)
}

func TestQuorumReaderModeRecordsForeignMatchingSubmittedEvent(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)
	m.submissionEnabled = false

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	foreignEvent := makeSubmittedEvent(app, currEpoch)
	foreignEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000002")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent}, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, foreignEvent.Raw.TxHash).
		Return(nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 1, transitions, "reader mode must mirror a matching Quorum ClaimSubmitted from any validator")
}

func TestQuorumSubmittedEventsIgnoresForeignAdversarialProofAndSubmitsLocalClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	foreignEvent := makeSubmittedEventWithRoots(
		app,
		currEpoch,
		common.HexToHash("0xf001"),
		common.HexToHash("0xf002"),
	)
	foreignEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000002")
	adversarialEvent := makeSubmittedEventWithRoots(
		app,
		currEpoch,
		*currEpoch.OutputsMerkleRoot,
		common.HexToHash("0xf003"),
	)
	adversarialEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000003")
	txHash := common.HexToHash("0x10")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent, adversarialEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(txHash, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, txHash, m.claimsInFlight[app.ID].txHash)
	assert.Equal(t, 1, transitions)
}

func TestQuorumSubmittedEventsOwnMismatchSetsDiverged(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	adversarialEvent := makeSubmittedEventWithRoots(
		app,
		currEpoch,
		*currEpoch.OutputsMerkleRoot,
		common.HexToHash("0xf003"),
	)
	adversarialEvent.Submitter = b.submitterAddress

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{adversarialEvent}, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	currEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), currEpochs, makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, 0, len(currEpochs))
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 0, transitions)
}

func TestQuorumReaderModeIgnoresNonMatchingSubmittedEvent(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)
	m.submissionEnabled = false
	b.hasSubmitter = false

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	foreignEvent := makeSubmittedEventWithRoots(
		app,
		currEpoch,
		*currEpoch.OutputsMerkleRoot,
		common.HexToHash("0xf003"),
	)
	foreignEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000002")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent}, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 0, transitions)
}

func TestSubmitClaimWithAntecessorMismatch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)

	// event has an incorrect LastProcessedBlockNumber field. Every other
	// field matches the epoch so the mismatch is unambiguously LastBlock.
	prevEvent := &iconsensus.IConsensusClaimSubmitted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevEpoch.LastBlock + 1),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *prevEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        testMachineHash(prevEpoch),
	}
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).
		Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).
		Once()
	r.On("UpdateApplicationStatus", mock.Anything, int64(0), model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

// !claimMatchesEvent(currClaim, currEvent)
func TestSubmitClaimWithEventMismatch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	prevEvent := makeSubmittedEvent(app, prevEpoch)
	wrongEvent := makeSubmittedEventWithRoots(
		app,
		currEpoch,
		common.HexToHash("0xbad1"),
		common.HexToHash("0xbad2"),
	)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{prevEvent, wrongEvent}, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, int64(0), model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

func TestQuorumPreviousSubmittedEventsIgnoresForeignMismatchAndSubmitsCurrentClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	prevEpoch := makeAcceptedEpoch(app, 2)
	currEpoch := makeComputedEpoch(app, 3)
	foreignPrevEvent := makeSubmittedEventWithRoots(
		app,
		prevEpoch,
		*prevEpoch.OutputsMerkleRoot,
		common.HexToHash("0xf003"),
	)
	foreignPrevEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000002")
	matchingPrevEvent := makeSubmittedEvent(app, prevEpoch)
	matchingPrevEvent.Submitter = b.submitterAddress
	txHash := common.HexToHash("0x10")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignPrevEvent, matchingPrevEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Return(txHash, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, txHash, m.claimsInFlight[app.ID].txHash)
	assert.Equal(t, 1, transitions)
}

func TestQuorumPreviousSubmittedEventsOwnMismatchSetsDiverged(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	prevEpoch := makeAcceptedEpoch(app, 2)
	currEpoch := makeComputedEpoch(app, 3)
	wrongPrevEvent := makeSubmittedEventWithRoots(
		app,
		prevEpoch,
		*prevEpoch.OutputsMerkleRoot,
		common.HexToHash("0xf003"),
	)
	wrongPrevEvent.Submitter = b.submitterAddress

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{wrongPrevEvent}, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	currEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), currEpochs, makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, 0, len(currEpochs))
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 0, transitions)
}

// !checkClaimsConstraint(prevClaim, currClaim) // epoch pair has its blocks out of order
func TestSubmitClaimWithAntecessorOutOfOrder(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	app := makeApplication()
	prevEpoch := makeSubmittedEpoch(app, 2)
	currEpoch := makeComputedEpoch(app, 1)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, uint64(0))
	r.On("UpdateApplicationStatus", mock.Anything, int64(0), model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), big.NewInt(0))
	assert.Error(t, err)
}

func TestCheckEpochSequenceConstraintAllowsAcceptedPredecessorWithoutClaimTransactionHash(t *testing.T) {
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	prevEpoch.ClaimTransactionHash = nil
	currEpoch := makeComputedEpoch(app, 2)

	require.NoError(t, checkEpochSequenceConstraint(prevEpoch, currEpoch))
}

func TestErrSubmittedMissingEvent(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeComputedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 2)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, int64(0), model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

func TestConsensusAddressChangedOnSubmittedClaims(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	wrongConsensusAddress := app.IConsensusAddress
	wrongConsensusAddress[0]++

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(wrongConsensusAddress, nil).
		Once()
	r.On("UpdateApplicationStatus", mock.Anything, int64(0), model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

func TestCheckConsensusForAddressChangeUsesTickBlock(t *testing.T) {
	m, _, b := newServiceMock(t)
	defer b.AssertExpectations(t)

	app := makeApplication()
	tickBlock := big.NewInt(123)

	b.On("getConsensusAddress", mock.Anything, app, mock.MatchedBy(func(blockNumber *big.Int) bool {
		return blockNumber != nil && blockNumber.Cmp(tickBlock) == 0
	})).
		Return(app.IConsensusAddress, nil).
		Once()

	err := m.checkConsensusForAddressChange(context.Background(), app, tickBlock)
	require.NoError(t, err)
}

func TestCheckConsensusForAddressChangeCachesTickResult(t *testing.T) {
	m, _, b := newServiceMock(t)
	defer b.AssertExpectations(t)

	app := makeApplication()
	tickBlock := big.NewInt(123)
	m.consensusAddressChecks = map[consensusAddressCheckKey]error{}

	b.On("getConsensusAddress", mock.Anything, app, tickBlock).
		Return(app.IConsensusAddress, nil).
		Once()

	ctx := context.Background()
	err := m.checkConsensusForAddressChange(ctx, app, tickBlock)
	require.NoError(t, err)
	err = m.checkConsensusForAddressChange(ctx, app, tickBlock)
	require.NoError(t, err)
}

func TestSubmitClaimTimeout(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionTimeout = 100 * time.Millisecond

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Run(func(args mock.Arguments) {
			ctx := args.Get(0).(context.Context)
			select {
			case <-ctx.Done():
				assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
			case <-time.After(2 * time.Second):
				assert.Fail(t, "context provided did not have the expected timeout")
			}
		}).
		Return(common.Hash{}, context.DeadlineExceeded).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(t.Context(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 0, transitions, "submitting a claim counts as a transition")
}

func TestSubmitClaimContextCanceled(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.submissionTimeout = 2 * time.Second

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, mock.Anything, app, currEpoch).
		Run(func(args mock.Arguments) {
			ctx := args.Get(0).(context.Context)
			select {
			case <-ctx.Done():
				assert.ErrorIs(t, ctx.Err(), context.Canceled)
			case <-time.After(2 * time.Second):
				assert.Fail(t, "context provided was not canceled")
			}
		}).
		Return(common.Hash{}, context.Canceled).Once()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	transitions, err := m.submitClaimsAndUpdateDatabase(ctx, makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, len(m.claimsInFlight))
	assert.Equal(t, 0, transitions, "submitting a claim counts as a transition")
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"
	"strings"
	"testing"

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

	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	require.Nil(t, m.publisher, "observation owns no publication queue")
	assert.Zero(t, transitions, "planning is not durable progress")
}

func TestSubmitClaimRejectsIncompletePersistedStateProofBeforeBroadcast(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	currEpoch.HtifTohostProof = currEpoch.HtifTohostProof[:model.StateProofSiblingCount-1]
	expectPreSubmitPath(b, app, currEpoch, endBlock)
	// Incomplete persisted proof must not become a publication candidate.
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted,
		mock.MatchedBy(func(reason *string) bool {
			return reason != nil && strings.Contains(*reason, "persisted machine state proof is incomplete")
		})).Return(nil).Once()

	currEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(
		t.Context(), currEpochs, makeApplicationMap(app), endBlock)

	require.Zero(t, transitions)
	require.ErrorContains(t, err, "persisted machine state proof is incomplete")
	require.Empty(t, currEpochs)
	require.Equal(t, model.ApplicationStatus_Corrupted, app.Status)
}

func TestSubmitClaimForeclosesUnstagedForeclosedApp(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := withForeclosed(makeApplication(), 35)
	currEpoch := makeComputedEpoch(app, 3)
	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()

	computedEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), computedEpochs, makeApplicationMap(app), endBlock)

	assert.NoError(t, err, "foreclosing an impossible claim is not an error")
	assert.Equal(t, 1, transitions, "CLAIM_FORECLOSED is a local status transition")
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
}

func TestSubmitClaimForeclosesUnstagedForeclosedAppWhenSubmissionDisabled(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionEnabled = false
	endBlock := big.NewInt(40)
	app := withForeclosed(makeApplication(), 35)
	currEpoch := makeComputedEpoch(app, 3)
	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
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
func TestSubmitClaimReconcilesAcceptedForForeclosedApp(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := withForeclosed(makeApplication(), 35)
	currEpoch := makeComputedEpoch(app, 3)
	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()
	// Chain returns ACCEPTED (status 2) — the reconcile-before-submit
	// path mirrors this to the local DB and skips broadcast entirely.
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusAccepted, currEpoch, 0), nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index, mock.Anything).
		Return(nil).Once()

	computedEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), computedEpochs, makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions, "ACCEPTED reconciliation counts as a transition")
}

func TestSubmitClaimReconcilesStagedBeforeBroadcast(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	var currEvent *iconsensus.IConsensusClaimSubmitted
	stagedAt := currEpoch.LastBlock + 2

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	r.On("UpdateEpochReconciledStaged", mock.Anything, app.ID, currEpoch.Index, stagedAt).
		Return(nil).Once()

	computedEpochs := makeEpochMap(currEpoch)
	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), computedEpochs, makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions, "STAGED reconciliation counts as a transition")
	assert.Empty(t, computedEpochs, "reconciled epoch must leave the computed work map")
}

func TestReconcileBeforeSubmitAcceptedOutputsMismatchSetsDiverged(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()
	claim := makeClaimStatus(claimStatusAccepted, currEpoch, 0)
	claim.StagedOutputsMerkleRoot = common.HexToHash("0xdeadbeef")
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(claim, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.Error(t, err)
	assert.Equal(t, 0, transitions)
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

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	require.Nil(t, m.publisher, "observation owns no publication queue")
	assert.Zero(t, transitions, "planning is not durable progress")
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
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{prevEvent, currEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)

	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	require.NoError(t, err)
	require.Nil(t, m.publisher, "observation owns no publication queue")
	assert.Zero(t, transitions, "planning is not durable progress")
}

func TestPlanSubmitClaimWithStagedAntecessor(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeStagedEpoch(app, 1, 25)
	currEpoch := makeComputedEpoch(app, 3)

	var currEvent *iconsensus.IConsensusClaimSubmitted

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()

	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)

	transitions, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 0, transitions, "a planned submission is not durable observer progress")
}

func TestSkipSubmitFirstClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionEnabled = false
	endBlock := big.NewInt(40)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)

	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
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

	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
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

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
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

	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, nil, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.TxHash).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
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

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
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

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Zero(t, transitions, "planning is not durable progress")
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

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Zero(t, transitions, "planning is not durable progress")
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

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
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
		*currEpoch.TxBufferDataBlock,
		common.HexToHash("0xf003"),
	)
	adversarialEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000003")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent, adversarialEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Zero(t, transitions, "planning is not durable progress")
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
		*currEpoch.TxBufferDataBlock,
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
	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), currEpochs, makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, 0, len(currEpochs))
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
		*currEpoch.TxBufferDataBlock,
		common.HexToHash("0xf003"),
	)
	foreignEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000002")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignEvent}, nil).Once()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, transitions)
}

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
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{prevEvent, wrongEvent}, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, int64(0), model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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
		*prevEpoch.TxBufferDataBlock,
		common.HexToHash("0xf003"),
	)
	foreignPrevEvent.Submitter = common.HexToAddress("0x0000000000000000000000000000000000000002")
	matchingPrevEvent := makeSubmittedEvent(app, prevEpoch)
	matchingPrevEvent.Submitter = b.submitterAddress

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, prevEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{foreignPrevEvent, matchingPrevEvent}, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Zero(t, transitions, "planning is not durable progress")
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

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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

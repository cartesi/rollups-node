// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAcceptFirstClaim(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeSubmittedEpoch(app, 3)
	var prevEvent *iconsensus.IConsensusClaimAccepted = nil
	currEvent := makeAcceptedEvent(app, currEpoch)
	ctx := context.Background()

	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(ctx, makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
}

func TestAcceptClaimWithAntecessor(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeSubmittedEpoch(app, 3)
	prevEvent := makeAcceptedEvent(app, prevEpoch)
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index, mock.Anything).
		Return(nil).Once()

	transitions, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 1, transitions, "accepting a claim counts as a transition")
}

// //////////////////////////////////////////////////////////////////////////////
// Failure

func TestFindClaimAcceptedEventAndSuccFailure0(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	expectedErr := fmt.Errorf("not found")
	endBlock := big.NewInt(100)

	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 2)
	var prevEvent *iconsensus.IConsensusClaimAccepted = nil
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, expectedErr).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

func TestFindClaimAcceptedEventAndSuccFailure1(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	expectedErr := fmt.Errorf("not found")
	endBlock := big.NewInt(100)

	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 2)
	prevEvent := makeAcceptedEvent(app, prevEpoch)
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, expectedErr).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

// !claimAcceptedMatch(prevClaim, prevEvent)
func TestAcceptClaimWithAntecessorMismatch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)

	// Every field matches the epoch except LastProcessedBlockNumber.
	prevEvent := &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(prevEpoch.LastBlock + 1),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *prevEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        testMachineHash(prevEpoch),
	}
	var currEvent *iconsensus.IConsensusClaimAccepted = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil)
	r.On("UpdateApplicationStatus", mock.Anything, mock.Anything, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

// !claimAcceptedMatch(currClaim, currEvent)
func TestAcceptClaimWithEventMismatch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	wrongEpoch := makeComputedEpoch(app, 2)
	currEpoch := makeComputedEpoch(app, 3)
	wrongEvent := makeAcceptedEvent(app, wrongEpoch)
	prevEvent := makeAcceptedEvent(app, prevEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, wrongEvent, nil)
	r.On("UpdateApplicationStatus", mock.Anything, mock.Anything, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

// !checkClaimsConstraint(prevClaim, currClaim)
func TestAcceptClaimWithAntecessorOutOfOrder(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	app := makeApplication()
	wrongEpoch := makeComputedEpoch(app, 2)
	currEpoch := makeComputedEpoch(app, 1)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, mock.Anything, model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).
		Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(wrongEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), big.NewInt(0))
	assert.Error(t, err)
}

func TestErrAcceptedMissingEvent(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeComputedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 2)
	var prevEvent *iconsensus.IConsensusClaimAccepted = nil
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, mock.Anything, model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

func TestUpdateEpochWithAcceptedClaimFailed(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	expectedErr := fmt.Errorf("not found")

	endBlock := big.NewInt(100)
	app := makeApplication()
	prevEpoch := makeSubmittedEpoch(app, 1)
	currEpoch := makeSubmittedEpoch(app, 2)
	prevEvent := makeAcceptedEvent(app, prevEpoch)
	currEvent := makeAcceptedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, prevEpoch, prevEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index, mock.Anything).
		Return(expectedErr).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

func TestConsensusAddressChangedOnAcceptedClaims(t *testing.T) {
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
		Return(nil).
		Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
}

func TestAcceptStagedFrontRunner(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusAccepted, currEpoch, stagedAt), nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index, mock.Anything).
		Return(nil).Once()

	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

func TestAcceptStagedBroadcastsWhenClaimStillStaged(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0xabc")
	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	b.On("acceptClaimOnBlockchain", mock.Anything, app, currEpoch).
		Return(txHash, nil).Once()

	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, transitions, "broadcasting acceptClaim records in-flight work but does not update DB yet")

	got, ok := m.acceptsInFlight[app.ID]
	require.True(t, ok)
	assert.Equal(t, txHash, got.txHash)
	assert.Equal(t, endBlock.Uint64(), got.firstSeenBlock)
	assert.Equal(t, uint64(1), m.acceptAttempts[acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}])
}

func TestAcceptStagedFrontRunnerOutputsMismatchSetsDiverged(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	claim := makeClaimStatus(claimStatusAccepted, currEpoch, stagedAt)
	claim.StagedOutputsMerkleRoot = common.HexToHash("0xdeadbeef")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(claim, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, 0, transitions)
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

// TestAcceptStagedUnmodeledClaimStatusFailsClosed verifies the open-enum guard
// on the getClaim path: a ClaimStatus this node does not model (here 3 — the
// enum is 0/1/2) must escalate the app to FAILED rather than skip silently
// every tick. No acceptClaim is broadcast.
func TestAcceptStagedUnmodeledClaimStatusFailsClosed(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(3, currEpoch, stagedAt), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
		Return(nil).Once()

	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, transitions)
	// SetFailedf returns nil on success; the FAILED write is asserted by the mock.
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.acceptsInFlight), "no acceptClaim broadcast on an unmodeled status")
}

func TestAcceptStagedForeclosesForeclosedApp(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionEnabled = false
	endBlock := big.NewInt(52)
	app := withForeclosed(makeApplication(), 51)
	app.ClaimStagingPeriod = 100
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	// Chain reports STAGED (status 1) — non-foreclosed apps would
	// fall through to acceptClaimOnBlockchain. Foreclosed apps must not.
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()
	// CRITICAL: no acceptClaimOnBlockchain expectation — testify reports
	// an unexpected call if the guard fails.

	ctx := context.Background()
	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(
		ctx, makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
	assert.Equal(t, 0, len(m.acceptsInFlight),
		"no acceptClaim should enter the in-flight set for a foreclosed app")
}

// TestAcceptStagedForeclosesForeclosedAppOnUnstaged verifies that a foreclosed
// app whose chain claim reads UNSTAGED terminalizes the epoch to
// CLAIM_FORECLOSED rather than falling into the generic UNSTAGED handling that
// would mark the app FAILED (and never DIVERGED/CORRUPTED). The foreclosure
// guard fires before any health-status write, so the drain is not stuck behind
// a re-enable loop.
func TestAcceptStagedForeclosesForeclosedAppOnUnstaged(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(52)
	app := withForeclosed(makeApplication(), 51)
	app.ClaimStagingPeriod = 100
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	// Chain reports UNSTAGED (status 0). A non-foreclosed app would be marked
	// FAILED here; a foreclosed app must terminalize the epoch instead.
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusUnstaged, currEpoch, stagedAt), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()
	// CRITICAL: no UpdateApplicationStatus expectation — testify reports an
	// unexpected call if the foreclosure guard fails and any FAILED/DIVERGED/
	// CORRUPTED write is attempted.

	ctx := context.Background()
	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(
		ctx, makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
	assert.Equal(t, model.ApplicationStatus_OK, app.Status,
		"foreclosure terminalization must not change application health status")
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

// TestAcceptStagedCapEnforced — after maxAcceptAttempts consecutive attempts
// to call acceptClaim, the next entry into the per-epoch budget exhausts it
// and the app is marked FAILED without another broadcast.
func TestAcceptStagedCapEnforced(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	// Prime the counter to exactly the cap — the next attempt must trip it.
	m.acceptAttempts[acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}] = m.maxAcceptAttempts

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	// No call to acceptClaimOnBlockchain — the cap stops it.
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.Anything).
		Return(nil).Once()

	_, err := m.acceptStagedClaimsAndIssueAcceptTx(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	// SetFailedf returns nil on success — no error surfaced.
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.acceptsInFlight))
	// Counter cleared once FAILED is set.
	_, present := m.acceptAttempts[acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}]
	assert.False(t, present)
}

func TestAcceptStagedUnknownBroadcastErrorsIncrementAttemptsUntilCap(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	attemptKey := acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}
	broadcastErr := fmt.Errorf("gas estimation failed")
	ctx := context.Background()

	for i := uint64(1); i <= m.maxAcceptAttempts; i++ {
		b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
			Return(app.IConsensusAddress, nil).Once()
		b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
			Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
		b.On("acceptClaimOnBlockchain", mock.Anything, app, currEpoch).
			Return(common.Hash{}, broadcastErr).Once()

		transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(ctx, makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

		assert.Equal(t, 0, transitions)
		assert.ErrorIs(t, err, broadcastErr)
		assert.Equal(t, i, m.acceptAttempts[attemptKey])
		assert.Equal(t, 0, len(m.acceptsInFlight))
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.MatchedBy(func(reason *string) bool {
		return reason != nil && strings.Contains(*reason, "acceptClaim has failed")
	})).
		Return(nil).Once()

	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(ctx, makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.Equal(t, 0, transitions)
	assert.NoError(t, err, "marking FAILED after the cap is a state transition outcome, not a tick error")
	assert.Equal(t, model.ApplicationStatus_Failed, app.Status)
	assert.NotContains(t, m.acceptAttempts, attemptKey)
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

func TestAcceptClaimNotStagedAcceptedRechecksOutputsMismatch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	mismatch := makeClaimStatus(claimStatusAccepted, currEpoch, stagedAt)
	mismatch.StagedOutputsMerkleRoot = common.HexToHash("0xdeadbeef")

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	b.On("acceptClaimOnBlockchain", mock.Anything, app, currEpoch).
		Return(common.Hash{}, claimNotStagedError(claimStatusAccepted)).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(mismatch, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	ctx := context.Background()
	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(ctx, makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, 0, transitions)
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

// TestAcceptStagedPeriodNotElapsed — current block too low; no tx issued.
func TestAcceptStagedPeriodNotElapsed(t *testing.T) {
	m, _, b := newServiceMock(t)
	defer b.AssertExpectations(t)

	app := makeApplication()
	app.ClaimStagingPeriod = 100
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()

	endBlock := big.NewInt(60) // only 10 blocks elapsed; need 100.
	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, transitions)
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

// TestAcceptStagedReaderMode — submissionEnabled=false; no acceptClaim tx
// is ever issued even when the period has elapsed. Caller waits for
// someone else to call acceptClaim (observed via the ClaimAccepted scan).
func TestAcceptStagedReaderMode(t *testing.T) {
	m, _, b := newServiceMock(t)
	defer b.AssertExpectations(t)
	m.submissionEnabled = false

	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	endBlock := big.NewInt(100)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()

	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, transitions)
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

// TestAcceptanceDivergence_QuorumStagedDoesNotRejectEpoch verifies that a
// divergent accepted claim observed after our claim is already staged halts the
// app without rewriting the epoch to CLAIM_REJECTED. Under Quorum this is an
// invariant violation, not the normal outvoted path.
func TestAcceptanceDivergence_QuorumStagedDoesNotRejectEpoch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	divergent := &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(currEpoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *currEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        common.HexToHash("0xbad"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.MatchedBy(func(reason *string) bool {
		return reason != nil && strings.Contains(*reason, "quorum_divergence_at_acceptance")
	})).
		Return(nil).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	assert.Equal(t, model.EpochStatus_ClaimStaged, currEpoch.Status)
}

func TestAcceptanceDivergence_QuorumComputedRejectsEpoch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)

	divergent := &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(currEpoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *currEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        common.HexToHash("0xbad"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
	r.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, currEpoch.Index, mock.MatchedBy(func(reason string) bool {
		return strings.Contains(reason, "quorum_divergence_at_acceptance")
	})).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	assert.Equal(t, model.EpochStatus_ClaimRejected, currEpoch.Status)
}

func TestAcceptanceDivergence_AuthorityComputedSetsDivergedWithoutRejectingEpoch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)

	divergent := &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(currEpoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *currEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        common.HexToHash("0xbad"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.MatchedBy(func(reason *string) bool {
		return reason != nil && strings.Contains(*reason, "authority_divergence_at_acceptance")
	})).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	assert.Equal(t, model.EpochStatus_ClaimComputed, currEpoch.Status)
}

func TestAcceptanceDivergence_AuthorityDoesNotRejectEpoch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	divergent := &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(currEpoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *currEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        common.HexToHash("0xbad"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	assert.Equal(t, model.EpochStatus_ClaimStaged, currEpoch.Status)
}

// TestStagingDivergenceReaderMode_Quorum — reader-mode parity: with
// submissionEnabled=false, a divergent ClaimStaged event still fires the
// same DIVERGED transition as in submit mode. No tx is ever issued (the
// stage's broadcast path is unconditionally skipped, so we don't even need

func TestAcceptanceDivergenceReaderMode_Quorum(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)
	m.submissionEnabled = false

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	divergent := &iconsensus.IConsensusClaimAccepted{
		LastProcessedBlockNumber: new(big.Int).SetUint64(currEpoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *currEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        common.HexToHash("0xbad"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.MatchedBy(func(reason *string) bool {
		return reason != nil && strings.Contains(*reason, "quorum_divergence_at_acceptance")
	})).
		Return(nil).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err, "acceptance divergence detection must fire in reader mode")
	assert.Equal(t, model.EpochStatus_ClaimStaged, currEpoch.Status)
}

// TestHandleAcceptClaimRevert — exhaustive dispatch matrix for the typed
// reverts handleAcceptClaimRevert recognises. The classifier never mutates

func TestAcceptClaimTimeout(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	m.submissionTimeout = 100 * time.Millisecond

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	attemptKey := acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	b.On("acceptClaimOnBlockchain", mock.Anything, app, currEpoch).
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

	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(t.Context(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.Equal(t, 0, transitions)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, uint64(1), m.acceptAttempts[attemptKey])
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

func TestAcceptClaimContextCanceled(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.submissionTimeout = 2 * time.Second

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ClaimStagingPeriod = 5
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	attemptKey := acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	b.On("acceptClaimOnBlockchain", mock.Anything, app, currEpoch).
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

	transitions, err := m.acceptStagedClaimsAndIssueAcceptTx(ctx, makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.Equal(t, 0, transitions)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, uint64(1), m.acceptAttempts[attemptKey])
	assert.Equal(t, 0, len(m.acceptsInFlight))
}

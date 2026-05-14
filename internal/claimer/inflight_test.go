// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInFlightCompleted(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication() // default: Authority consensus
	currEpoch := makeComputedEpoch(app, 3)
	currEpoch.ClaimTransactionHash = &txHash

	m.claimsInFlight[app.ID] = inFlightTx{txHash: *currEpoch.ClaimTransactionHash}

	// v3 Authority emits ClaimSubmitted + ClaimStaged in the same tx. The
	// staging fast-path captures this and records COMPUTED → SUBMITTED →
	// STAGED atomically via UpdateEpochThroughStaging.
	stagedLog := makeClaimStagedLog(app, currEpoch)
	receiptBlock := uint64(currEpoch.LastBlock + 1)
	stagedLog.BlockNumber = receiptBlock
	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			ContractAddress: app.IApplicationAddress,
			TxHash:          txHash,
			BlockNumber:     new(big.Int).SetUint64(receiptBlock),
			Status:          1,
			Logs:            []*types.Log{&stagedLog},
		}, nil).Once()
	r.On("UpdateEpochThroughStaging", mock.Anything, app.ID, currEpoch.Index, txHash, receiptBlock).
		Return(nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
	// v3 fast path: submitted (1) + staged (1) = 2 transitions.
	assert.Equal(t, 2, transitions)
}

// TestInFlightCompleted_QuorumNonDeciding — variant where the submit tx
// confirmed but the receipt does NOT contain a ClaimStaged log (Quorum,
// non-deciding vote). tryStageFromReceipt returns stageReceiptNoMatch; the
// caller falls back to UpdateEpochWithSubmittedClaim. Epoch transitions
// COMPUTED → SUBMITTED (not STAGED).
func TestInFlightCompleted_QuorumNonDeciding(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeComputedEpoch(app, 3)
	currEpoch.ClaimTransactionHash = &txHash

	m.claimsInFlight[app.ID] = inFlightTx{txHash: *currEpoch.ClaimTransactionHash}

	receiptBlock := uint64(currEpoch.LastBlock + 1)
	// Quorum non-deciding submit: receipt has Status=1 but no ClaimStaged log.
	// The submitClaim emits ClaimSubmitted, but tryStageFromReceipt only
	// scans for ClaimStaged — so the log list can be empty here without
	// affecting the assertion.
	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			ContractAddress: app.IApplicationAddress,
			TxHash:          txHash,
			BlockNumber:     new(big.Int).SetUint64(receiptBlock),
			Status:          1,
			Logs:            []*types.Log{},
		}, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, txHash).
		Return(nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, len(errs))
	assert.Equal(t, 0, len(m.claimsInFlight))
	// Fall-back path: one transition (COMPUTED → SUBMITTED), not the fast-path's two.
	assert.Equal(t, 1, transitions)
}

func TestInFlightReverted(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	currEpoch.ClaimTransactionHash = &txHash

	var prevEvent *iconsensus.IConsensusClaimSubmitted = nil
	var currEvent *iconsensus.IConsensusClaimSubmitted = nil

	m.claimsInFlight[app.ID] = inFlightTx{txHash: *currEpoch.ClaimTransactionHash}

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			ContractAddress: app.IApplicationAddress,
			TxHash:          txHash,
			BlockNumber:     new(big.Int).SetUint64(currEpoch.LastBlock + 1),
			Status:          0,
		}, nil).Once()
	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, prevEvent, currEvent, nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, app, currEpoch).
		Return(common.HexToHash("0x10"), nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 0)
	assert.Equal(t, len(m.claimsInFlight), 1)
}

func TestClaimInFlightMissingFromCurrClaims(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	reqHash := common.HexToHash("0x01")
	receipt := new(types.Receipt)

	app := makeApplication()
	m.claimsInFlight[app.ID] = inFlightTx{txHash: reqHash}

	b.On("pollTransaction", mock.Anything, reqHash, endBlock).
		Return(true, receipt, nil).Once()

	_, errs := m.submitClaimsAndUpdateDatabase(makeEpochMap(), makeEpochMap(), makeApplicationMap(app), endBlock)
	assert.Equal(t, len(errs), 0)
}

func TestClaimInFlightPollErrorKeepsTrackingAndStopsDuplicateSubmit(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	expectedErr := fmt.Errorf("temporary receipt RPC failure")
	endBlock := big.NewInt(100)
	reqHash := common.HexToHash("0x01")
	var nilReceipt *types.Receipt

	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)

	m.claimsInFlight[app.ID] = inFlightTx{txHash: reqHash}

	b.On("pollTransaction", mock.Anything, reqHash, endBlock).
		Return(false, nilReceipt, expectedErr).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	require.Equal(t, 1, len(errs))
	assert.ErrorIs(t, errs[0], expectedErr)
	assert.Equal(t, 0, transitions)
	assert.Contains(t, m.claimsInFlight, app.ID,
		"receipt lookup errors do not prove the tx failed; keep in-flight tracking")
}

func TestClaimInFlightPollErrorsDoNotStopOtherApps(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	err1 := fmt.Errorf("temporary receipt RPC failure 1")
	err2 := fmt.Errorf("temporary receipt RPC failure 2")
	endBlock := big.NewInt(100)
	tx1 := common.HexToHash("0x01")
	tx2 := common.HexToHash("0x02")
	var nilReceipt *types.Receipt

	app1 := makeApplication()
	app2 := makeApplication()
	app2.ID = app1.ID + 1
	epoch1 := makeComputedEpoch(app1, 3)
	epoch2 := makeComputedEpoch(app2, 4)

	m.claimsInFlight[app1.ID] = inFlightTx{txHash: tx1}
	m.claimsInFlight[app2.ID] = inFlightTx{txHash: tx2}

	b.On("pollTransaction", mock.Anything, tx1, endBlock).
		Return(false, nilReceipt, err1).Once()
	b.On("pollTransaction", mock.Anything, tx2, endBlock).
		Return(false, nilReceipt, err2).Once()

	transitions, err := m.checkClaimsInFlight(makeEpochMap(epoch1, epoch2), makeApplicationMap(app1, app2), endBlock)
	require.Error(t, err)
	assert.ErrorIs(t, err, err1)
	assert.ErrorIs(t, err, err2)
	assert.Equal(t, 0, transitions)
	assert.Contains(t, m.claimsInFlight, app1.ID)
	assert.Contains(t, m.claimsInFlight, app2.ID)
}

func TestClaimInFlightReceiptNotFoundBeforeTimeoutKeepsTrackingAndStopsDuplicateSubmit(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	reqHash := common.HexToHash("0x01")
	var nilReceipt *types.Receipt

	app := makeApplication()
	prevEpoch := makeAcceptedEpoch(app, 1)
	currEpoch := makeComputedEpoch(app, 3)
	m.claimsInFlight[app.ID] = inFlightTx{
		txHash:         reqHash,
		firstSeenBlock: endBlock.Uint64() - maxInFlightReceiptNotFoundBlocks + 1,
	}

	b.On("pollTransaction", mock.Anything, reqHash, endBlock).
		Return(false, nilReceipt, nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(
		makeEpochMap(prevEpoch), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	require.Empty(t, errs)
	assert.Equal(t, 0, transitions)
	assert.Contains(t, m.claimsInFlight, app.ID,
		"receipt NotFound before timeout still means the tx may be pending")
}

func TestClaimInFlightReceiptNotFoundAfterTimeoutClearsAndRetries(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	oldTxHash := common.HexToHash("0x01")
	newTxHash := common.HexToHash("0x10")
	var nilReceipt *types.Receipt

	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	m.claimsInFlight[app.ID] = inFlightTx{
		txHash:         oldTxHash,
		firstSeenBlock: endBlock.Uint64() - maxInFlightReceiptNotFoundBlocks,
	}

	b.On("pollTransaction", mock.Anything, oldTxHash, endBlock).
		Return(false, nilReceipt, nil).Once()
	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted(nil), nil).Once()
	expectGetClaimStatusUnstaged(b, app, currEpoch, endBlock)
	b.On("submitClaimToBlockchain", mock.Anything, app, currEpoch).
		Return(newTxHash, nil).Once()

	transitions, errs := m.submitClaimsAndUpdateDatabase(makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	require.Empty(t, errs)
	assert.Equal(t, 1, transitions, "stale in-flight tx should allow the normal submit path to retry")
	got, ok := m.claimsInFlight[app.ID]
	require.True(t, ok)
	assert.Equal(t, newTxHash, got.txHash)
	assert.Equal(t, endBlock.Uint64(), got.firstSeenBlock)
}

func TestAcceptInFlightPollErrorKeepsTracking(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	expectedErr := fmt.Errorf("temporary receipt RPC failure")
	var nilReceipt *types.Receipt

	m.acceptsInFlight[app.ID] = inFlightTx{txHash: txHash}

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(false, nilReceipt, expectedErr).Once()

	transitions, err := m.checkAcceptsInFlight(makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 0, transitions)
	assert.Contains(t, m.acceptsInFlight, app.ID,
		"receipt lookup errors do not prove the tx failed; keep in-flight tracking")
}

func TestAcceptInFlightErrorsDoNotStopOtherAppsOrDropPollErrors(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	pollErr := fmt.Errorf("temporary receipt RPC failure")
	updateErr := fmt.Errorf("temporary DB update failure")
	endBlock := big.NewInt(100)
	tx1 := common.HexToHash("0x10")
	tx2 := common.HexToHash("0x20")
	stagedAt := uint64(50)
	var nilReceipt *types.Receipt

	app1 := makeApplication()
	app2 := makeApplication()
	app2.ID = app1.ID + 1
	epoch1 := makeStagedEpoch(app1, 3, stagedAt)
	epoch2 := makeStagedEpoch(app2, 4, stagedAt)

	m.acceptsInFlight[app1.ID] = inFlightTx{txHash: tx1}
	m.acceptsInFlight[app2.ID] = inFlightTx{txHash: tx2}

	b.On("pollTransaction", mock.Anything, tx1, endBlock).
		Return(false, nilReceipt, pollErr).Once()
	b.On("pollTransaction", mock.Anything, tx2, endBlock).
		Return(true, &types.Receipt{
			TxHash:      tx2,
			Status:      1,
			BlockNumber: big.NewInt(99),
		}, nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app2.ID, epoch2.Index, (*common.Hash)(nil)).
		Return(updateErr).Once()

	transitions, err := m.checkAcceptsInFlight(makeEpochMap(epoch1, epoch2), makeApplicationMap(app1, app2), endBlock)
	require.Error(t, err)
	assert.ErrorIs(t, err, pollErr)
	assert.ErrorIs(t, err, updateErr)
	assert.Equal(t, 0, transitions)
	assert.Contains(t, m.acceptsInFlight, app1.ID)
	assert.Contains(t, m.acceptsInFlight, app2.ID)
}

func TestAcceptInFlightReceiptNotFoundAfterTimeoutClearsTracking(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	var nilReceipt *types.Receipt

	m.acceptsInFlight[app.ID] = inFlightTx{
		txHash:         txHash,
		firstSeenBlock: endBlock.Uint64() - maxInFlightReceiptNotFoundBlocks,
	}

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(false, nilReceipt, nil).Once()

	transitions, err := m.checkAcceptsInFlight(makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	require.NoError(t, err)
	assert.Equal(t, 0, transitions)
	assert.NotContains(t, m.acceptsInFlight, app.ID,
		"stale receipt NotFound should unblock the next accept lifecycle pass")
}

func TestAcceptInFlightSuccessUpdatesEpochAndClearsTracking(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	attemptKey := acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}
	stagedEpochs := makeEpochMap(currEpoch)

	m.acceptsInFlight[app.ID] = inFlightTx{txHash: txHash}
	m.acceptAttempts[attemptKey] = 2

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			TxHash:      txHash,
			Status:      1,
			BlockNumber: big.NewInt(99),
		}, nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index, (*common.Hash)(nil)).
		Return(nil).Once()

	transitions, err := m.checkAcceptsInFlight(stagedEpochs, makeApplicationMap(app), endBlock)
	require.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.NotContains(t, m.acceptsInFlight, app.ID)
	assert.NotContains(t, m.acceptAttempts, attemptKey)
	assert.Empty(t, stagedEpochs, "accepted epoch must leave the staged work map")
}

func TestAcceptInFlightRevertedAcceptedReconcilesEpoch(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)
	attemptKey := acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}
	stagedEpochs := makeEpochMap(currEpoch)

	m.acceptsInFlight[app.ID] = inFlightTx{txHash: txHash}
	m.acceptAttempts[attemptKey] = 2

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			TxHash:      txHash,
			Status:      0,
			BlockNumber: big.NewInt(99),
		}, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusAccepted, currEpoch, stagedAt), nil).Once()
	r.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, currEpoch.Index, (*common.Hash)(nil)).
		Return(nil).Once()

	transitions, err := m.checkAcceptsInFlight(stagedEpochs, makeApplicationMap(app), endBlock)
	require.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.NotContains(t, m.acceptsInFlight, app.ID)
	assert.NotContains(t, m.acceptAttempts, attemptKey)
	assert.Empty(t, stagedEpochs, "front-run accepted epoch must leave the staged work map")
}

func TestAcceptInFlightRevertedUnstagedMarksApplicationFailed(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	m.acceptsInFlight[app.ID] = inFlightTx{txHash: txHash}

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			TxHash:      txHash,
			Status:      0,
			BlockNumber: big.NewInt(99),
		}, nil).Once()
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusUnstaged, currEpoch, 0), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed, mock.MatchedBy(func(reason *string) bool {
		return reason != nil && strings.Contains(*reason, "DB inconsistent with chain")
	})).
		Return(nil).Once()

	transitions, err := m.checkAcceptsInFlight(makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	require.NoError(t, err)
	assert.Equal(t, 0, transitions)
	assert.Equal(t, model.ApplicationStatus_Failed, app.Status)
	assert.NotContains(t, m.acceptsInFlight, app.ID)
}

// TestAcceptInFlightRevertedForeclosedTerminalizes is the in-flight twin of
// TestAcceptStagedForeclosesForeclosedAppOnUnstaged: when an in-flight accept
// tx reverts and the app is foreclosed, the epoch must terminalize to
// CLAIM_FORECLOSED (not FAILED) and the app health must stay OK. This pins the
// symmetric C1 guard on the in-flight path (inflight.go:311), mirroring the
// pre-accept guard on the accept path.
func TestAcceptInFlightRevertedForeclosedTerminalizes(t *testing.T) {
	m, r, b := newServiceMock()
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := withForeclosed(makeApplication(), 60)
	stagedAt := uint64(50)
	currEpoch := makeStagedEpoch(app, 3, stagedAt)

	m.acceptsInFlight[app.ID] = inFlightTx{txHash: txHash}

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{TxHash: txHash, Status: 0, BlockNumber: big.NewInt(99)}, nil).Once()
	// Chain reports a non-ACCEPTED status (UNSTAGED). A non-foreclosed app would
	// be marked FAILED here; a foreclosed app must terminalize the epoch instead.
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusUnstaged, currEpoch, 0), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()
	// CRITICAL: no UpdateApplicationStatus expectation — any FAILED/DIVERGED/
	// CORRUPTED write trips the mock as an unexpected call.

	transitions, err := m.checkAcceptsInFlight(makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	require.NoError(t, err)
	assert.Equal(t, 1, transitions, "terminalization counts as completed work")
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
	assert.Equal(t, model.ApplicationStatus_OK, app.Status,
		"foreclosure terminalization must not change application health status")
	assert.NotContains(t, m.acceptsInFlight, app.ID)
}

func TestCleanupOrphanedInFlight(t *testing.T) {
	m, _, _ := newServiceMock()

	liveApp := makeApplication() // ID = 0
	stagedApp := repotest.NewApplicationBuilder().
		WithName("staged-app").Build()
	stagedApp.ID = 1
	stagedEpoch := makeStagedEpoch(stagedApp, 7, 50)

	// Live app: kept in computedApps. Its entry must survive.
	m.claimsInFlight[liveApp.ID] = inFlightTx{txHash: common.HexToHash("0xaa")}

	// Orphan app: not in any work map. Its entries must be dropped.
	const orphanAppID int64 = 99
	m.claimsInFlight[orphanAppID] = inFlightTx{txHash: common.HexToHash("0xbb")}
	m.acceptsInFlight[orphanAppID] = inFlightTx{txHash: common.HexToHash("0xcc")}
	m.acceptAttempts[acceptAttemptKey{orphanAppID, 3}] = 4

	// Staged app present but for a different epoch — old counter must be dropped.
	m.acceptsInFlight[stagedApp.ID] = inFlightTx{txHash: common.HexToHash("0xdd")}
	m.acceptAttempts[acceptAttemptKey{stagedApp.ID, stagedEpoch.Index}] = 2
	m.acceptAttempts[acceptAttemptKey{stagedApp.ID, stagedEpoch.Index - 1}] = 9

	m.cleanupOrphanedInFlight(
		makeApplicationMap(liveApp),
		makeApplicationMap(stagedApp),
		makeEpochMap(stagedEpoch),
	)

	_, liveOK := m.claimsInFlight[liveApp.ID]
	assert.True(t, liveOK, "live app's submit-in-flight must be kept")

	_, orphanSubmit := m.claimsInFlight[orphanAppID]
	assert.False(t, orphanSubmit, "orphan submit-in-flight must be dropped")
	_, orphanAccept := m.acceptsInFlight[orphanAppID]
	assert.False(t, orphanAccept, "orphan accept-in-flight must be dropped")
	_, orphanAttempts := m.acceptAttempts[acceptAttemptKey{orphanAppID, 3}]
	assert.False(t, orphanAttempts, "orphan accept-attempt counter must be dropped")

	_, stagedAccept := m.acceptsInFlight[stagedApp.ID]
	assert.True(t, stagedAccept, "live staged app's accept-in-flight must be kept")
	_, currentCounter := m.acceptAttempts[acceptAttemptKey{stagedApp.ID, stagedEpoch.Index}]
	assert.True(t, currentCounter, "live staged app's current-epoch counter must be kept")
	_, oldCounter := m.acceptAttempts[acceptAttemptKey{stagedApp.ID, stagedEpoch.Index - 1}]
	assert.False(t, oldCounter, "counter for a non-current epoch on the same app must be dropped")
}

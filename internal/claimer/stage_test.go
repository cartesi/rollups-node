// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestStagingFastPathDivergence(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	currEpoch.ClaimTransactionHash = &txHash
	m.claimsInFlight[app.ID] = inFlightTx{txHash: txHash}

	// Build a divergent ClaimStaged log: same (app, lpbn, outputs) but
	// different machineMerkleRoot.
	divergent := makeStagedEvent(app, currEpoch)
	differentMachineMerkleRoot := common.HexToHash("0xdeadbeef")
	divergent.MachineMerkleRoot = differentMachineMerkleRoot
	stagedLog := buildClaimStagedLog(app, currEpoch, *currEpoch.OutputsMerkleRoot, differentMachineMerkleRoot)
	receiptBlock := currEpoch.LastBlock + 1

	b.On("pollTransaction", mock.Anything, txHash, endBlock).
		Return(true, &types.Receipt{
			TxHash:      txHash,
			BlockNumber: new(big.Int).SetUint64(receiptBlock),
			Status:      1,
			Logs:        []*types.Log{&stagedLog},
		}, nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	// The fast-path consumed the receipt and triggered DIVERGED. The
	// divergence error is surfaced (matching the convention used by other
	// terminal-status setters);
	// UpdateEpochThroughStaging is NOT called and the in-flight tx is dropped.
	assert.Error(t, err, "divergence at staging fast-path must surface as an error")
	assert.Equal(t, 0, len(m.claimsInFlight))
}

// TestStagingFastPathDBPending — happy fast-path match but the atomic
// UpdateEpochThroughStaging write fails. The fix must NOT fall back to
// UpdateEpochWithSubmittedClaim (which would hide the STAGED event from
// this tick's pipeline so the next tick's staging scan would have to
// re-discover it from chain — surface signal goes silent under correlated
// DB outages). Instead it surfaces the error and leaves the in-flight
// tracking + computedEpochs entry intact so the next tick polls the
// receipt again and retries the atomic write.
func TestStagingFastPathDBPending(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	txHash := common.HexToHash("0x10")
	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	currEpoch.ClaimTransactionHash = &txHash
	m.claimsInFlight[app.ID] = inFlightTx{txHash: txHash}

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
	dbErr := fmt.Errorf("statement timeout")
	r.On("UpdateEpochThroughStaging", mock.Anything, app.ID, currEpoch.Index, txHash, receiptBlock).
		Return(dbErr).Once()
	// No UpdateEpochWithSubmittedClaim expectation — falling back to a
	// plain SUBMITTED update would lose the staged-at-block atomicity
	// that UpdateEpochThroughStaging guarantees in a single transaction.

	computedEpochs := makeEpochMap(currEpoch)
	_, err := m.submitClaimsAndUpdateDatabase(
		context.Background(), makeEpochMap(), computedEpochs, makeApplicationMap(app), endBlock)

	assert.ErrorIs(t, err, dbErr, "DB-pending must surface as a tick-level error")
	// Both work-tracking entries must remain so the next tick can retry
	// from the same receipt.
	assert.Contains(t, m.claimsInFlight, app.ID,
		"claimsInFlight must be retained so the next tick polls the receipt again")
	assert.Contains(t, computedEpochs, app.ID,
		"computedEpochs entry must be retained for cleanupOrphanedInFlight")
}

// buildClaimStagedLog builds a types.Log for a ClaimStaged event with

func TestStageByObservation(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeSubmittedEpoch(app, 3)
	currEvent := makeStagedEvent(app, currEpoch)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, currEvent, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("UpdateEpochToStaged", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.BlockNumber).
		Return(nil).Once()

	transitions, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
}

func TestStageForeclosesSubmittedForeclosedApp(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := withForeclosed(makeApplication(), 80)
	currEpoch := makeSubmittedEpoch(app, 3)

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, (*iconsensus.IConsensusClaimStaged)(nil), (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()

	transitions, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
}

// TestStagingDivergence_Quorum — Quorum case where ClaimStaged is observed
// with a machineMerkleRoot != ours → CLAIM_REJECTED and DIVERGED with
// quorum_divergence_at_staging.
func TestStagingDivergence_Quorum(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeSubmittedEpoch(app, 3)

	// Divergent event: different machineMerkleRoot.
	differentMachineMerkleRoot := common.HexToHash("0xfeed")
	divergent := &iconsensus.IConsensusClaimStaged{
		LastProcessedBlockNumber: new(big.Int).SetUint64(currEpoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *currEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        differentMachineMerkleRoot,
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, currEpoch.Index, mock.MatchedBy(func(reason string) bool {
		return strings.Contains(reason, "quorum_divergence_at_staging")
	})).
		Return(nil).Once()

	_, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	assert.Equal(t, model.EpochStatus_ClaimRejected, currEpoch.Status)
}

func TestStagingDivergence_AuthorityDoesNotRejectEpoch(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeSubmittedEpoch(app, 3)

	divergent := &iconsensus.IConsensusClaimStaged{
		LastProcessedBlockNumber: new(big.Int).SetUint64(currEpoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *currEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        common.HexToHash("0xfeed"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	assert.Equal(t, model.EpochStatus_ClaimSubmitted, currEpoch.Status)
}

func TestStagingMatcherPreconditionFailureMarksApplicationCorrupted(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	endBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeSubmittedEpoch(app, 3)
	event := makeStagedEvent(app, currEpoch)
	currEpoch.MachineHash = nil

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, event, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted, mock.MatchedBy(func(reason *string) bool {
		return reason != nil && strings.Contains(*reason, "cannot compare epoch")
	})).
		Return(nil).Once()

	_, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, model.ApplicationStatus_Corrupted, app.Status)
}

// TestAcceptStagedFrontRunner — staging period elapsed; pre-flight getClaim
// returns ACCEPTED (status=2) before our acceptClaim → reconcile to

func TestStagingDivergenceReaderMode_Quorum(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)
	m.submissionEnabled = false

	endBlock := big.NewInt(100)
	app := makeApplication()
	app.ConsensusType = model.Consensus_Quorum
	currEpoch := makeSubmittedEpoch(app, 3)

	differentMachineMerkleRoot := common.HexToHash("0xfeed")
	divergent := &iconsensus.IConsensusClaimStaged{
		LastProcessedBlockNumber: new(big.Int).SetUint64(currEpoch.LastBlock),
		AppContract:              app.IApplicationAddress,
		OutputsMerkleRoot:        *currEpoch.OutputsMerkleRoot,
		MachineMerkleRoot:        differentMachineMerkleRoot,
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, currEpoch.Index, mock.MatchedBy(func(reason string) bool {
		return strings.Contains(reason, "quorum_divergence_at_staging")
	})).
		Return(nil).Once()

	_, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err, "divergence detection must fire in reader mode")
	assert.Equal(t, model.EpochStatus_ClaimRejected, currEpoch.Status)
}

// TestAcceptanceDivergenceReaderMode_Quorum — reader-mode parity for the
// acceptance stage. submissionEnabled doesn't gate event-based divergence
// detection; the DIVERGED transition must fire identically, but a staged
// epoch remains CLAIM_STAGED because a different accepted claim is an invariant

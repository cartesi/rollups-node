// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

	transitions, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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

	transitions, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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
		OutputsMerkleRoot:        *currEpoch.TxBufferDataBlock,
		MachineMerkleRoot:        differentMachineMerkleRoot,
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, currEpoch.Index, mock.MatchedBy(func(reason string) bool {
		return strings.Contains(reason, "quorum_divergence_at_staging")
	})).
		Return(repository.RejectEpochAndDivergeResult{
			EpochRejected:       true,
			ApplicationDiverged: true,
		}, nil).Once()

	_, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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
		OutputsMerkleRoot:        *currEpoch.TxBufferDataBlock,
		MachineMerkleRoot:        common.HexToHash("0xfeed"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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

	_, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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
		OutputsMerkleRoot:        *currEpoch.TxBufferDataBlock,
		MachineMerkleRoot:        differentMachineMerkleRoot,
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimStagedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimStaged)(nil), nil).Once()
	r.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, currEpoch.Index, mock.MatchedBy(func(reason string) bool {
		return strings.Contains(reason, "quorum_divergence_at_staging")
	})).
		Return(repository.RejectEpochAndDivergeResult{
			EpochRejected:       true,
			ApplicationDiverged: true,
		}, nil).Once()

	_, err := m.stageClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err, "divergence detection must fire in reader mode")
	assert.Equal(t, model.EpochStatus_ClaimRejected, currEpoch.Status)
}

// TestAcceptanceDivergenceReaderMode_Quorum — reader-mode parity for the
// acceptance stage. submissionEnabled doesn't gate event-based divergence
// detection; the DIVERGED transition must fire identically, but a staged
// epoch remains CLAIM_STAGED because a different accepted claim is an invariant

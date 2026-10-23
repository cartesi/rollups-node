// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAcceptClaimUsesCurrentEpochIdentity(t *testing.T) {
	const (
		accepted      = "accepted"
		differentRoot = "different root"
		notVisible    = "not visible"
		otherEpoch    = "other epoch"
		readError     = "read error"
		writeError    = "write error"
	)
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		for _, mode := range []string{accepted, notVisible, otherEpoch, differentRoot, readError, writeError} {
			t.Run(consensus.String()+"/"+mode, func(t *testing.T) {
				s, repo, chain := newServiceMock(t)
				app := makeApplication()
				app.ConsensusType = consensus
				previous := makeStagedEpoch(app, 1, 25)
				current := makeStagedEpoch(app, 3, 45)
				event := makeAcceptedEvent(app, current)
				event.Raw.TxHash = common.HexToHash("0xabc")
				var readErr, writeErr error
				switch mode {
				case notVisible:
					event = nil
				case otherEpoch:
					event = makeAcceptedEvent(app, makeComputedEpoch(app, 2))
				case differentRoot:
					event.OutputsMerkleRoot = common.HexToHash("0xbad")
				case readError:
					readErr = errors.New("event provider unavailable")
				case writeError:
					writeErr = errors.New("database unavailable")
				}
				chain.On("getConsensusAddress", mock.Anything, app, mock.Anything).Return(app.IConsensusAddress, nil).Once()
				chain.On("findClaimAcceptedEventAndSucc", mock.Anything, app, current, current.LastBlock+1, uint64(100)).
					Return(&iconsensus.IConsensus{}, event, makeAcceptedEvent(app, previous), readErr).Once()
				if mode == accepted || mode == writeError {
					repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, current.Index, &event.Raw.TxHash).
						Return(writeErr).Once()
				}
				if mode == differentRoot {
					repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
						Return(nil).Once()
				}
				count, err := s.acceptClaimsAndUpdateDatabase(t.Context(), makeEpochMap(current),
					makeApplicationMap(app), big.NewInt(100))

				switch mode {
				case accepted:
					require.NoError(t, err)
					require.Equal(t, 1, count)
				case notVisible, otherEpoch:
					require.NoError(t, err)
					require.Zero(t, count)
				default:
					require.Error(t, err)
					require.Zero(t, count)
				}
				if mode != differentRoot {
					require.Equal(t, model.ApplicationStatus_OK, app.Status)
				}
				repo.AssertExpectations(t)
				chain.AssertExpectations(t)
			})
		}
	}
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

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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

	transitions, err := m.observeStagedClaims(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
}

func TestAcceptStagedBroadcastsWhenClaimStillStaged(t *testing.T) {
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
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()

	transitions, err := m.observeStagedClaims(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Zero(t, transitions, "planning is not durable progress")
	require.Nil(t, m.publisher, "observation does not construct publication state")
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

	transitions, err := m.observeStagedClaims(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err)
	assert.Equal(t, 0, transitions)
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

	transitions, err := m.observeStagedClaims(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Equal(t, 0, transitions)
	// SetFailedf returns nil on success; the FAILED write is asserted by the mock.
	assert.NoError(t, err)
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
	// produce an accept candidate. Foreclosed apps must not.
	b.On("getClaimStatus", mock.Anything, app, currEpoch, endBlock).
		Return(makeClaimStatus(claimStatusStaged, currEpoch, stagedAt), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, currEpoch.Index).
		Return(nil).Once()

	ctx := context.Background()
	transitions, err := m.observeStagedClaims(
		ctx, makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
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
	transitions, err := m.observeStagedClaims(
		ctx, makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)

	assert.NoError(t, err)
	assert.Equal(t, 1, transitions)
	assert.Equal(t, model.EpochStatus_ClaimForeclosed, currEpoch.Status)
	assert.Equal(t, model.ApplicationStatus_OK, app.Status,
		"foreclosure terminalization must not change application health status")
}

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
	transitions, err := m.observeStagedClaims(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, transitions)
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

	transitions, err := m.observeStagedClaims(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.NoError(t, err)
	assert.Equal(t, 0, transitions)
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
		OutputsMerkleRoot:        *currEpoch.TxBufferDataBlock,
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

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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
		OutputsMerkleRoot:        *currEpoch.TxBufferDataBlock,
		MachineMerkleRoot:        common.HexToHash("0xbad"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
	r.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, currEpoch.Index, mock.MatchedBy(func(reason string) bool {
		return strings.Contains(reason, "quorum_divergence_at_acceptance")
	})).
		Return(repository.RejectEpochAndDivergeResult{
			EpochRejected:       true,
			ApplicationDiverged: true,
		}, nil).Once()

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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
		OutputsMerkleRoot:        *currEpoch.TxBufferDataBlock,
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

	_, err := m.submitClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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
		OutputsMerkleRoot:        *currEpoch.TxBufferDataBlock,
		MachineMerkleRoot:        common.HexToHash("0xbad"),
	}

	b.On("getConsensusAddress", mock.Anything, app, mock.Anything).
		Return(app.IConsensusAddress, nil).Once()
	b.On("findClaimAcceptedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, endBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, divergent, (*iconsensus.IConsensusClaimAccepted)(nil), nil).Once()
	r.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Diverged, mock.Anything).
		Return(nil).Once()

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
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
		OutputsMerkleRoot:        *currEpoch.TxBufferDataBlock,
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

	_, err := m.acceptClaimsAndUpdateDatabase(context.Background(), makeEpochMap(currEpoch), makeApplicationMap(app), endBlock)
	assert.Error(t, err, "acceptance divergence detection must fire in reader mode")
	assert.Equal(t, model.EpochStatus_ClaimStaged, currEpoch.Status)
}

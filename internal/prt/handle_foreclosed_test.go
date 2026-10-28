// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"log/slog"
	"math/big"
	"testing"
)

func prtForeclosedApp(id int64, block uint64) *model.Application {
	txHash := common.HexToHash("0xcafe")
	return &model.Application{
		ID:                   id,
		Name:                 "prt-app",
		IApplicationAddress:  common.BigToAddress(common.Big1),
		ConsensusType:        model.Consensus_PRT,
		Enabled:              true,
		Status:               model.ApplicationStatus_OK,
		ForecloseBlock:       block,
		ForecloseTransaction: &txHash,
		// Both DaveConsensus observation cursors default to the foreclose
		// block so callers that don't care about the bootstrap guard skip it.
		// Tests that exercise the guard override one cursor explicitly.
		LastEpochCheckBlock: block,
		LastInputCheckBlock: block,
	}
}

func TestOrdinaryApplicationObservationDoesNotRunForeclosureDrain(t *testing.T) {
	f := newDeliveryFixture(t)
	view := resultTestSnapshot(f.epoch, false)
	opts := mock.MatchedBy(resultCallOptsAtBlock(100))
	f.consensus.On("GetCurrentSealedEpoch", opts).Return(view.sealed, nil)
	f.consensus.On("CanStageTournamentResult", opts).Return(view.stage, nil)
	f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(view.accept, nil)
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, mock.Anything, false).Return([]*model.Epoch{}, uint64(0), nil)
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, observerFacts(100)))
	f.repo.AssertNotCalled(t, "HasUndrainedEpochsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
}

func TestObserveRetainsTerminalApplicationStatus(t *testing.T) {
	statuses := []model.ApplicationStatus{
		model.ApplicationStatus_Failed, model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted,
	}
	for _, status := range statuses {
		t.Run(status.String(), func(t *testing.T) {
			f, client := tickFixture(t)
			f.app.Status = status
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, mock.Anything, false).Return([]*model.Epoch{}, uint64(0), nil)
			f.repo.publishedRoots = nil
			tickHead(client, 110)
			require.NoError(t, f.d.observe(t.Context()))
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock, "empty observation does not invent coverage")
			require.Equal(t, status, f.app.Status)
			require.Empty(t, f.repo.windows)
			require.Empty(t, f.backend.published)
		})
	}
}

func TestForeclosureDrainDefersNewerSavedForeclosure(t *testing.T) {
	f := newDeliveryFixture(t)
	f.app.ForecloseBlock = 120
	f.app.LastEpochCheckBlock, f.app.LastInputCheckBlock = 120, 120
	f.repo.publishedRoots = nil
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, mock.Anything, false).
		Return([]*model.Epoch{}, uint64(0), nil).Once()

	err := f.d.observeApplication(t.Context(), f.app, observerFacts(100))
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	f.repo.AssertNotCalled(t, "HasUndrainedEpochsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertExpectations(t)
}

// TestHandleForeclosedApp_DefersWhenUndrained verifies the
// pre-foreclosure-work guard. While the advancer/validator have epochs to
// process before the foreclose block, the PRT app must keep its current
// status. Marking it terminal early would lose the last machine state needed
// to process its last pre-foreclosure epoch.
func TestHandleForeclosedApp_DefersWhenUndrained(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()
	// No UpdateApplicationStatus expectation — see TestProcessForeclosedApps_DefersWhenUndrained
	// in the claimer suite for the equivalent reasoning.

	require.NoError(t, s.drainForeclosedClaims(context.Background(), app, nil, nil, 120))
}

// A completed local drain must not stop passive tournament observation. With
// no roots, this pass only queries eligibility and checks the existing gates.
func TestHandleForeclosedApp_ObservesAfterLocalDrain(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()

	require.NoError(t, s.drainForeclosedClaims(context.Background(), app, nil, nil, 120))
}

// TestHandleForeclosedApp_SurfacesDrainCheckError verifies the surrounding
// behavior on transient repository failures: the error must propagate so
// the Tick's err slice marks the app as in trouble; the app keeps its current
// status for retry on the next tick.
func TestHandleForeclosedApp_SurfacesDrainCheckError(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("connection refused")
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, dbErr).Once()

	err := s.drainForeclosedClaims(context.Background(), app, nil, nil, 120)
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

// TestHandleForeclosedApp_DefersWhenStillBackfilling verifies the
// bootstrap-readiness guard. When a freshly registered PRT app encounters
// an already-foreclosed contract, evmreader sets ForecloseBlock before
// checkForEpochsAndInputs has ingested any historical sealed epochs. The
// drain gate would then see an empty input table and incorrectly return
// false, making the app look drained before any pre-foreclosure epoch is
// observed locally. The guard must defer the drain check until
// both DaveConsensus observation cursors reach ForecloseBlock.
//
// The mock has no HasUndrainedEpochsBeforeBlock or UpdateApplicationStatus
// expectation registered; testify/mock panics on an unexpected call, so
// either reach attempt fails the test loudly.
func TestHandleForeclosedApp_DefersWhenStillBackfilling(t *testing.T) {
	for _, test := range []struct {
		name        string
		epochCursor uint64
		inputCursor uint64
	}{
		{name: "sealed epochs still landing", epochCursor: 50, inputCursor: 100},
		{name: "same-block open inputs still landing", epochCursor: 100, inputCursor: 99},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, r := newPRTServiceMock()
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			app := prtForeclosedApp(1, 100)
			app.LastEpochCheckBlock = test.epochCursor
			app.LastInputCheckBlock = test.inputCursor

			require.NoError(t, s.drainForeclosedClaims(t.Context(), app, nil, nil, 120))
			require.Contains(t, output.String(), "sealed epochs and inputs")
			require.Contains(t, output.String(), "last_epoch_check_block="+new(big.Int).SetUint64(test.epochCursor).String())
			require.Contains(t, output.String(), "last_input_check_block="+new(big.Int).SetUint64(test.inputCursor).String())
			require.Contains(t, output.String(), "foreclose_block=100")
			r.AssertNotCalled(t, "HasUndrainedEpochsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
			r.AssertNotCalled(t, "HasUnreconciledClaimsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
			r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
			r.AssertExpectations(t)
		})
	}
}

// TestHandleForeclosedApp_SurfacesReconciliationCheckError verifies the
// epoch-level completion gate's error propagates so the Tick retries on the
// next pass and the app keeps its status meanwhile.
func TestHandleForeclosedApp_SurfacesReconciliationCheckError(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("connection refused")
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, dbErr).Once()

	err := s.drainForeclosedClaims(context.Background(), app, nil, nil, 120)
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

// TestHandleForeclosedApp_LeavesClaimedEpochForNextReconciliationPass models
// an epoch that appears between the observation query and the local drain
// query. An on-chain EpochSealed transaction hash requires reconciliation on
// the next pass; the local drain must not mark that epoch CLAIM_FORECLOSED.
func TestHandleForeclosedApp_LeavesClaimedEpochForNextReconciliationPass(t *testing.T) {
	s, r := newPRTServiceMock()
	app := prtForeclosedApp(1, 100)
	claimTx := common.HexToHash("0xbeef")
	r.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(true, nil).Once()
	r.On("ListEpochs", mock.Anything, app.Name,
		repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}, repository.Pagination{}, false).
		Return([]*model.Epoch{{Index: 1, ClaimTransactionHash: &claimTx}}, uint64(1), nil).Once()
	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, nil, nil, 120))
	r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(1))
	r.AssertExpectations(t)
}

// Foreclosure terminalizes only epochs whose claim path is finished; earlier
// INPUTS_PROCESSED epochs still require validation, and sealed observations stay
// available for acceptance reconciliation.
func TestForeclosePendingClaimEpochsPreservesValidationAndSealedClaims(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	claimTx := common.HexToHash("0xbeef")
	r.On("ListEpochs",
		mock.Anything, app.Name,
		repository.EpochFilter{Status: []model.EpochStatus{
			model.EpochStatus_Open, model.EpochStatus_Closed, model.EpochStatus_InputsProcessed,
			model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged,
		}},
		repository.Pagination{}, false,
	).Return([]*model.Epoch{
		{Index: 0, Status: model.EpochStatus_Open, FirstBlock: 100, LastBlock: 150},
		{Index: 1, Status: model.EpochStatus_Closed},
		{Index: 2, Status: model.EpochStatus_InputsProcessed},
		{Index: 3},
		{Index: 4, Status: model.EpochStatus_ClaimStaged},
		{Index: 5, ClaimTransactionHash: &claimTx},
		{Index: 6, Status: model.EpochStatus_ClaimSubmitted},
		{Index: 7, Status: model.EpochStatus_Open, FirstBlock: 101},
	}, uint64(8), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(0)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(1)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(3)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(4)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(6)).Return(nil).Once()

	require.NoError(t, s.foreclosePendingClaimEpochs(context.Background(), app))
	r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(2))
	r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(5))
	r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(7))
}

// TestForeclosePendingClaimEpochs_PropagatesUpdateError verifies a failed
// terminalization surfaces so the Tick retries rather than silently dropping
// a still-non-terminal epoch.
func TestForeclosePendingClaimEpochs_PropagatesUpdateError(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("write failed")
	r.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{{Index: 7}}, uint64(1), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(7)).Return(dbErr).Once()

	err := s.foreclosePendingClaimEpochs(context.Background(), app)
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

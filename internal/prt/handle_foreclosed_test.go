// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
		// LastEpochCheckBlock defaults to the foreclose block so callers
		// who don't care about the bootstrap guard skip past it. Tests
		// that exercise the guard override this field explicitly.
		LastEpochCheckBlock: block,
	}
}

// TestHandleForeclosedApp_NoOpWhenForecloseBlockZero verifies the guard at
// the top of handleForeclosedApp. The PRT Tick passes every running app
// through this function; only those with a non-zero ForecloseBlock should
// drive any work.
func TestHandleForeclosedApp_NoOpWhenForecloseBlockZero(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := &model.Application{ID: 1, ConsensusType: model.Consensus_PRT}
	require.NoError(t, s.handleForeclosedApp(context.Background(), app))
}

func TestGetAllRunningApplications_UsesPRTTickFilter(t *testing.T) {
	r := &prtRepositoryMock{}
	r.On("ListApplications",
		mock.Anything,
		mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Enabled != nil && *f.Enabled &&
				f.ConsensusType != nil && *f.ConsensusType == model.Consensus_PRT &&
				assert.ElementsMatch(t,
					[]model.ApplicationStatus{model.ApplicationStatus_OK},
					f.Statuses,
				)
		}),
		repository.Pagination{},
		false,
	).Return([]*model.Application{}, uint64(0), nil).Once()

	_, _, err := getAllRunningApplications(context.Background(), r)
	require.NoError(t, err)
	r.AssertExpectations(t)
}

// TestHandleForeclosedApp_DefersWhenUndrained verifies the
// pre-foreclosure-work guard. While the advancer/validator have epochs to
// process before the foreclose block, the PRT app must keep its current
// status. Marking it terminal early would lose the last machine state needed
// to settle any in-flight tournament.
func TestHandleForeclosedApp_DefersWhenUndrained(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()
	// No UpdateApplicationStatus expectation — see TestProcessForeclosedApps_DefersWhenUndrained
	// in the claimer suite for the equivalent reasoning.

	require.NoError(t, s.handleForeclosedApp(context.Background(), app))
}

// TestHandleForeclosedApp_NoOpWhenFullyDrained verifies that once every
// pre-foreclosure epoch is terminal, handleForeclosedApp is a no-op: it does
// not reconcile (no chain reads), does not foreclose, and does not touch the
// application status. The app keeps health status OK with foreclose_block set;
// evmreader picks up the post-foreclosure observation work from here.
//
// The mock registers no UpdateApplicationStatus / ListEpochs /
// UpdateEpochWithForeclosedClaim expectation; testify/mock fails the test on an
// unexpected call, so any regression that re-runs drain work on an already
// terminal app trips this test loudly.
func TestHandleForeclosedApp_NoOpWhenFullyDrained(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()

	require.NoError(t, s.handleForeclosedApp(context.Background(), app))
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

	err := s.handleForeclosedApp(context.Background(), app)
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
// LastEpochCheckBlock >= ForecloseBlock.
//
// The mock has no HasUndrainedEpochsBeforeBlock or UpdateApplicationStatus
// expectation registered; testify/mock panics on an unexpected call, so
// either reach attempt fails the test loudly.
func TestHandleForeclosedApp_DefersWhenStillBackfilling(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	app.LastEpochCheckBlock = 50 // scanner is well below the foreclose block

	require.NoError(t, s.handleForeclosedApp(context.Background(), app))
}

// TestHandleForeclosedApp_ProceedsAfterBackfillCatchesUp verifies the
// guard does not over-defer. Once LastEpochCheckBlock reaches the
// foreclose block, the gate is consulted normally; on a "drained=false"
// response the function returns nil silently (no terminal action — see
// TestHandleForeclosedApp_NoTransitionWhenDrained).
func TestHandleForeclosedApp_ProceedsAfterBackfillCatchesUp(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	app.LastEpochCheckBlock = app.ForecloseBlock // exact-boundary case: caught up

	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()

	require.NoError(t, s.handleForeclosedApp(context.Background(), app))
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

	err := s.handleForeclosedApp(context.Background(), app)
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

// TestHandleForeclosedApp_LeavesClaimedEpochForNextReconciliationPass models
// the race where checkEpochs takes its CLAIM_COMPUTED snapshot before the
// validator computes a later epoch, but forecloseComputedEpochs sees that later
// epoch in the same tick. If the epoch already has an on-chain EpochSealed
// transaction hash, it must stay CLAIM_COMPUTED for the next reconciliation
// pass instead of being terminalized to CLAIM_FORECLOSED.
func TestHandleForeclosedApp_LeavesClaimedEpochForNextReconciliationPass(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	client := &ethClientMock{}
	client.On("BlockNumber", mock.Anything).Return(uint64(120), nil).Once()
	s.client = client
	claimTx := common.HexToHash("0xbeef")

	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()
	r.On("ListEpochs",
		mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false,
	).Return([]*model.Epoch{}, uint64(0), nil).Once()
	r.On("ListEpochs",
		mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false,
	).Return([]*model.Epoch{
		{Index: 1, ClaimTransactionHash: &claimTx},
	}, uint64(1), nil).Once()

	require.NoError(t, s.handleForeclosedApp(context.Background(), app))
	r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(1))
	client.AssertExpectations(t)
}

// TestForecloseComputedEpochs_TerminalizesEachComputedEpoch verifies the
// foreclose step: every CLAIM_COMPUTED epoch without an on-chain sealed-event
// transaction is transitioned to CLAIM_FORECLOSED.
func TestForecloseComputedEpochs_TerminalizesEachComputedEpoch(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	claimTx := common.HexToHash("0xbeef")
	r.On("ListEpochs",
		mock.Anything, app.Name,
		mock.MatchedBy(func(f repository.EpochFilter) bool {
			return len(f.Status) == 1 && f.Status[0] == model.EpochStatus_ClaimComputed
		}),
		repository.Pagination{}, false,
	).Return([]*model.Epoch{
		{Index: 3},
		{Index: 4},
		{Index: 5, ClaimTransactionHash: &claimTx},
	}, uint64(3), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(3)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(4)).Return(nil).Once()

	require.NoError(t, s.forecloseComputedEpochs(context.Background(), app))
}

// TestForecloseComputedEpochs_PropagatesUpdateError verifies a failed
// terminalization surfaces so the Tick retries rather than silently dropping
// a still-non-terminal epoch.
func TestForecloseComputedEpochs_PropagatesUpdateError(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("write failed")
	r.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{{Index: 7}}, uint64(1), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(7)).Return(dbErr).Once()

	err := s.forecloseComputedEpochs(context.Background(), app)
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// prtRepositoryMock is a hand-written mock for the prtRepository interface,
// stubbing only the methods used by handleForeclosedApp. Unused methods
// keep zero-value Return signatures so the surface compiles; if a test
// accidentally invokes them, testify/mock reports an unexpected call.
type prtRepositoryMock struct {
	mock.Mock
}

func (m *prtRepositoryMock) HasUndrainedEpochsBeforeBlock(
	ctx context.Context, appID int64, blockBound uint64,
) (bool, error) {
	args := m.Called(ctx, appID, blockBound)
	return args.Bool(0), args.Error(1)
}

func (m *prtRepositoryMock) HasUnreconciledClaimsBeforeBlock(
	ctx context.Context, appID int64, blockBound uint64,
) (bool, error) {
	args := m.Called(ctx, appID, blockBound)
	return args.Bool(0), args.Error(1)
}

func (m *prtRepositoryMock) UpdateEpochWithForeclosedClaim(
	ctx context.Context, applicationID int64, index uint64,
) error {
	args := m.Called(ctx, applicationID, index)
	return args.Error(0)
}

func (m *prtRepositoryMock) UpdateApplicationStatus(
	ctx context.Context, appID int64, status model.ApplicationStatus, reason *string,
) error {
	args := m.Called(ctx, appID, status, reason)
	return args.Error(0)
}

// Unused-by-this-suite methods. We satisfy the interface but each panics
// loudly if invoked — handleForeclosedApp must not reach for them.
func (m *prtRepositoryMock) ListApplications(
	ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool,
) ([]*model.Application, uint64, error) {
	args := m.Called(ctx, f, p, descending)
	return args.Get(0).([]*model.Application), args.Get(1).(uint64), args.Error(2)
}
func (m *prtRepositoryMock) ListEpochs(
	ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination, descending bool,
) ([]*model.Epoch, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	return args.Get(0).([]*model.Epoch), args.Get(1).(uint64), args.Error(2)
}
func (m *prtRepositoryMock) GetEpoch(context.Context, string, uint64) (*model.Epoch, error) {
	panic("unexpected GetEpoch")
}
func (m *prtRepositoryMock) UpdateEpochStatus(context.Context, string, *model.Epoch) error {
	panic("unexpected UpdateEpochStatus")
}
func (m *prtRepositoryMock) CreateTournament(context.Context, string, *model.Tournament) error {
	panic("unexpected CreateTournament")
}
func (m *prtRepositoryMock) GetTournament(context.Context, string, string) (*model.Tournament, error) {
	panic("unexpected GetTournament")
}
func (m *prtRepositoryMock) UpdateTournament(context.Context, string, *model.Tournament) error {
	panic("unexpected UpdateTournament")
}
func (m *prtRepositoryMock) ListTournaments(
	context.Context, string, repository.TournamentFilter, repository.Pagination, bool,
) ([]*model.Tournament, uint64, error) {
	panic("unexpected ListTournaments")
}
func (m *prtRepositoryMock) StoreTournamentEvents(
	context.Context, int64, []*model.Commitment, []*model.Match,
	[]*model.MatchAdvanced, []*model.Match, uint64,
) error {
	panic("unexpected StoreTournamentEvents")
}
func (m *prtRepositoryMock) GetCommitment(context.Context, string, uint64, string, string) (*model.Commitment, error) {
	panic("unexpected GetCommitment")
}
func (m *prtRepositoryMock) SaveNodeConfigRaw(context.Context, string, []byte) error {
	panic("unexpected SaveNodeConfigRaw")
}
func (m *prtRepositoryMock) LoadNodeConfigRaw(context.Context, string) ([]byte, time.Time, time.Time, error) {
	panic("unexpected LoadNodeConfigRaw")
}

// newPRTServiceMock builds a minimal Service wired to a prtRepositoryMock.
// Only the fields handleForeclosedApp reaches for are populated.
func newPRTServiceMock() (*Service, *prtRepositoryMock) {
	repo := &prtRepositoryMock{}
	s := &Service{
		Service: service.Service{
			Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
		},
		repository: repo,
	}
	return s, repo
}

type prtEthClientMock struct {
	blockNumber uint64
}

func (m prtEthClientMock) BlockNumber(context.Context) (uint64, error) {
	return m.blockNumber, nil
}

func (m prtEthClientMock) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	panic("unexpected TransactionReceipt")
}

func (m prtEthClientMock) ChainID(context.Context) (*big.Int, error) {
	panic("unexpected ChainID")
}

func (m prtEthClientMock) TransactionByHash(context.Context, common.Hash) (*types.Transaction, bool, error) {
	panic("unexpected TransactionByHash")
}

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
	s.client = prtEthClientMock{blockNumber: 120}
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

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"os"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newForeclosureServiceFixture builds the smallest Service surface that
// checkForForeclosure / foreclosureSearchStartBlock reach for, plus the
// mocks bound to it. This avoids the full EvmReaderSuite bootstrap which
// wires up websocket clients, adapter factories, and tick-loop plumbing
// none of which are exercised by these unit tests.
//
// newMockApplicationContract pre-registers .Maybe() stubs for IsForeclosed
// and RetrieveForeclosureEvents (FIFO match). For foreclosure-path tests
// those defaults must be cleared so the per-test .On(...) expectations
// match — see the doc comment on newMockApplicationContract.
func newForeclosureServiceFixture(t *testing.T) (
	*Service, *MockApplicationContract, *MockRepository,
) {
	t.Helper()
	repo := newMockRepository()
	appContract := newMockApplicationContract()
	appContract.Unset("IsForeclosed")
	appContract.Unset("RetrieveForeclosureEvents")
	s := &Service{
		repository: repo,
	}
	require.NoError(t, service.InitTickServiceTemplate(
		&s.TickServiceTemplate,
		&service.TickServiceConfigs{
			BaseConfigs: service.BaseConfigs{
				Name:   "evm-reader",
				Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
			},
		},
		s,
	))
	return s, appContract, repo
}

// foreclosureAppContracts wraps an Application with the per-app contract
// adapter that checkForForeclosure consults.
func foreclosureAppContracts(app *Application, c *MockApplicationContract) appContracts {
	return appContracts{
		application:         app,
		applicationContract: c,
	}
}

// makeForeclosureEvent constructs a Foreclosure event with the given block
// and tx hash on the Raw log. The Foreclosure event body itself carries no
// fields (see ABI); only Raw.BlockNumber / Raw.TxHash are read by the
// observer.
func makeForeclosureEvent(block uint64, txHash common.Hash) *iapplication.IApplicationForeclosure {
	return &iapplication.IApplicationForeclosure{
		Raw: types.Log{BlockNumber: block, TxHash: txHash},
	}
}

// foreclosureTestApp builds an Application whose ForecloseBlock is zero
// (the "not yet observed" state). Unique address and ID keep mock
// assertions specific.
func foreclosureTestApp(id int64) *Application {
	return &Application{
		ID:                  id,
		Name:                "app",
		IApplicationAddress: common.BigToAddress(big.NewInt(id)),
		Status:              ApplicationStatus_OK,
	}
}

// ---------------------------------------------------------------------------
// checkForForeclosure
// ---------------------------------------------------------------------------

// TestCheckForForeclosure_SkipsWhenAlreadyRecorded verifies that the
// in-memory ForecloseBlock guard short-circuits the function: no on-chain
// reads, no DB write. This is the steady state for every foreclosed app
// after its first observation tick.
func TestCheckForForeclosure_SkipsWhenAlreadyRecorded(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	app.ForecloseBlock = 50

	// No mock expectations — any IsForeclosed / RetrieveForeclosureEvents /
	// UpdateApplicationForeclosure call would fail
	// testify's assertion.
	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, 100)
}

// TestCheckForForeclosure_SkipsWhenNotForeclosed verifies the common-case
// path: isForeclosed returns false, the function advances the cursor without
// filtering events.
func TestCheckForForeclosure_SkipsWhenNotForeclosed(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	const head = uint64(100)
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("IsForeclosed", mock.Anything).Return(false, nil).Once()
	repo.On("UpdateApplicationLastForecloseCheckBlock",
		mock.Anything, app.ID, head).Return(nil).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, head)

	assert.Zero(t, app.ForecloseBlock, "ForecloseBlock must remain zero")
	assert.Equal(t, head, app.LastForecloseCheckBlock)
}

// TestCheckForForeclosure_PersistsOnFirstObservation walks the happy path:
// isForeclosed=true, deployment block resolves, exactly one Foreclosure
// event is returned, the repository persists the (block, txHash) pair and
// cursor atomically, and the in-memory ForecloseBlock / ForecloseTransaction
// are populated so other code paths in this tick see the marker.
func TestCheckForForeclosure_PersistsOnFirstObservation(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	const head = uint64(100)
	const evBlock = uint64(80)
	txHash := common.HexToHash("0xfeed")

	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents",
		mock.Anything,
	).Return([]*iapplication.IApplicationForeclosure{
		makeForeclosureEvent(evBlock, txHash),
	}, nil).Once()
	repo.On("UpdateApplicationForeclosure",
		mock.Anything, app.ID, evBlock, txHash, head,
	).Return(nil).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, head)

	assert.Equal(t, evBlock, app.ForecloseBlock,
		"in-memory ForecloseBlock must be set so this tick's downstream "+
			"code sees the marker without re-reading the DB")
	if assert.NotNil(t, app.ForecloseTransaction) {
		assert.Equal(t, txHash, *app.ForecloseTransaction)
	}
	assert.Equal(t, head, app.LastForecloseCheckBlock)
}

// TestCheckForForeclosure_DoesNotAdvanceCursorWhenEventNotFound exercises an
// inconsistent RPC/log view where isForeclosed() is true but the matching
// Foreclosure log is absent from the search window. The function must leave
// both foreclose_block and last_foreclose_check_block unchanged so the next
// tick retries the same window.
func TestCheckForForeclosure_DoesNotAdvanceCursorWhenEventNotFound(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	const head = uint64(100)
	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents", mock.Anything).
		Return([]*iapplication.IApplicationForeclosure{}, nil).Once()
	// No atomic foreclosure marker write — the absence is the assertion.

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, head)

	assert.Zero(t, app.ForecloseBlock,
		"ForecloseBlock must remain zero so the next tick re-scans")
	assert.Zero(t, app.LastForecloseCheckBlock,
		"LastForecloseCheckBlock must remain unchanged so the same window is retried")
}

// TestCheckForForeclosure_SkipsAppOnIsForeclosedError verifies the per-app
// failure isolation: a transient RPC failure on one app must not prevent
// other apps in the same tick from being checked. Tested here by ensuring
// IsForeclosed-error leaves ForecloseBlock unset, with no DB write.
func TestCheckForForeclosure_SkipsAppOnIsForeclosedError(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	app.LastForecloseCheckBlock = 10
	c.On("IsForeclosed", mock.Anything).Return(false, errors.New("rpc dial")).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, 100)

	assert.Zero(t, app.ForecloseBlock)
}

// TestCheckForForeclosure_SkipsAppOnRetrieveError verifies the same
// isolation property for the event-filter call.
func TestCheckForForeclosure_SkipsAppOnRetrieveError(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents", mock.Anything).
		Return([]*iapplication.IApplicationForeclosure{}, errors.New("eth_getLogs failed")).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, 100)

	assert.Zero(t, app.ForecloseBlock)
}

// TestCheckForForeclosure_SkipsAppOnPersistError verifies the DB-error
// branch. The in-memory marker must NOT be set when the persist failed —
// otherwise the next tick would read a zero DB column but a non-zero
// in-memory marker, racing with restarts that drop the in-memory state.
func TestCheckForForeclosure_SkipsAppOnPersistError(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	const evBlock = uint64(80)
	txHash := common.HexToHash("0xfeed")

	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents", mock.Anything).
		Return([]*iapplication.IApplicationForeclosure{makeForeclosureEvent(evBlock, txHash)}, nil).Once()
	repo.On("UpdateApplicationForeclosure",
		mock.Anything, app.ID, evBlock, txHash, uint64(100),
	).Return(errors.New("db deadlock")).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, 100)

	assert.Zero(t, app.ForecloseBlock,
		"in-memory marker must not run ahead of the DB on persist failure")
}

// TestCheckForForeclosure_StopsOnContextCanceled verifies the early-exit
// on shutdown. IsForeclosed and RetrieveForeclosureEvents both check for
// context.Canceled and return immediately to avoid log-spam during the
// orchestrator's coordinated stop.
func TestCheckForForeclosure_StopsOnContextCanceled(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	app.LastForecloseCheckBlock = 10
	c.On("IsForeclosed", mock.Anything).Return(false, context.Canceled).Once()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.checkForForeclosure(ctx, []appContracts{foreclosureAppContracts(app, c)}, 100)
}

// TestCheckForForeclosure_AbortsLoopOnDeadlineExceeded verifies the
// deadline-exceeded short-circuit. Once the tick's context is past
// deadline every subsequent IsForeclosed call would fail the same way;
// surfacing one ERROR per app is wasted noise. The fix logs once at the
// site and aborts the loop, leaving recovery to the next tick — distinct
// from context.Canceled (silent) and other RPC errors (per-app log +
// continue), per the project's context-error-semantics convention.
func TestCheckForForeclosure_AbortsLoopOnDeadlineExceeded(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app1 := foreclosureTestApp(1)
	app1.LastForecloseCheckBlock = 10
	app2 := foreclosureTestApp(2)
	app2.LastForecloseCheckBlock = 10

	// Only app1's IsForeclosed call is expected. If the loop kept going,
	// app2's call would fail testify's "unexpected call" assertion —
	// that is the assertion this test relies on.
	c.On("IsForeclosed", mock.Anything).Return(false, context.DeadlineExceeded).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app1, c), foreclosureAppContracts(app2, c)}, 100)
}

// TestCheckForForeclosure_AbortsOnDeadlineExceededAtRetrieve mirrors the
// previous test for the second blocking RPC: even after IsForeclosed
// succeeds, a deadline during RetrieveForeclosureEvents on the first app
// must abort the loop rather than continuing to the second app.
func TestCheckForForeclosure_AbortsOnDeadlineExceededAtRetrieve(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app1 := foreclosureTestApp(1)
	app2 := foreclosureTestApp(2)

	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents", mock.Anything).
		Return([]*iapplication.IApplicationForeclosure{}, context.DeadlineExceeded).Once()
	// No expectations registered for app2 — an unexpected call fails the test.

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app1, c), foreclosureAppContracts(app2, c)}, 100)
}

// TestCheckForForeclosure_SkipsInMemoryMarkerOnErrNotFound verifies the
// ErrNotFound branch on the atomic foreclosure write. The row was deleted
// between the tick's ListApplications scan and this write; the caller
// must NOT populate app.ForecloseBlock / app.ForecloseTransaction
// because doing so would diverge from a DB row that no longer exists.
func TestCheckForForeclosure_SkipsInMemoryMarkerOnErrNotFound(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	const evBlock = uint64(80)
	txHash := common.HexToHash("0xfeed")

	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents", mock.Anything).
		Return([]*iapplication.IApplicationForeclosure{makeForeclosureEvent(evBlock, txHash)}, nil).Once()
	repo.On("UpdateApplicationForeclosure",
		mock.Anything, app.ID, evBlock, txHash, uint64(100),
	).Return(repository.ErrNotFound).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, 100)

	assert.Zero(t, app.ForecloseBlock,
		"ErrNotFound means the row is gone — in-memory marker must not be set")
	assert.Nil(t, app.ForecloseTransaction)
}

// TestCheckForForeclosure_SkipsBeforeDeploymentBlock verifies that a newly
// registered app does not query historical contract state at queued block
// headers from before the application contract existed.
func TestCheckForForeclosure_SkipsBeforeDeploymentBlock(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	const head = uint64(90)
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int), bind.ErrNoCode).Once()
	// No IsForeclosed expectation: querying it at block 90 would be an
	// eth_call against an address with no code.

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, head)

	assert.Zero(t, app.LastForecloseCheckBlock)
}

// TestCheckForForeclosure_SetsInMemoryMarkerOnIdempotentNil verifies the
// idempotent path: when the atomic foreclosure write returns nil for the
// "already foreclosed" case, the in-memory marker IS populated. The
// Foreclosure() event is one-way on chain so every observer derives the
// same (block, txHash); writing the marker is safe and lets other code
// paths in this tick see the foreclosure.
func TestCheckForForeclosure_SetsInMemoryMarkerOnIdempotentNil(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	const evBlock = uint64(80)
	txHash := common.HexToHash("0xfeed")

	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents", mock.Anything).
		Return([]*iapplication.IApplicationForeclosure{makeForeclosureEvent(evBlock, txHash)}, nil).Once()
	// nil for the idempotent "already foreclosed" path. The repository
	// contract distinguishes this from ErrNotFound.
	repo.On("UpdateApplicationForeclosure",
		mock.Anything, app.ID, evBlock, txHash, uint64(100),
	).Return(nil).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, 100)

	assert.Equal(t, evBlock, app.ForecloseBlock)
	if assert.NotNil(t, app.ForecloseTransaction) {
		assert.Equal(t, txHash, *app.ForecloseTransaction)
	}
}

// ---------------------------------------------------------------------------
// foreclosureSearchFloor
// ---------------------------------------------------------------------------

// TestForeclosureSearchFloor_ReturnsDeploymentBlock verifies the happy
// path: a positive deployment block is returned for the lower bound of
// the very first scan window.
func TestForeclosureSearchFloor_ReturnsDeploymentBlock(t *testing.T) {
	s, c, _ := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)

	app := foreclosureTestApp(1)
	ac := foreclosureAppContracts(app, c)
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(123), nil).Once()

	got, err := s.foreclosureSearchFloor(context.Background(), &ac, 200)
	require.NoError(t, err)
	assert.Equal(t, uint64(123), got)
}

// TestForeclosureSearchFloor_AcceptsDeploymentBlockZero verifies that a
// zero deployment block is accepted: anvil / genesis-snapshot fixtures can
// legitimately place contract code at block 0, and the previous defensive
// reject tripped on otherwise-valid devnet runs. A zero floor only widens
// the scan window; last_foreclose_check_block bounds it on the next tick.
func TestForeclosureSearchFloor_AcceptsDeploymentBlockZero(t *testing.T) {
	s, c, _ := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)

	app := foreclosureTestApp(1)
	ac := foreclosureAppContracts(app, c)
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(0), nil).Once()

	got, err := s.foreclosureSearchFloor(context.Background(), &ac, 200)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got)
}

// TestForeclosureSearchFloor_PropagatesRPCError verifies that the
// underlying RPC failure surfaces verbatim so the caller can log it with
// the right context.
func TestForeclosureSearchFloor_PropagatesRPCError(t *testing.T) {
	s, c, _ := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)

	app := foreclosureTestApp(1)
	ac := foreclosureAppContracts(app, c)
	rpcErr := errors.New("eth_call timeout")
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int), rpcErr).Once()

	_, err := s.foreclosureSearchFloor(context.Background(), &ac, 200)
	require.Error(t, err)
	assert.ErrorIs(t, err, rpcErr)
}

// ---------------------------------------------------------------------------
// LastForecloseCheckBlock advancement
// ---------------------------------------------------------------------------

// TestCheckForForeclosure_RetriesSameWindowWhenEventMissing pins the
// correctness contract: if isForeclosed() is true but no Foreclosure log is
// found, the scan cursor must not advance. The second tick therefore scans
// from the original deployment floor again, extending only the upper bound.
func TestCheckForForeclosure_RetriesSameWindowWhenEventMissing(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	const (
		head1 = uint64(100)
		head2 = uint64(110)
	)

	// First tick: LastForecloseCheckBlock==0, so the deployment block is read.
	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == 10 && opts.End != nil && *opts.End == head1
		}),
	).Return([]*iapplication.IApplicationForeclosure{}, nil).Once()

	// Second tick: LastForecloseCheckBlock is still zero, so the deployment
	// floor is read again and the scan retries [deployment, newHead].
	c.On("IsForeclosed", mock.Anything).Return(true, nil).Once()
	c.On("GetDeploymentBlockNumber", mock.Anything).
		Return(new(big.Int).SetUint64(10), nil).Once()
	c.On("RetrieveForeclosureEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == 10 && opts.End != nil && *opts.End == head2
		}),
	).Return([]*iapplication.IApplicationForeclosure{}, nil).Once()

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, head1)
	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, head2)

	assert.Zero(t, app.LastForecloseCheckBlock)
}

// TestCheckForForeclosure_SkipsWhenAlreadyScannedPastHead verifies the
// short-circuit: if a previous tick already advanced
// LastForecloseCheckBlock past the current head (e.g. defaultBlock
// policy temporarily falls back), the function must not issue any
// RetrieveForeclosureEvents call and must not read the deployment block
// either (LastForecloseCheckBlock > 0 alone satisfies the lower-bound
// check).
func TestCheckForForeclosure_SkipsWhenAlreadyScannedPastHead(t *testing.T) {
	s, c, repo := newForeclosureServiceFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := foreclosureTestApp(1)
	app.LastForecloseCheckBlock = 200
	// No GetDeploymentBlockNumber, no RetrieveForeclosureEvents, no
	// last_foreclose_check_block update — the short-circuit is the
	// assertion.

	s.checkForForeclosure(context.Background(),
		[]appContracts{foreclosureAppContracts(app, c)}, 150)

	assert.Equal(t, uint64(200), app.LastForecloseCheckBlock,
		"last_foreclose_check_block must not regress when head < last block")
}

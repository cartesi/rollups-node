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

// newPostForeclosureFixture builds the minimal Service surface needed by
// the post-foreclosure scans (drive-prove + withdrawal).
func newPostForeclosureFixture(t *testing.T) (
	*Service, *MockApplicationContract, *MockRepository,
) {
	t.Helper()
	repo := newMockRepository()
	appContract := newMockApplicationContract()
	s := &Service{
		repository: repo,
	}
	require.NoError(t, service.Create(
		context.Background(),
		&service.CreateInfo{Name: "evm-reader", Impl: s, Logger: slog.New(slog.NewTextHandler(os.Stdout, nil))},
		&s.Service,
	))
	return s, appContract, repo
}

// driveProvedTestApp builds a foreclosed Application with foreclose_block
// set to the given foreclose block; accounts_drive_proved_block is zero so
// the dispatcher routes here.
func driveProvedTestApp(id int64, forecloseBlock uint64) *Application {
	return &Application{
		ID:                  id,
		Name:                "app",
		IApplicationAddress: common.BigToAddress(big.NewInt(id)),
		Status:              ApplicationStatus_OK,
		ForecloseBlock:      forecloseBlock,
	}
}

// makeDriveProvedEvent builds a synthetic IApplicationAccountsDriveMerkleRootProved
// event with the given block / tx / root for stubbing
// RetrieveAccountsDriveProvedEvents.
func makeDriveProvedEvent(
	block uint64, txHash common.Hash, root common.Hash,
) *iapplication.IApplicationAccountsDriveMerkleRootProved {
	return &iapplication.IApplicationAccountsDriveMerkleRootProved{
		AccountsDriveMerkleRoot: [32]byte(root),
		Raw: types.Log{
			BlockNumber: block,
			TxHash:      txHash,
		},
	}
}

// ---------------------------------------------------------------------------
// checkForDriveProved
// ---------------------------------------------------------------------------

// TestCheckForDriveProved_NoEvent verifies the steady-state path: the on-chain
// proved flag is false, so no event scan or UpdateAccountsDriveProved call is
// made. The cursor still advances to mostRecent so the next tick scans only the
// new slice.
func TestCheckForDriveProved_NoEvent(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	const head = uint64(120)

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(false, common.Hash{}, nil).Once()
	repo.On("UpdateApplicationLastAccountsDriveProvedCheckBlock",
		mock.Anything, app.ID, head).Return(nil).Once()

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, head, app.LastAccountsDriveProvedCheckBlock,
		"in-memory cursor mirrors the DB advance")
	assert.Zero(t, app.AccountsDriveProvedBlock,
		"AccountsDriveProvedBlock must remain zero when no event was observed")
}

// TestCheckForDriveProved_PersistsAndMirrors walks the happy path: one
// AccountsDriveMerkleRootProved event in the window; the persist call
// receives the event's (block, txHash, root); the in-memory marker is
// mirrored so the next tick's dispatcher routes to the withdrawal scan.
func TestCheckForDriveProved_PersistsAndMirrors(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	const head = uint64(120)
	const eventBlock = uint64(110)
	txHash := common.HexToHash("0xcafe")
	root := common.HexToHash("0xfeed")

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(true, root, nil).Once()
	c.On("RetrieveAccountsDriveProvedEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		// startBlock = max(foreclose_block=100, last_cursor+1=1) = 100.
		return opts.Start == 100 && opts.End != nil && *opts.End == head
	})).Return([]*iapplication.IApplicationAccountsDriveMerkleRootProved{
		makeDriveProvedEvent(eventBlock, txHash, root),
	}, nil).Once()

	repo.On("UpdateAccountsDriveProved",
		mock.Anything, app.ID, eventBlock, txHash, root, head,
	).Return(nil).Once()

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, eventBlock, app.AccountsDriveProvedBlock,
		"in-memory AccountsDriveProvedBlock must mirror the DB write")
	if assert.NotNil(t, app.AccountsDriveProvedTransaction) {
		assert.Equal(t, txHash, *app.AccountsDriveProvedTransaction)
	}
	if assert.NotNil(t, app.AccountsDriveMerkleRoot) {
		assert.Equal(t, root, *app.AccountsDriveMerkleRoot)
	}
	assert.Equal(t, head, app.LastAccountsDriveProvedCheckBlock)
}

// TestCheckForDriveProved_TakesFirstWhenMultiple is defensive: the contract
// caps emissions at one (AccountsDriveMerkleRootAlreadyProved on a second
// call), but if FilterLogs ever returns more than one we must persist the
// first and ignore the rest rather than overwriting with the later event.
func TestCheckForDriveProved_TakesFirstWhenMultiple(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	const head = uint64(120)
	firstTx := common.HexToHash("0xaaaa")
	firstRoot := common.HexToHash("0x1111")

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(true, firstRoot, nil).Once()
	c.On("RetrieveAccountsDriveProvedEvents", mock.Anything).Return(
		[]*iapplication.IApplicationAccountsDriveMerkleRootProved{
			makeDriveProvedEvent(110, firstTx, firstRoot),
			makeDriveProvedEvent(115, common.HexToHash("0xbbbb"), common.HexToHash("0x2222")),
		}, nil,
	).Once()

	repo.On("UpdateAccountsDriveProved",
		mock.Anything, app.ID, uint64(110), firstTx, firstRoot, head,
	).Return(nil).Once()

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	if assert.NotNil(t, app.AccountsDriveMerkleRoot) {
		assert.Equal(t, firstRoot, *app.AccountsDriveMerkleRoot,
			"in-memory marker must hold the FIRST event's data")
	}
}

// TestCheckForDriveProved_CursorRespectsForecloseBlockAsFloor pins the
// search-window lower bound. When LastAccountsDriveProvedCheckBlock is 0
// and the foreclose block is mid-range, the scan must start at
// ForecloseBlock (not 1, not 0) — drive-prove cannot land before the
// foreclosure that gates it. If the proved state is true but the event is
// missing, the cursor remains unchanged so the window is retried.
func TestCheckForDriveProved_CursorRespectsForecloseBlockAsFloor(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 500)
	const head = uint64(600)

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(true, common.Hash{}, nil).Once()
	c.On("RetrieveAccountsDriveProvedEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == 500
	})).Return([]*iapplication.IApplicationAccountsDriveMerkleRootProved{}, nil).Once()

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.LastAccountsDriveProvedCheckBlock)
}

// TestCheckForDriveProved_SkipsWhenCursorPastHead verifies the
// short-circuit: a previous tick already advanced the cursor past the
// current head (defaultBlock policy drift, reorg recovery, etc.). The
// function must not issue any RetrieveAccountsDriveProvedEvents call and
// must not regress the cursor.
func TestCheckForDriveProved_SkipsWhenCursorPastHead(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	app.LastAccountsDriveProvedCheckBlock = 200
	const head = uint64(150)

	// No mock expectations — assertion is by negation.

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, uint64(200), app.LastAccountsDriveProvedCheckBlock,
		"in-memory cursor must not regress when head < last cursor")
}

// TestCheckForDriveProved_DoesNotAdvanceCursorOnQueryError verifies that when
// the FilterLogs call errors, the cursor remains unchanged so the next tick
// retries the same range instead of permanently skipping the event.
func TestCheckForDriveProved_DoesNotAdvanceCursorOnQueryError(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	const head = uint64(120)

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(true, common.Hash{}, nil).Once()
	c.On("RetrieveAccountsDriveProvedEvents", mock.Anything).
		Return([]*iapplication.IApplicationAccountsDriveMerkleRootProved(nil),
			errors.New("eth_getLogs failed")).Once()

	// No atomic drive-proved marker write — the scan errored before any persist
	// could fire.

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.LastAccountsDriveProvedCheckBlock,
		"scan failure keeps cursor unchanged for retry")
}

// TestCheckForDriveProved_AbortsOnDeadlineExceeded verifies the
// context-error semantics: a DeadlineExceeded mid-scan must abort the
// loop with one ERROR log; the cursor must NOT advance (otherwise we'd
// silently mask a stuck tick by claiming progress).
func TestCheckForDriveProved_AbortsOnDeadlineExceeded(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	const head = uint64(120)

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(false, common.Hash{}, context.DeadlineExceeded).Once()

	// No cursor advance expected — abort path.

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.LastAccountsDriveProvedCheckBlock,
		"DeadlineExceeded aborts before cursor advance")
}

// TestCheckForDriveProved_ErrNotFoundSkipsInMemoryMarker verifies the
// row-deleted-between-scan-and-write path. The repository returns
// ErrNotFound for the atomic drive-proved marker write; the in-memory marker must
// NOT be written (writing it would diverge from a row that no longer
// exists). Subsequent ticks have nothing to repair because the row is
// gone.
func TestCheckForDriveProved_ErrNotFoundSkipsInMemoryMarker(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	const head = uint64(120)
	const eventBlock = uint64(110)
	txHash := common.HexToHash("0xcafe")
	root := common.HexToHash("0xfeed")

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(true, root, nil).Once()
	c.On("RetrieveAccountsDriveProvedEvents", mock.Anything).Return(
		[]*iapplication.IApplicationAccountsDriveMerkleRootProved{
			makeDriveProvedEvent(eventBlock, txHash, root),
		}, nil).Once()
	repo.On("UpdateAccountsDriveProved",
		mock.Anything, app.ID, eventBlock, txHash, root, head,
	).Return(repository.ErrNotFound).Once()

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.AccountsDriveProvedBlock,
		"ErrNotFound must not set the in-memory marker — row is gone")
	assert.Nil(t, app.AccountsDriveProvedTransaction)
	assert.Nil(t, app.AccountsDriveMerkleRoot)
}

// TestCheckForDriveProved_DoesNotAdvanceCursorOnPersistError verifies that a
// failed write of the observed event leaves the scan cursor unchanged. This
// prevents the node from moving past the only window where the event was seen.
func TestCheckForDriveProved_DoesNotAdvanceCursorOnPersistError(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	const head = uint64(120)
	const eventBlock = uint64(110)
	txHash := common.HexToHash("0xcafe")
	root := common.HexToHash("0xfeed")

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(true, root, nil).Once()
	c.On("RetrieveAccountsDriveProvedEvents", mock.Anything).Return(
		[]*iapplication.IApplicationAccountsDriveMerkleRootProved{
			makeDriveProvedEvent(eventBlock, txHash, root),
		}, nil).Once()
	repo.On("UpdateAccountsDriveProved",
		mock.Anything, app.ID, eventBlock, txHash, root, head,
	).Return(errors.New("db unavailable")).Once()

	s.checkForDriveProved(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.AccountsDriveProvedBlock)
	assert.Zero(t, app.LastAccountsDriveProvedCheckBlock,
		"persist failure keeps cursor unchanged for retry")
}

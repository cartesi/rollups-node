// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// postForeclosureWithdrawalApp builds an Application that has already been
// foreclosed AND had its accounts drive proved; this is the state where the
// withdrawal scan runs.
func postForeclosureWithdrawalApp(id int64, forecloseBlock, driveProvedBlock uint64) *Application {
	return &Application{
		ID:                       id,
		Name:                     "app",
		IApplicationAddress:      common.BigToAddress(big.NewInt(id)),
		Status:                   ApplicationStatus_OK,
		ForecloseBlock:           forecloseBlock,
		AccountsDriveProvedBlock: driveProvedBlock,
	}
}

// makeWithdrawalEvent builds a synthetic IApplicationWithdrawal event with
// the given block/log positions and account fields. Used by the
// withdrawal-scan tests to stub RetrieveWithdrawalEvents.
func makeWithdrawalEvent(
	block uint64, logIndex uint, txHash common.Hash,
	accountIndex uint64, account, output []byte,
) *iapplication.IApplicationWithdrawal {
	return &iapplication.IApplicationWithdrawal{
		AccountIndex: accountIndex,
		Account:      account,
		Output:       output,
		Raw: types.Log{
			BlockNumber: block,
			TxHash:      txHash,
			Index:       logIndex,
		},
	}
}

// ---------------------------------------------------------------------------
// checkForPostForeclosureWithdrawals
// ---------------------------------------------------------------------------

// TestCheckForWithdrawals_NoWithdrawalsYet verifies the common steady-state
// path: the counter stays at zero across the whole scan window, no
// transitions fire, no withdrawals are persisted, and the cursor advances.
func TestCheckForWithdrawals_NoWithdrawalsYet(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	const head = uint64(130)

	c.On("GetNumberOfWithdrawals", mock.Anything).
		Return(big.NewInt(0), nil)
	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 0
		}), head).Return(nil).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, head, app.LastWithdrawalCheckBlock)
}

// TestCheckForWithdrawals_SingleWithdrawal walks the happy path:
// getNumberOfWithdrawals goes 0→1 at block 120, FilterWithdrawal is called
// with a 1-block window [120, 120], one event is returned, and the event plus
// cursor are persisted atomically.
func TestCheckForWithdrawals_SingleWithdrawal(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	const head = uint64(130)
	txHash := common.HexToHash("0xcafe")
	accountBytes := []byte{0xaa, 0xbb}
	outputBytes := []byte{0xcc, 0xdd}

	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() < 120
	})).Return(big.NewInt(0), nil)
	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= 120
	})).Return(big.NewInt(1), nil)

	c.On("RetrieveWithdrawalEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == 120 && opts.End != nil && *opts.End == 120
	})).Return([]*iapplication.IApplicationWithdrawal{
		makeWithdrawalEvent(120, 0, txHash, 7, accountBytes, outputBytes),
	}, nil).Once()

	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 1 &&
				ws[0].ApplicationID == app.ID &&
				ws[0].AccountIndex == 7 &&
				string(ws[0].Account) == string(accountBytes) &&
				string(ws[0].Output) == string(outputBytes) &&
				ws[0].BlockNumber == 120 &&
				ws[0].TransactionHash == txHash &&
				ws[0].LogIndex == 0
		}), head).Return(nil).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)
}

// TestCheckForWithdrawals_WithdrawalAtDriveProvedFloor verifies that the
// scanner detects a transition at AccountsDriveProvedBlock itself. This
// requires seeding FindTransitions with the contract invariant that no
// withdrawal exists before the accounts drive is proved; otherwise a
// withdrawal in the first scanned block is invisible.
func TestCheckForWithdrawals_WithdrawalAtDriveProvedFloor(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 120)
	const head = uint64(130)
	txHash := common.HexToHash("0xcafe")

	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= 120
	})).Return(big.NewInt(1), nil)
	c.On("RetrieveWithdrawalEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == 120 && opts.End != nil && *opts.End == 120
	})).Return([]*iapplication.IApplicationWithdrawal{
		makeWithdrawalEvent(120, 0, txHash, 7, []byte{0xaa}, []byte{0xbb}),
	}, nil).Once()

	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 1 &&
				ws[0].AccountIndex == 7 &&
				ws[0].BlockNumber == 120 &&
				ws[0].TransactionHash == txHash
		}), head).Return(nil).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, head, app.LastWithdrawalCheckBlock)
}

// TestCheckForWithdrawals_WithdrawalAtCursorNextBlock verifies that the
// scanner detects a withdrawal in the first newly scanned block after a
// previous successful tick.
func TestCheckForWithdrawals_WithdrawalAtCursorNextBlock(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	app.LastWithdrawalCheckBlock = 119
	const head = uint64(130)
	txHash := common.HexToHash("0xbeef")

	repo.On("GetNumberOfWithdrawals", mock.Anything, app.ID).Return(uint64(0), nil).Once()
	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= 120
	})).Return(big.NewInt(1), nil)
	c.On("RetrieveWithdrawalEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == 120 && opts.End != nil && *opts.End == 120
	})).Return([]*iapplication.IApplicationWithdrawal{
		makeWithdrawalEvent(120, 0, txHash, 9, []byte{0x01}, []byte{0x02}),
	}, nil).Once()

	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 1 &&
				ws[0].AccountIndex == 9 &&
				ws[0].BlockNumber == 120 &&
				ws[0].TransactionHash == txHash
		}), head).Return(nil).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, head, app.LastWithdrawalCheckBlock)
}

// TestCheckForWithdrawals_DoesNotAdvanceCursorWhenDBCountFails verifies that
// later scan windows rely on the DB withdrawal count as the previous counter.
// If that local read fails, no chain scan is attempted and the cursor remains
// unchanged.
func TestCheckForWithdrawals_DoesNotAdvanceCursorWhenDBCountFails(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	app.LastWithdrawalCheckBlock = 119
	const head = uint64(130)

	repo.On("GetNumberOfWithdrawals", mock.Anything, app.ID).
		Return(uint64(0), errors.New("db unavailable")).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, uint64(119), app.LastWithdrawalCheckBlock)
}

// TestCheckForWithdrawals_MultipleInOneBlock verifies the multi-event-per-
// block path: two Withdrawals fire in the same block (different account
// indices). Both must be persisted in the same cursor-advance transaction,
// preserving distinct log_index values.
func TestCheckForWithdrawals_MultipleInOneBlock(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	const head = uint64(130)
	txHash := common.HexToHash("0xbeef")

	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() < 120
	})).Return(big.NewInt(0), nil)
	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= 120
	})).Return(big.NewInt(2), nil)

	c.On("RetrieveWithdrawalEvents", mock.Anything).Return(
		[]*iapplication.IApplicationWithdrawal{
			makeWithdrawalEvent(120, 0, txHash, 3, []byte{0x01}, []byte{0x10}),
			makeWithdrawalEvent(120, 1, txHash, 5, []byte{0x02}, []byte{0x20}),
		}, nil,
	).Once()

	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 2 &&
				ws[0].AccountIndex == 3 &&
				ws[0].LogIndex == 0 &&
				ws[1].AccountIndex == 5 &&
				ws[1].LogIndex == 1
		}), head).Return(nil).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)
}

// TestCheckForWithdrawals_CursorRespectsDriveProvedAsFloor pins the
// search-window lower bound. When LastWithdrawalCheckBlock is 0 and the
// drive was proved mid-range, the scan must start at
// AccountsDriveProvedBlock (not 1, not 0) — withdrawals cannot land
// before the drive-prove that gates them.
func TestCheckForWithdrawals_CursorRespectsDriveProvedAsFloor(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 500)
	const head = uint64(600)

	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= 500
	})).Return(big.NewInt(0), nil)
	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 0
		}), head).Return(nil).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)
}

// TestCheckForWithdrawals_SkipsWhenCursorPastHead verifies the
// short-circuit: a previous tick already advanced the cursor past head.
// No RPC, no DB write. Mirrors the same check on the drive-prove side.
func TestCheckForWithdrawals_SkipsWhenCursorPastHead(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	app.LastWithdrawalCheckBlock = 200
	const head = uint64(150)

	// No mock expectations.

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, uint64(200), app.LastWithdrawalCheckBlock,
		"cursor must not regress when head < last cursor")
}

// TestCheckForWithdrawals_PersistErrorDoesNotAdvanceCursor verifies the
// atomic persistence contract: if inserting the observed withdrawals or
// advancing the cursor fails, the in-memory cursor must not advance.
func TestCheckForWithdrawals_PersistErrorDoesNotAdvanceCursor(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	const head = uint64(130)

	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() < 120
	})).Return(big.NewInt(0), nil)
	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= 120
	})).Return(big.NewInt(2), nil)

	c.On("RetrieveWithdrawalEvents", mock.Anything).Return(
		[]*iapplication.IApplicationWithdrawal{
			makeWithdrawalEvent(120, 0, common.HexToHash("0xaa"), 1, []byte{0x01}, []byte{0x10}),
			makeWithdrawalEvent(120, 1, common.HexToHash("0xbb"), 2, []byte{0x02}, []byte{0x20}),
		}, nil,
	).Once()

	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 2 &&
				ws[0].AccountIndex == 1 &&
				ws[1].AccountIndex == 2
		}), head).Return(errors.New("constraint violation")).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.LastWithdrawalCheckBlock,
		"insert failure keeps cursor unchanged for retry")
}

// TestCheckForWithdrawals_DoesNotAdvanceCursorOnQueryError verifies that a
// RetrieveWithdrawalEvents error mid-scan leaves the cursor unchanged. The
// next tick must retry the same block range instead of permanently skipping
// the missing events.
func TestCheckForWithdrawals_DoesNotAdvanceCursorOnQueryError(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	const head = uint64(130)

	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() < 120
	})).Return(big.NewInt(0), nil)
	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= 120
	})).Return(big.NewInt(1), nil)

	c.On("RetrieveWithdrawalEvents", mock.Anything).
		Return([]*iapplication.IApplicationWithdrawal(nil), errors.New("eth_getLogs failed")).Once()
	// No persistence expectation — the scan errored before completion.

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.LastWithdrawalCheckBlock,
		"query failure keeps cursor unchanged for retry")
}

// TestCheckForWithdrawals_DoesNotAdvanceCursorWhenTransitionHasNoEvent
// verifies the inconsistent RPC/log view path. A counter transition without a
// matching Withdrawal log is treated as retryable and must not advance the
// cursor.
func TestCheckForWithdrawals_DoesNotAdvanceCursorWhenTransitionHasNoEvent(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	const head = uint64(130)

	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() < 120
	})).Return(big.NewInt(0), nil)
	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() >= 120
	})).Return(big.NewInt(1), nil)
	c.On("RetrieveWithdrawalEvents", mock.Anything).
		Return([]*iapplication.IApplicationWithdrawal{}, nil).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.LastWithdrawalCheckBlock,
		"missing event after counter transition keeps cursor unchanged for retry")
}

// TestCheckForWithdrawals_AbortsOnDeadlineExceeded mirrors the drive-prove
// abort path: a DeadlineExceeded mid-scan aborts the loop without
// advancing the cursor.
func TestCheckForWithdrawals_AbortsOnDeadlineExceeded(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	const head = uint64(130)

	c.On("GetNumberOfWithdrawals", mock.Anything).
		Return((*big.Int)(nil), context.DeadlineExceeded).Maybe()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Zero(t, app.LastWithdrawalCheckBlock,
		"DeadlineExceeded aborts before cursor advance")
}

// TestCheckForWithdrawals_CountDivergenceMarksCorrupted verifies the
// fail-closed path: when the local DB withdrawal count exceeds the on-chain
// counter at the scan boundary, FindTransitions reports a monotonic violation.
// The node must treat this as terminal state corruption — mark the app
// CORRUPTED and stop — rather than self-healing. The cursor must not advance
// and no withdrawals may be persisted (no committed history is rewritten).
func TestCheckForWithdrawals_CountDivergenceMarksCorrupted(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	app.LastWithdrawalCheckBlock = 119
	const head = uint64(130)

	// Local ledger says 5 withdrawals; the chain says only 2 at the boundary.
	repo.On("GetNumberOfWithdrawals", mock.Anything, app.ID).Return(uint64(5), nil).Once()
	c.On("GetNumberOfWithdrawals", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() == 120
	})).Return(big.NewInt(2), nil).Once()

	repo.On("UpdateApplicationStatus",
		mock.Anything, app.ID, ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()
	// No StoreWithdrawalEvents expectation — failing closed persists nothing.

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, ApplicationStatus_Corrupted, app.Status,
		"count divergence must mark the app CORRUPTED")
	assert.True(t, hasWithdrawalLedgerDivergence(app),
		"the durable reason must identify withdrawal-ledger corruption")
	assert.Equal(t, uint64(119), app.LastWithdrawalCheckBlock,
		"cursor must not advance on a detected divergence")
}

// TestCheckForWithdrawals_SkipsPersistedWithdrawalLedgerDivergence verifies
// that once this specific ledger failure is durable, the scan stops entirely:
// no RPC, no DB read, and no persistence. The mocks have no expectations, so
// any call trips the test.
func TestCheckForWithdrawals_SkipsPersistedWithdrawalLedgerDivergence(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	app.LastWithdrawalCheckBlock = 119
	app.Status = ApplicationStatus_Corrupted
	reason := withdrawalLedgerDivergenceReasonPrefix + " test fixture"
	app.Reason = &reason
	const head = uint64(130)

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, uint64(119), app.LastWithdrawalCheckBlock,
		"the diverged withdrawal ledger's cursor must stay frozen")
}

// TestCheckForWithdrawals_ExistingIntegrityTerminalReportsLedgerDivergence
// verifies the honest fallback when the first terminal cause is immutable:
// preserve it, leave the cursor frozen, and report every detected fund-ledger
// divergence rather than hiding it behind a process-local latch.
func TestCheckForWithdrawals_ExistingIntegrityTerminalReportsLedgerDivergence(t *testing.T) {
	for _, status := range []ApplicationStatus{
		ApplicationStatus_Diverged,
		ApplicationStatus_Corrupted,
	} {
		t.Run(status.String(), func(t *testing.T) {
			s, c, repo := newPostForeclosureFixture(t)
			defer c.AssertExpectations(t)
			defer repo.AssertExpectations(t)

			var logs bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&logs, nil))

			app := postForeclosureWithdrawalApp(1, 100, 110)
			app.LastWithdrawalCheckBlock = 119
			app.Status = status
			originalReason := "earlier integrity failure"
			app.Reason = &originalReason

			repo.On("GetNumberOfWithdrawals", mock.Anything, app.ID).
				Return(uint64(5), nil).Once()
			c.On("GetNumberOfWithdrawals", mock.Anything).
				Return(big.NewInt(2), nil).Once()

			s.checkForPostForeclosureWithdrawals(context.Background(),
				appContracts{application: app, applicationContract: c}, 130)

			assert.Contains(t, logs.String(), "Withdrawal ledger divergence detected")
			assert.Equal(t, status, app.Status)
			assert.Equal(t, originalReason, *app.Reason)
			assert.Equal(t, uint64(119), app.LastWithdrawalCheckBlock)
			repo.AssertNumberOfCalls(t, "UpdateApplicationStatus", 0)
		})
	}
}

// TestCheckForWithdrawals_OtherCorruptionCauseStillIndexes verifies that the
// shared CORRUPTED status does not suppress a healthy withdrawal ledger when
// another observer, such as output indexing, discovered the inconsistency.
func TestCheckForWithdrawals_OtherCorruptionCauseStillIndexes(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	app.Status = ApplicationStatus_Corrupted
	reason := "Output mismatch. Application is in an invalid state."
	app.Reason = &reason
	const head = uint64(130)

	c.On("GetNumberOfWithdrawals", mock.Anything).Return(big.NewInt(0), nil)
	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 0
		}), head).Return(nil).Once()

	s.checkForPostForeclosureWithdrawals(context.Background(),
		appContracts{application: app, applicationContract: c}, head)

	assert.Equal(t, head, app.LastWithdrawalCheckBlock,
		"an unrelated corruption cause must not stop withdrawal observation")
}

// ---------------------------------------------------------------------------
// checkPostForeclosure dispatcher routing
// ---------------------------------------------------------------------------

// TestCheckPostForeclosure_SkipsNonForeclosedApps verifies the top-level
// dispatcher's gate: apps whose ForecloseBlock is zero must not reach
// either of the two scan branches. The mock has no expectations for any
// adapter or repo method tied to the scans — any call trips the test.
func TestCheckPostForeclosure_SkipsNonForeclosedApps(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := &Application{
		ID:                  99,
		Name:                "not-foreclosed",
		IApplicationAddress: common.BigToAddress(big.NewInt(99)),
		Status:              ApplicationStatus_OK,
		// ForecloseBlock left zero — should be skipped.
	}

	s.checkPostForeclosure(context.Background(),
		[]appContracts{{application: app, applicationContract: c}}, 100)
}

// TestCheckPostForeclosure_RoutesToDriveProvedWhenZero verifies the dispatcher
// routes to the drive-prove scan when AccountsDriveProvedBlock == 0.
// GetAccountsDriveMerkleRoot must be called; GetNumberOfWithdrawals must NOT be.
func TestCheckPostForeclosure_RoutesToDriveProvedWhenZero(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := driveProvedTestApp(1, 100)
	const head = uint64(120)

	c.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(false, common.Hash{}, nil).Once()
	repo.On("UpdateApplicationLastAccountsDriveProvedCheckBlock",
		mock.Anything, app.ID, head).Return(nil).Once()
	// No GetNumberOfWithdrawals — assertion by negation.

	s.checkPostForeclosure(context.Background(),
		[]appContracts{{application: app, applicationContract: c}}, head)
}

// TestCheckPostForeclosure_RoutesToWithdrawalsWhenProved verifies the
// dispatcher routes to the withdrawal scan once
// AccountsDriveProvedBlock != 0. GetNumberOfWithdrawals must be called;
// RetrieveAccountsDriveProvedEvents must NOT be.
func TestCheckPostForeclosure_RoutesToWithdrawalsWhenProved(t *testing.T) {
	s, c, repo := newPostForeclosureFixture(t)
	defer c.AssertExpectations(t)
	defer repo.AssertExpectations(t)

	app := postForeclosureWithdrawalApp(1, 100, 110)
	const head = uint64(130)

	c.On("GetNumberOfWithdrawals", mock.Anything).Return(big.NewInt(0), nil)
	repo.On("StoreWithdrawalEvents",
		mock.Anything, app.ID, mock.MatchedBy(func(ws []*Withdrawal) bool {
			return len(ws) == 0
		}), head).Return(nil).Once()

	s.checkPostForeclosure(context.Background(),
		[]appContracts{{application: app, applicationContract: c}}, head)
}

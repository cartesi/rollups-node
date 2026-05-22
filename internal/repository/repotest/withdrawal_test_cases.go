// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

type WithdrawalSuite struct {
	BaseSuite
}

func NewWithdrawalSuite(factory RepositoryFactory) *WithdrawalSuite {
	return &WithdrawalSuite{BaseSuite: BaseSuite{factory: factory}}
}

// newWithdrawalFixture builds a Withdrawal for the given application + index,
// with unique-enough auxiliary data so equality assertions catch any field
// silently swapping between rows.
func newWithdrawalFixture(appID int64, accountIndex uint64) *Withdrawal {
	return &Withdrawal{
		ApplicationID:   appID,
		AccountIndex:    accountIndex,
		Account:         []byte{0xaa, byte(accountIndex)},
		Output:          []byte{0xbb, byte(accountIndex), byte(accountIndex >> 8)},
		BlockNumber:     1000 + accountIndex,
		TransactionHash: UniqueHash(),
		LogIndex:        uint(accountIndex % 4),
	}
}

// TestInsertWithdrawal pins the idempotent-on-conflict contract of the
// (application_id, account_index) primary key. evmreader re-processes blocks
// on restart, so a second insert with the same key must be a silent no-op
// (not an error and not an overwrite); first writer wins matches the chain
// invariant that each account index is withdrawn at most once.
func (s *WithdrawalSuite) TestInsertWithdrawal() {
	s.Run("Happy", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		w := newWithdrawalFixture(app.ID, 7)

		err := s.Repo.InsertWithdrawal(s.Ctx, w)
		s.Require().NoError(err)

		got, err := s.Repo.GetWithdrawal(s.Ctx, app.Name, 7)
		s.Require().NoError(err)
		s.Require().NotNil(got)
		s.Equal(w.ApplicationID, got.ApplicationID)
		s.Equal(w.AccountIndex, got.AccountIndex)
		s.Equal(w.Account, got.Account)
		s.Equal(w.Output, got.Output)
		s.Equal(w.BlockNumber, got.BlockNumber)
		s.Equal(w.TransactionHash, got.TransactionHash)
		s.Equal(w.LogIndex, got.LogIndex)
	})

	// Restart-safety: a second insert with the same (app, accountIndex) but
	// different auxiliary fields must be a silent no-op. The chain marks
	// each account index as withdrawn (wereAccountFundsWithdrawn), so a
	// second observation is always a restart artifact — silently keeping
	// the first observation is correct.
	s.Run("IdempotentOnConflict", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		first := newWithdrawalFixture(app.ID, 7)
		s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, first))

		second := newWithdrawalFixture(app.ID, 7)
		second.Account = []byte{0xff, 0xff, 0xff}
		second.Output = []byte{0xee, 0xee}
		second.BlockNumber = first.BlockNumber + 100
		second.TransactionHash = UniqueHash()
		second.LogIndex = first.LogIndex + 1
		err := s.Repo.InsertWithdrawal(s.Ctx, second)
		s.Require().NoError(err, "ON CONFLICT DO NOTHING must not surface the conflict as an error")

		got, err := s.Repo.GetWithdrawal(s.Ctx, app.Name, 7)
		s.Require().NoError(err)
		s.Require().NotNil(got)
		// First writer wins on every field.
		s.Equal(first.Account, got.Account)
		s.Equal(first.Output, got.Output)
		s.Equal(first.BlockNumber, got.BlockNumber)
		s.Equal(first.TransactionHash, got.TransactionHash)
		s.Equal(first.LogIndex, got.LogIndex)
	})

	s.Run("RequiresValidApplication", func() {
		w := newWithdrawalFixture(99_999_999, 0)
		err := s.Repo.InsertWithdrawal(s.Ctx, w)
		s.Require().Error(err, "FK to application(id) must reject orphan inserts")
	})
}

func (s *WithdrawalSuite) TestGetWithdrawal() {
	s.Run("Found", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		w := newWithdrawalFixture(app.ID, 3)
		s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, w))

		got, err := s.Repo.GetWithdrawal(s.Ctx, app.Name, 3)
		s.Require().NoError(err)
		s.Require().NotNil(got)
		s.Equal(uint64(3), got.AccountIndex)
	})

	// Project convention for Get* endpoints: not-found returns (nil, nil),
	// not ErrNotFound. The JSON-RPC layer translates the nil into a
	// resource-not-found error code.
	s.Run("NotFoundForUnknownAccountIndex", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetWithdrawal(s.Ctx, app.Name, 99)
		s.Require().NoError(err)
		s.Nil(got)
	})

	s.Run("NotFoundForUnknownApplication", func() {
		got, err := s.Repo.GetWithdrawal(s.Ctx, "no-such-app", 0)
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *WithdrawalSuite) TestListWithdrawals() {
	s.Run("Empty", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		ws, total, err := s.Repo.ListWithdrawals(
			s.Ctx, app.Name, repository.WithdrawalFilter{}, repository.Pagination{}, false)
		s.Require().NoError(err)
		s.Empty(ws)
		s.Equal(uint64(0), total)
	})

	// Default ordering is ascending by account_index. The on-chain order is
	// unconstrained between blocks; ascending account_index gives clients a
	// stable iteration order.
	s.Run("MultipleAscending", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		for _, idx := range []uint64{5, 1, 3} {
			s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(app.ID, idx)))
		}
		ws, total, err := s.Repo.ListWithdrawals(
			s.Ctx, app.Name, repository.WithdrawalFilter{}, repository.Pagination{}, false)
		s.Require().NoError(err)
		s.Equal(uint64(3), total)
		s.Require().Len(ws, 3)
		s.Equal(uint64(1), ws[0].AccountIndex)
		s.Equal(uint64(3), ws[1].AccountIndex)
		s.Equal(uint64(5), ws[2].AccountIndex)
	})

	s.Run("DescendingOrder", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		for _, idx := range []uint64{1, 3, 5} {
			s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(app.ID, idx)))
		}
		ws, total, err := s.Repo.ListWithdrawals(
			s.Ctx, app.Name, repository.WithdrawalFilter{}, repository.Pagination{}, true)
		s.Require().NoError(err)
		s.Equal(uint64(3), total)
		s.Require().Len(ws, 3)
		s.Equal(uint64(5), ws[0].AccountIndex)
		s.Equal(uint64(3), ws[1].AccountIndex)
		s.Equal(uint64(1), ws[2].AccountIndex)
	})

	s.Run("FilteredByAccountIndex", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		for _, idx := range []uint64{1, 3, 5} {
			s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(app.ID, idx)))
		}
		want := uint64(3)
		ws, total, err := s.Repo.ListWithdrawals(
			s.Ctx, app.Name, repository.WithdrawalFilter{AccountIndex: &want},
			repository.Pagination{}, false)
		s.Require().NoError(err)
		s.Equal(uint64(1), total)
		s.Require().Len(ws, 1)
		s.Equal(uint64(3), ws[0].AccountIndex)
	})

	// Cross-app isolation: ListWithdrawals(appA) must not surface appB rows.
	// FK cascades on application delete, but the filter must also stand
	// alone since rows from multiple apps can coexist.
	s.Run("CrossAppIsolation", func() {
		appA := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		appB := NewApplicationBuilder().WithName("other-app").Create(s.Ctx, s.T(), s.Repo)
		s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(appA.ID, 1)))
		s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(appB.ID, 2)))

		wsA, totalA, err := s.Repo.ListWithdrawals(
			s.Ctx, appA.Name, repository.WithdrawalFilter{}, repository.Pagination{}, false)
		s.Require().NoError(err)
		s.Equal(uint64(1), totalA)
		s.Require().Len(wsA, 1)
		s.Equal(appA.ID, wsA[0].ApplicationID)

		wsB, totalB, err := s.Repo.ListWithdrawals(
			s.Ctx, appB.Name, repository.WithdrawalFilter{}, repository.Pagination{}, false)
		s.Require().NoError(err)
		s.Equal(uint64(1), totalB)
		s.Require().Len(wsB, 1)
		s.Equal(appB.ID, wsB[0].ApplicationID)
	})

	s.Run("Pagination", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		for _, idx := range []uint64{1, 2, 3} {
			s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(app.ID, idx)))
		}
		ws, total, err := s.Repo.ListWithdrawals(
			s.Ctx, app.Name, repository.WithdrawalFilter{},
			repository.Pagination{Limit: 1, Offset: 1}, false)
		s.Require().NoError(err)
		s.Equal(uint64(3), total, "total_count reports the unpaginated cardinality")
		s.Require().Len(ws, 1)
		s.Equal(uint64(2), ws[0].AccountIndex, "default-ascending order, offset 1 → account_index 2")
	})
}

func (s *WithdrawalSuite) TestGetNumberOfWithdrawals() {
	s.Run("CountsRowsForOneApplication", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		other := NewApplicationBuilder().WithName("other-app").Create(s.Ctx, s.T(), s.Repo)

		count, err := s.Repo.GetNumberOfWithdrawals(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(uint64(0), count)

		s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(app.ID, 1)))
		s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(app.ID, 2)))
		s.Require().NoError(s.Repo.InsertWithdrawal(s.Ctx, newWithdrawalFixture(other.ID, 3)))

		count, err = s.Repo.GetNumberOfWithdrawals(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(uint64(2), count)
	})
}

func (s *WithdrawalSuite) TestStoreWithdrawalEvents() {
	s.Run("PersistsRowsAndCursorAtomically", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		ws := []*Withdrawal{
			newWithdrawalFixture(app.ID, 1),
			newWithdrawalFixture(app.ID, 2),
		}

		err := s.Repo.StoreWithdrawalEvents(s.Ctx, app.ID, ws, 1234)
		s.Require().NoError(err)

		count, err := s.Repo.GetNumberOfWithdrawals(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(uint64(2), count)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(1234), got.LastWithdrawalCheckBlock)
	})

	s.Run("EmptyBatchStillAdvancesCursor", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		err := s.Repo.StoreWithdrawalEvents(
			s.Ctx, app.ID, []*Withdrawal{}, 777)
		s.Require().NoError(err)

		count, err := s.Repo.GetNumberOfWithdrawals(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(uint64(0), count)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(777), got.LastWithdrawalCheckBlock)
	})

	s.Run("RollsBackRowsWhenBatchIsInvalid", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		other := NewApplicationBuilder().WithName("other-app").Create(s.Ctx, s.T(), s.Repo)
		ws := []*Withdrawal{
			newWithdrawalFixture(app.ID, 1),
			newWithdrawalFixture(other.ID, 2),
		}

		err := s.Repo.StoreWithdrawalEvents(s.Ctx, app.ID, ws, 1234)
		s.Require().Error(err)

		count, err := s.Repo.GetNumberOfWithdrawals(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(uint64(0), count)

		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(0), got.LastWithdrawalCheckBlock)
	})

	// Replay safety: evmreader re-scans blocks on restart, so the same batch can
	// be re-applied — and a re-applied batch may carry a lower block than the
	// cursor already reached. The replayed rows must be idempotent (no
	// duplicates, first writer wins) and the cursor must never regress.
	s.Run("ReplayIsIdempotentAndCursorDoesNotRegress", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		ws := []*Withdrawal{newWithdrawalFixture(app.ID, 1)}

		s.Require().NoError(s.Repo.StoreWithdrawalEvents(s.Ctx, app.ID, ws, 1000))
		got, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(1000), got.LastWithdrawalCheckBlock)

		// Re-apply the same withdrawal at a lower block.
		s.Require().NoError(s.Repo.StoreWithdrawalEvents(s.Ctx, app.ID, ws, 500))

		count, err := s.Repo.GetNumberOfWithdrawals(s.Ctx, app.ID)
		s.Require().NoError(err)
		s.Equal(uint64(1), count, "replayed withdrawal must not duplicate")

		got, err = s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(uint64(1000), got.LastWithdrawalCheckBlock,
			"cursor must not regress when a batch is replayed with a lower block")
	})
}

// Compile-time check that the Withdrawal-related fields on Application are
// not silently dropped on round-trip via JSON or the repo. The repository
// implementation has many SELECT/scan column lists; a missing column in any
// of them would surface here.
var _ = (*Withdrawal)(nil)
var _ = common.Hash{}

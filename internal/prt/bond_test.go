// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

const noWinnerCase = "no winner"

func TestBondRecoveryFromBindingMapsAndOwnsPayment(t *testing.T) {
	claimer := common.HexToAddress("0x100")
	payment := big.NewInt(42)
	recovery, err := bondRecoveryFromBinding(2, claimer, payment)
	require.NoError(t, err)
	require.Equal(t, model.BondDispositionRecoverable, recovery.Disposition)
	require.Equal(t, claimer, recovery.Claimer)
	require.Equal(t, int64(42), recovery.Payment.Int64())
	recovery.Payment.SetInt64(7)
	require.Equal(t, int64(42), payment.Int64())
}

func TestBondRecoveryFromBindingValidatesCanonicalState(t *testing.T) {
	claimer := common.HexToAddress("0x100")
	for _, test := range []struct {
		name        string
		disposition uint8
		claimer     common.Address
		payment     *big.Int
	}{
		{"unknown disposition", 4, common.Address{}, big.NewInt(0)},
		{"nil payment", 3, common.Address{}, nil},
		{"negative payment", 2, claimer, big.NewInt(-1)},
		{"running claimer", 0, claimer, big.NewInt(0)},
		{"no winner payment", 1, common.Address{}, big.NewInt(1)},
		{"recovered claimer", 3, claimer, big.NewInt(0)},
		{"recoverable zero claimer", 2, common.Address{}, big.NewInt(0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := bondRecoveryFromBinding(test.disposition, test.claimer, test.payment)
			require.ErrorIs(t, err, errInvalidBondRecovery)
		})
	}
	recovery, err := bondRecoveryFromBinding(2, claimer, big.NewInt(0))
	require.NoError(t, err)
	require.Zero(t, recovery.Payment.Sign(), "a zero recoverable payment is valid")
}

// Recovery plans read mutable live state; the fake implements only the read
// operation that these plans need. Publishing always goes through txsender.
type recoveryPortAdapter struct {
	TournamentAdapter
	recovery BondRecovery
	err      error
	reads    []*bind.CallOpts
}

func (a *recoveryPortAdapter) BondRecovery(opts *bind.CallOpts) (BondRecovery, error) {
	a.reads = append(a.reads, opts)
	return a.recovery, a.err
}

type recoveryPortFixture struct {
	*deliveryFixture
	target  common.Address
	adapter *recoveryPortAdapter
}

func newRecoveryPortFixture(t *testing.T) *recoveryPortFixture {
	t.Helper()
	f := newDeliveryFixture(t)
	target := common.HexToAddress("0x900")
	f.repo.recoveries = []repository.RootBondRecoveryCandidate{{EpochIndex: 3, Tournament: target}}
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).Return(new(uint64(3)), nil).Maybe()
	adapter := &recoveryPortAdapter{recovery: canonicalBondRecovery(model.BondDispositionRecoverable, f.repo.signer, 1)}
	f.backend.code[f.repo.signer] = nil
	f.factory.On("CreateTournamentAdapter", target).Return(adapter, nil).Maybe()
	return &recoveryPortFixture{f, target, adapter}
}

func (f *recoveryPortFixture) plan(ctx context.Context) (*plannedAction, error) {
	return f.d.planAction(ctx, f.app, recoverBond, f.block, txsender.BlockRef{Number: 100})
}

func TestAcceptDoesNotCreateRecoveryCandidates(t *testing.T) {
	f := newDeliveryFixture(t)
	f.epoch.Status = model.EpochStatus_ClaimStaged
	view := resultTestSnapshot(f.epoch, true)
	view.accept.IsClaimStagingPeriodOver = true
	f.snapshot(view)
	call, err := f.d.planAction(t.Context(), f.app, acceptTournament, f.block, txsender.BlockRef{Number: 100})
	require.NoError(t, err)
	require.NotNil(t, call)
	require.Equal(t, acceptTournament, call.Key.Kind)
	require.Empty(t, f.d.rootBondRecoveryScans, "accept planning must not manufacture refund candidates")
	f.repo.AssertNotCalled(t, "ListRecoverableRootBonds", mock.Anything)
}

func TestRootBondRecoveryHandlesEveryDispositionAndOwnership(t *testing.T) {
	for _, test := range []struct {
		name     string
		recovery BondRecovery
		eligible bool
	}{
		{"running", canonicalBondRecovery(model.BondDispositionTournamentRunning, common.Address{}, 0), false},
		{noWinnerCase, canonicalBondRecovery(model.BondDispositionNoWinner, common.Address{}, 0), false},
		{"recovered by other actor", canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), false},
		{"external claimer", canonicalBondRecovery(model.BondDispositionRecoverable, common.HexToAddress("0x700"), 8), false},
		{"owned zero payment", canonicalBondRecovery(model.BondDispositionRecoverable, common.HexToAddress("0x600"), 0), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newRecoveryPortFixture(t)
			f.adapter.recovery = test.recovery
			if test.eligible {
				f.adapter.recovery.Claimer = f.repo.signer
			}
			call, err := f.plan(t.Context())
			require.NoError(t, err)
			require.Equal(t, test.eligible, call != nil)
			require.Empty(t, f.backend.published)
			require.Empty(t, f.repo.windows)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			for _, opts := range f.adapter.reads {
				require.Equal(t, f.block.Hash, opts.BlockHash)
				require.Nil(t, opts.BlockNumber)
				require.False(t, opts.Pending)
			}
		})
	}
}

func TestRootBondRecoveryIsIsolatedPerApplication(t *testing.T) {
	f := newRecoveryPortFixture(t)
	first, err := f.plan(t.Context())
	require.NoError(t, err)
	appTwo := *f.app
	appTwo.ID++
	appTwo.IApplicationAddress = common.HexToAddress("0x108")
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, appTwo.IApplicationAddress.Hex()).Return(new(uint64(3)), nil).Once()
	second, err := f.d.planAction(t.Context(), &appTwo, recoverBond, f.block, txsender.BlockRef{Number: 100})
	require.NoError(t, err)
	require.NotNil(t, second)
	require.NotEqual(t, first.Key, second.Key)
	require.Contains(t, f.d.rootBondRecoveryScans, f.app.ID)
	require.Contains(t, f.d.rootBondRecoveryScans, appTwo.ID)
}

func canonicalBondRecovery(disposition model.BondDisposition, claimer common.Address, payment int64) BondRecovery {
	return BondRecovery{Disposition: disposition, Claimer: claimer, Payment: big.NewInt(payment)}
}

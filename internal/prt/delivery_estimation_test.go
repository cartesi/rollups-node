// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum"
	"github.com/stretchr/testify/require"
)

// Model the RPC's balance cap independently of sender preparation. An unfunded
// priced estimate must fail before the sender can check its padded budget.
type balanceCappedEstimator struct {
	*deliveryBackendFake
	gas       uint64
	tip       *big.Int
	estimates []ethereum.CallMsg
}

func (b *balanceCappedEstimator) Estimate(ctx context.Context, msg ethereum.CallMsg, block txsender.BlockRef) (uint64, error) {
	b.estimates = append(b.estimates, msg)
	feeCap := msg.GasFeeCap
	if feeCap == nil {
		feeCap = msg.GasPrice
	}
	balance, err := b.Balance(ctx, msg.From, block)
	if err != nil {
		return 0, err
	}
	available := new(big.Int).Sub(balance, msg.Value)
	cost := new(big.Int).Mul(feeCap, new(big.Int).SetUint64(b.gas))
	if cost.Cmp(available) > 0 {
		return 0, &rpcDataError{code: 3, msg: "execution reverted", data: "0x"}
	}
	return b.gas, nil
}

func (b *balanceCappedEstimator) Tip(ctx context.Context) (*big.Int, error) {
	if b.tip != nil {
		return new(big.Int).Set(b.tip), nil
	}
	return b.deliveryBackendFake.Tip(ctx)
}

func useBalanceCappedEstimator(t *testing.T, f *deliveryFixture, gas uint64, legacy bool) *balanceCappedEstimator {
	t.Helper()
	backend := &balanceCappedEstimator{deliveryBackendFake: f.backend, gas: gas}
	policy := f.d.deliveryPolicy
	policy.Legacy = legacy
	var err error
	f.d.sender, err = txsender.New[actionID](backend, f.d.txOptsFactory, 31337, policy, f.d.guardPublication, f.d.Logger)
	require.NoError(t, err)
	return backend
}

func TestPRTOnlyJoinEstimatesWithoutFees(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		t.Run(actionName(kind), func(t *testing.T) {
			f, _, _ := preparePRTAction(t, kind)
			backend := useBalanceCappedEstimator(t, f, intrinsicGasFloor, false)
			require.NoError(t, f.d.tick(t.Context()))
			require.NotEmpty(t, backend.estimates)
			for _, msg := range backend.estimates {
				if kind == joinTournament {
					require.Zero(t, msg.GasFeeCap.Sign())
					require.Zero(t, msg.GasTipCap.Sign())
				} else {
					require.Positive(t, msg.GasFeeCap.Sign())
					require.Positive(t, msg.GasTipCap.Sign())
				}
			}
			require.Len(t, f.backend.published, 1)
			require.Positive(t, f.backend.published[0].GasFeeCap().Sign())
		})
	}
}

func TestPRTZeroBondJoinStillEstimatesWithoutFees(t *testing.T) {
	f, _, _ := preparePRTAction(t, joinTournament)
	adapter, err := f.factory.CreateTournamentAdapter(*f.epoch.TournamentAddress)
	require.NoError(t, err)
	for _, call := range adapter.(*tournamentAdapterMock).ExpectedCalls {
		if call.Method == "BondValue" {
			call.ReturnArguments[0] = new(big.Int)
		}
	}
	backend := useBalanceCappedEstimator(t, f, intrinsicGasFloor, false)
	require.NoError(t, f.d.tick(t.Context()))
	require.NotEmpty(t, backend.estimates)
	require.Zero(t, backend.estimates[0].GasFeeCap.Sign())
	require.Len(t, f.backend.published, 1)
	require.Zero(t, f.backend.published[0].Value().Sign())
}

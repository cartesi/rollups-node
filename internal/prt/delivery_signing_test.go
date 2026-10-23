// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"github.com/cartesi/rollups-node/internal/model"
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

const forwardBlockCase = "forward block"

type deliverySigningFactory struct {
	ethutil.TransactOptsFactory
	afterSign func()
}

func (f deliverySigningFactory) NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts, err := f.TransactOptsFactory.NewTransactOpts(ctx)
	if err != nil {
		return nil, err
	}
	result := *opts
	result.Signer = func(from common.Address, tx *types.Transaction) (*types.Transaction, error) {
		signed, signErr := opts.Signer(from, tx)
		if signErr == nil {
			f.afterSign()
		}
		return signed, signErr
	}
	return &result, nil
}

func TestPRTSlowSigningRechecksTheSameActionBeforePost(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		for _, change := range []string{disabledApplicationCase, "contract completed", forwardBlockCase} {
			t.Run(actionName(kind)+"/"+change, func(t *testing.T) {
				f, call, view := preparePRTAction(t, kind)
				factory := deliverySigningFactory{TransactOptsFactory: f.d.txOptsFactory, afterSign: func() {
					switch change {
					case disabledApplicationCase:
						f.app.Enabled = false
					case "contract completed":
						view.sealed.EpochNumber++
						view.stage.EpochNumber++
						view.accept.EpochNumber++
						updatePRTView(f, 111, view)
					case forwardBlockCase:
						updatePRTView(f, 111, view)
					}
				}}
				sender, err := txsender.New[actionID](f.backend, factory, 31337, f.d.deliveryPolicy, f.d.guardPublication, f.d.Logger)
				require.NoError(t, err)
				report, err := sender.PublishNext(t.Context(), singleAction{f.d, call})
				if change == forwardBlockCase {
					require.NoError(t, err)
					require.True(t, report.Attempted, "a normal next head does not discard a useful signature")
				} else {
					require.ErrorIs(t, err, txsender.ErrIneligible)
					require.False(t, report.Attempted)
					require.Empty(t, f.backend.published)
				}
			})
		}
	}
}

func TestPRTRecoverySigningRechecksRecipientAndDisposition(t *testing.T) {
	for _, change := range []string{disabledApplicationCase, "recovered", "foreign recipient", "recipient code"} {
		t.Run(change, func(t *testing.T) {
			f := newRecoveryPortFixture(t)
			call, err := f.plan(t.Context())
			require.NoError(t, err)
			require.NotNil(t, call)
			factory := deliverySigningFactory{TransactOptsFactory: f.d.txOptsFactory, afterSign: func() {
				switch change {
				case disabledApplicationCase:
					f.app.Enabled = false
				case "recovered":
					f.adapter.recovery = canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0)
				case "foreign recipient":
					f.adapter.recovery.Claimer = common.HexToAddress("0x777")
				case "recipient code":
					f.backend.code[f.repo.signer] = []byte{1}
				}
			}}
			sender, err := txsender.New[actionID](f.backend, factory, 31337, f.d.deliveryPolicy, f.d.guardPublication, f.d.Logger)
			require.NoError(t, err)
			report, err := sender.PublishNext(t.Context(), singleAction{f.d, *call})
			require.Error(t, err)
			require.False(t, report.Attempted)
			require.Empty(t, f.backend.published)
		})
	}
}

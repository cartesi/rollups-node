// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
)

func TestJoinTransactionSuccessDiagnostic(t *testing.T) {
	for _, sendErr := range []error{nil, errors.New("send failed")} {
		name := "sent"
		if sendErr != nil {
			name = "failed"
		}
		t.Run(name, func(t *testing.T) {
			app := prtRevertTestApp()
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
			factory := &adapterFactoryMock{}
			s := newRootBondTestService(common.HexToAddress("0x600"), factory)
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			repo := s.repository.(*prtRepositoryMock)
			repo.On("GetCommitment", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index,
				epoch.TournamentAddress.Hex(), epoch.Commitment.Hex()).Return(nil, nil).Once()
			adapter := &tournamentAdapterMock{}
			factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil).Once()
			opts := mock.MatchedBy(resultCallOptsAtBlock(20))
			adapter.On("CommitmentStanding", opts, [32]byte(*epoch.Commitment)).Return(CommitmentStanding{}, nil).Once()
			adapter.On("Descriptor", opts).
				Return(TournamentDescriptor{Height: model.Log2EpochComputationHashLeafCount}, nil).Once()
			adapter.On("BondValue", opts).Return(big.NewInt(1), nil).Once()
			tx := types.NewTx(&types.LegacyTx{Nonce: 1})
			adapter.On("JoinTournament", mock.Anything, [32]byte(*epoch.MachineHash),
				mock.Anything, mock.Anything, mock.Anything).Return(tx, sendErr).Once()

			joined, err := s.reactToTournament(t.Context(), app, epoch, 20)
			require.False(t, joined)
			if sendErr != nil {
				require.ErrorIs(t, err, sendErr)
				require.NotContains(t, output.String(), "Sent tournament join transaction")
				require.Empty(t, s.pendingTransactions)
			} else {
				require.NoError(t, err)
				require.Contains(t, output.String(), `level=INFO msg="Sent tournament join transaction"`)
				require.Contains(t, output.String(), "application="+app.Name)
				require.Contains(t, output.String(), "epoch_index=3")
				require.Contains(t, output.String(), "tournament="+epoch.TournamentAddress.Hex())
				require.Contains(t, output.String(), "tx="+tx.Hash().Hex())
			}
			repo.AssertExpectations(t)
			factory.AssertExpectations(t)
			adapter.AssertExpectations(t)
		})
	}
}

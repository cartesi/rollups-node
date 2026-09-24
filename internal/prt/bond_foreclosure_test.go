// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRootBondRecoveryForeclosureAdmission(t *testing.T) {
	owned := common.HexToAddress("0x600")
	tests := []struct {
		name           string
		foreclosed     bool
		candidateEpoch uint64
		recovery       BondRecovery
		wantRead       bool
		wantTx         bool
	}{
		{
			name: "foreclosed current epoch recovers owned bond", foreclosed: true, candidateEpoch: 3,
			recovery: canonicalBondRecovery(model.BondDispositionRecoverable, owned, 1), wantRead: true, wantTx: true,
		},
		{
			name: "foreclosed future epoch waits", foreclosed: true, candidateEpoch: 4,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := prtRevertTestApp()
			if test.foreclosed {
				app.ForecloseBlock = 10
			}
			factory := &adapterFactoryMock{}
			service := newRootBondTestService(owned, factory)
			tournamentAddress := common.HexToAddress("0x300")
			service.queueRootBondRecovery(app.ID, test.candidateEpoch, tournamentAddress)
			consensus := &daveConsensusAdapterMock{}
			consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(20))).
				Return(CurrentSealedEpoch{EpochNumber: 3, Tournament: tournamentAddress}, nil).Once()
			factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
			tournament := &tournamentAdapterMock{}
			if test.wantRead {
				factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()
				tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(20))).Return(test.recovery, nil).Once()
			}
			if test.wantTx {
				tournament.On("TryRecoveringBond", mock.Anything).
					Return(types.NewTx(&types.LegacyTx{Nonce: 1}), nil).Once()
			}

			require.NoError(t, service.recoverRootBonds(t.Context(), app, 20))
			require.Len(t, service.rootBondRecoveries[app.ID], 1)
			require.Equal(t, test.wantTx, service.rootBondRecoveries[app.ID][0].TxHash != nil)
			require.Empty(t, service.pendingTransactions)
			service.repository.(*prtRepositoryMock).AssertExpectations(t)
			factory.AssertExpectations(t)
			consensus.AssertExpectations(t)
			tournament.AssertExpectations(t)
		})
	}
}

func TestRootBondRecoveryWithoutCandidatesDoesNotReadChain(t *testing.T) {
	app := prtForeclosedApp(1, 10)
	factory := &adapterFactoryMock{}
	service := newRootBondTestService(common.HexToAddress("0x600"), factory)

	require.NoError(t, service.recoverRootBonds(t.Context(), app, 20))
	require.Empty(t, service.rootBondRecoveries)
	factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
	factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
}

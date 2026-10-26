// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func schedulingApp(id int64, consensus Consensus, source InputSourceAdapter) appContracts {
	return appContracts{application: &Application{
		ID: id, Enabled: true, Status: ApplicationStatus_OK, ConsensusType: consensus,
		IApplicationAddress: common.BigToAddress(big.NewInt(id)), IConsensusAddress: consensusAddr,
		IInputBoxAddress: inputBoxAddr, IInputBoxBlock: 10, EpochLength: 1000,
		LastInputCheckBlock: 99, LastOutputCheckBlock: 101, LastForecloseCheckBlock: 101,
	}, inputSource: source}
}

func schedulingEvents(addr common.Address, first uint64) []iinputbox.IInputBoxInputAdded {
	return []iinputbox.IInputBoxInputAdded{
		makeInputEvent(addr, first, 100), makeInputEvent(addr, first+1, 100),
	}
}

func TestInputSchedulingRotatesAcrossConsensusTypes(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		name := "busy application"
		if failFirst {
			name = "failing application"
		}
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			source := newMockInputBox()
			dave := newMockDaveConsensus()
			reader := &Service{repository: repo, maxInputsPerStep: 2}
			reader.Logger = testLogger(t)
			daveApp := schedulingApp(10, Consensus_PRT, source)
			daveApp.daveConsensus = dave
			daveApp.application.LastEpochCheckBlock = 99
			authority := schedulingApp(20, Consensus_Authority, source)
			quorum := schedulingApp(30, Consensus_Quorum, source)
			// An eight-input backlog exceeds every tick's allowance. No app
			// finishes during this test, so progress requires rotating priority.
			previous := &Epoch{Index: 0, FirstBlock: 10, LastBlock: 90, Status: EpochStatus_Closed}
			open := &Epoch{Index: 1, FirstBlock: 90, LastBlock: 99, Status: EpochStatus_Open}
			tournament := common.HexToAddress("0xaaaa")
			event := makeSealedEpochEvent(1, 0, 8, 101, tournament)
			attempts := 2
			if failFirst {
				attempts = 1
			}
			dave.On("GetCurrentSealedEpoch", blockRange(99, 101)).
				Return(makeSealedEpochResult(0, 0, 0, common.Address{}), nil)
			dave.On("GetCurrentSealedEpoch", blockFrom(101)).
				Return(makeSealedEpochResult(1, 0, 8, tournament), nil)
			dave.On("RetrieveSealedEpochs", mock.Anything).
				Return([]*idaveconsensus.IDaveConsensusEpochSealed{event}, nil).Times(attempts)
			address := daveApp.application.IApplicationAddress.Hex()
			repo.On("GetEpoch", t.Context(), address, uint64(0)).Return(previous, nil).Times(attempts)
			repo.On("UpdateEpochClaimTransactionHash", t.Context(), address, previous).Return(nil).Times(attempts)
			repo.On("GetEpoch", t.Context(), address, uint64(1)).Return(open, nil).Times(attempts)
			if !failFirst {
				repo.On("UpdateEventLastCheckBlock", t.Context(), []int64{10}, MonitoredEvent_EpochSealed, uint64(100)).
					Run(func(mock.Arguments) { daveApp.application.LastEpochCheckBlock = 100 }).Return(nil).Twice()
			}

			var order []int64
			stored := make(map[string]int)
			for _, app := range []appContracts{daveApp, authority, quorum} {
				addr := app.application.IApplicationAddress
				id := app.application.ID
				source.On("GetNumberOfInputs", blockRange(99, 100), addr).Return(big.NewInt(0), nil).Maybe()
				source.On("GetNumberOfInputs", blockFrom(100), addr).Return(big.NewInt(8), nil)
				logs := source.On("RetrieveInputs", mock.Anything, []common.Address{addr}, inputIndexRange(0, 2)).
					Run(func(mock.Arguments) {
						order = append(order, id)
						require.NotNil(t, reader.inputBudget)
						require.Zero(t, *reader.inputBudget)
					}).Once()
				if id == 10 && failFirst {
					logs.Return([]iinputbox.IInputBoxInputAdded(nil), errors.New("RPC unavailable"))
				} else {
					logs.Return(schedulingEvents(addr, 0), nil)
				}
				if id != 10 {
					repo.On("GetNumberOfInputs", t.Context(), addr.Hex()).Return(uint64(0), nil).Once()
					repo.On("GetEpoch", t.Context(), addr.Hex(), uint64(0)).Return((*Epoch)(nil), nil).Once()
				}
			}
			if !failFirst {
				addr := daveApp.application.IApplicationAddress
				source.On("RetrieveInputs", mock.Anything, []common.Address{addr}, inputIndexRange(2, 4)).
					Run(func(mock.Arguments) { order = append(order, 10) }).
					Return(schedulingEvents(addr, 2), nil).Once()
			}
			writes := 4
			if failFirst {
				writes = 2
			}
			repo.On("CreateEpochsAndInputs", t.Context(), mock.Anything, mock.Anything, uint64(99)).
				Run(func(args mock.Arguments) {
					addr := args.String(1)
					for _, inputs := range args.Get(2).(map[*Epoch][]*Input) {
						require.Len(t, inputs, 2)
						require.Equal(t, uint64(stored[addr]), inputs[0].Index)
						stored[addr] += len(inputs)
					}
				}).Return(nil).Times(writes)

			want := []int64{10, 20, 30, 10}
			if failFirst {
				want = want[:3]
			}
			for tick := range want {
				// Repository order changes must not give the busy Dave app
				// priority over the other consensus types every tick.
				apps := []appContracts{quorum, daveApp, authority}
				if tick%2 != 0 {
					apps = []appContracts{authority, quorum, daveApp}
				}
				require.Equal(t, !(failFirst && tick == 0), reader.runBlockScanners(t.Context(), apps, 101))
				require.Equal(t, want[:tick+1], order)
				require.Nil(t, reader.inputBudget)
			}
			require.Equal(t, 2, stored[authority.application.IApplicationAddress.Hex()])
			require.Equal(t, 2, stored[quorum.application.IApplicationAddress.Hex()])
			repo.AssertExpectations(t)
			source.AssertExpectations(t)
			dave.AssertExpectations(t)
		})
	}
}

func TestInputSchedulingHandlesApplicationMembershipChanges(t *testing.T) {
	repo := newMockRepository()
	source := newMockInputBox()
	reader := &Service{repository: repo, maxInputsPerStep: 2}
	reader.Logger = testLogger(t)
	apps := make(map[int64]appContracts)
	var order []int64
	for _, id := range []int64{5, 10, 20, 30, 40} {
		app := schedulingApp(id, Consensus_Authority, source)
		apps[id] = app
		addr := app.application.IApplicationAddress
		source.On("GetNumberOfInputs", mock.Anything, addr).Return(big.NewInt(8), nil)
		source.On("RetrieveInputs", mock.Anything, []common.Address{addr}, inputIndexRange(0, 2)).
			Run(func(mock.Arguments) { order = append(order, id) }).Return(schedulingEvents(addr, 0), nil).Once()
		repo.On("GetNumberOfInputs", t.Context(), addr.Hex()).Return(uint64(0), nil).Once()
		repo.On("GetEpoch", t.Context(), addr.Hex(), uint64(0)).Return((*Epoch)(nil), nil).Once()
		repo.On("CreateEpochsAndInputs", t.Context(), addr.Hex(), mock.Anything, uint64(99)).Return(nil).Once()
	}
	for tick, ids := range [][]int64{
		{30, 10, 20}, {40, 30, 20}, {40, 30}, {5, 40}, {5},
	} {
		var targets []appContracts
		for _, id := range ids {
			targets = append(targets, apps[id])
		}
		require.True(t, reader.runBlockScanners(t.Context(), targets, 101))
		require.Equal(t, []int64{10, 20, 30, 40, 5}[:tick+1], order)
	}
	repo.AssertExpectations(t)
	source.AssertExpectations(t)
}

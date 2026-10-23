// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"math"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInputBatchWithoutLimit(t *testing.T) {
	for _, test := range []struct {
		name                 string
		limit, stored, added uint64
	}{
		{name: "zero limit from first input", added: 1000},
		{name: "zero limit after stored inputs", stored: 500, added: 1000},
		{name: "zero limit without new inputs", stored: 500},
		{name: "large limit does not wrap", limit: math.MaxUint64, stored: 500, added: 1000},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			source := newMockInputBox()
			reader := &Service{repository: repo, maxInputsPerStep: test.limit}
			reader.Logger = testLogger(t)
			app := newDaveAppContracts(source, nil)
			end := test.stored + test.added
			repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(test.stored, nil).Once()
			source.On("GetNumberOfInputs", blockRange(100, 105), app1Addr).
				Return(new(big.Int).SetUint64(test.stored), nil)
			source.On("GetNumberOfInputs", blockFrom(105), app1Addr).
				Return(new(big.Int).SetUint64(end), nil)
			if test.added > 0 {
				source.On("RetrieveInputs", mock.Anything, []common.Address{app1Addr}, inputIndexRange(test.stored, end)).
					Return(batchInputEvents(test.stored, end, 105), nil).Once()
			}

			inputs, last, err := reader.readApplicationInputs(t.Context(), app, 100, 110)
			require.NoError(t, err)
			require.Len(t, inputs, int(test.added))
			require.Equal(t, uint64(110), last)
			for i, input := range inputs {
				require.Equal(t, test.stored+uint64(i), input.Index)
			}
			if test.added == 0 {
				source.AssertNotCalled(t, "RetrieveInputs", mock.Anything, mock.Anything, mock.Anything)
			}
			repo.AssertExpectations(t)
			source.AssertExpectations(t)
		})
	}
}

func TestEmptyInputBoundsDoNotQueryWildcardTopics(t *testing.T) {
	source := newMockInputBox()
	reader := &Service{}
	reader.Logger = testLogger(t)
	app := newDaveAppContracts(source, nil)
	source.On("GetNumberOfInputs", mock.Anything, app1Addr).Return(big.NewInt(10), nil).Once()

	inputs, _, _, err := reader.fetchInputs(t.Context(), app, 100, 100, big.NewInt(0), 5, 5)
	require.NoError(t, err)
	require.Empty(t, inputs)
	// An empty index topic list would match every input, rather than none.
	source.AssertNotCalled(t, "RetrieveInputs", mock.Anything, mock.Anything, mock.Anything)
	source.AssertExpectations(t)
}

func batchInputEvents(first, end, block uint64) []iinputbox.IInputBoxInputAdded {
	events := make([]iinputbox.IInputBoxInputAdded, 0, end-first)
	for index := first; index < end; index++ {
		events = append(events, makeInputEvent(app1Addr, index, block))
	}
	return events
}

func TestInputBatchResumesAtPartialBlockAfterGap(t *testing.T) {
	repo := newMockRepository()
	source := newMockInputBox()
	reader := &Service{repository: repo, maxInputsPerStep: 500}
	reader.Logger = testLogger(t)
	app := newDaveAppContracts(source, nil)
	source.On("GetNumberOfInputs", blockRange(100, 105), app1Addr).Return(big.NewInt(0), nil)
	source.On("GetNumberOfInputs", blockFrom(105), app1Addr).Return(big.NewInt(1000), nil)
	source.On("RetrieveInputs", mock.Anything, []common.Address{app1Addr}, inputIndexRange(0, 500)).
		Return(batchInputEvents(0, 500, 105), nil).Once()
	source.On("RetrieveInputs", mock.Anything, []common.Address{app1Addr}, inputIndexRange(500, 1000)).
		Return(batchInputEvents(500, 1000, 105), nil).Once()
	repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(uint64(0), nil).Once()

	inputs, last, err := reader.readApplicationInputs(t.Context(), app, 100, 110)
	require.NoError(t, err)
	require.Len(t, inputs, 500)
	require.Equal(t, uint64(104), last)

	// Model the first batch having been committed before the next tick.
	repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(uint64(500), nil).Once()
	inputs, last, err = reader.readApplicationInputs(t.Context(), app, last+1, 110)
	require.NoError(t, err)
	require.Len(t, inputs, 500)
	require.Equal(t, uint64(500), inputs[0].Index)
	require.Equal(t, uint64(999), inputs[len(inputs)-1].Index)
	require.GreaterOrEqual(t, last, uint64(105))
	repo.AssertExpectations(t)
	source.AssertExpectations(t)
}

func TestDavePartialSealDefersOpenScanUntilNextTick(t *testing.T) {
	repo := newMockRepository()
	source := newMockInputBox()
	dave := newMockDaveConsensus()
	reader := &Service{repository: repo, maxInputsPerStep: 500}
	reader.Logger = testLogger(t)
	app := newDaveAppContracts(source, dave)
	app.application.LastInputCheckBlock = 99
	app.application.LastEpochCheckBlock = 99
	app.application.Status = ApplicationStatus_OK
	app.application.Enabled = true
	app.application.ConsensusType = Consensus_PRT
	app.application.LastOutputCheckBlock = 101
	app.application.LastForecloseCheckBlock = 101
	previous := &Epoch{Index: 0, LastBlock: 50, Status: EpochStatus_Closed}
	open := &Epoch{Index: 1, FirstBlock: 50, LastBlock: 99, Status: EpochStatus_Open}
	tournament := common.HexToAddress("0xaaaa")
	event := makeSealedEpochEvent(1, 0, 600, 101, tournament)

	// Block 100 contains 500 inputs. Block 101 has 100 more before the seal
	// and 100 after it, belonging to epoch 2.
	dave.On("GetCurrentSealedEpoch", blockRange(99, 101)).
		Return(makeSealedEpochResult(0, 0, 0, common.Address{}), nil)
	dave.On("GetCurrentSealedEpoch", blockFrom(101)).
		Return(makeSealedEpochResult(1, 0, 600, tournament), nil)
	dave.On("RetrieveSealedEpochs", mock.Anything).
		Return([]*idaveconsensus.IDaveConsensusEpochSealed{event}, nil).Twice()
	repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).Return(previous, nil).Twice()
	repo.On("UpdateEpochClaimTransactionHash", t.Context(), app1Addr.Hex(), previous).Return(nil).Twice()
	repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(open, nil).Twice()
	source.On("GetNumberOfInputs", blockRange(99, 100), app1Addr).Return(big.NewInt(0), nil)
	source.On("GetNumberOfInputs", blockRange(100, 101), app1Addr).Return(big.NewInt(500), nil)
	source.On("GetNumberOfInputs", blockFrom(101), app1Addr).Return(big.NewInt(700), nil)
	for _, batch := range []struct{ first, end, block uint64 }{
		{0, 500, 100}, {500, 600, 101}, {600, 700, 101},
	} {
		source.On("RetrieveInputs", mock.Anything, []common.Address{app1Addr}, inputIndexRange(batch.first, batch.end)).
			Return(batchInputEvents(batch.first, batch.end, batch.block), nil).Once()
	}

	assignments := make(map[uint64]uint64)
	for _, cursor := range []uint64{99, 100, 101} {
		repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(), mock.Anything, cursor).
			Run(func(args mock.Arguments) {
				for epoch, inputs := range args.Get(2).(map[*Epoch][]*Input) {
					for _, input := range inputs {
						_, duplicate := assignments[input.Index]
						require.False(t, duplicate)
						assignments[input.Index] = epoch.Index
					}
				}
				app.application.LastInputCheckBlock = max(app.application.LastInputCheckBlock, args.Get(3).(uint64))
			}).Return(nil).Once()
	}
	for _, cursor := range []uint64{100, 101} {
		repo.On("UpdateEventLastCheckBlock", t.Context(), []int64{app.application.ID}, MonitoredEvent_EpochSealed, cursor).
			Run(func(args mock.Arguments) { app.application.LastEpochCheckBlock = args.Get(3).(uint64) }).Return(nil).Once()
	}
	// These expectations can only be reached after epoch 1 has closed.
	repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).
		Run(func(mock.Arguments) { require.Equal(t, EpochStatus_Closed, open.Status) }).Return(open, nil).Once()
	repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(2)).Return((*Epoch)(nil), nil).Once()
	repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID, MonitoredEvent_InputAdded).Return(uint64(100), nil).Once()
	repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(uint64(600), nil).Once()
	// The same budget also covers a different consensus application's inputs.
	otherSource := newMockInputBox()
	other := appContracts{application: &Application{
		ID: 2, Enabled: true, Status: ApplicationStatus_OK, ConsensusType: Consensus_Authority,
		IApplicationAddress: app2Addr, IInputBoxAddress: inputBoxAddr, IInputBoxBlock: 10,
		EpochLength: 1000, LastInputCheckBlock: 100, LastOutputCheckBlock: 101, LastForecloseCheckBlock: 101,
	}, inputSource: otherSource}
	otherSource.On("GetNumberOfInputs", mock.Anything, app2Addr).Return(big.NewInt(300), nil)
	otherEvents := make([]iinputbox.IInputBoxInputAdded, 300)
	for i := range otherEvents {
		otherEvents[i] = makeInputEvent(app2Addr, uint64(i), 101)
	}
	otherSource.On("RetrieveInputs", mock.Anything, []common.Address{app2Addr}, inputIndexRange(0, 300)).
		Return(otherEvents, nil).Once()
	repo.On("GetNumberOfInputs", t.Context(), app2Addr.Hex()).Return(uint64(0), nil).Once()
	repo.On("GetEpoch", t.Context(), app2Addr.Hex(), uint64(0)).Return((*Epoch)(nil), nil).Once()
	otherStored := 0
	repo.On("CreateEpochsAndInputs", t.Context(), app2Addr.Hex(), mock.Anything, uint64(101)).
		Run(func(args mock.Arguments) {
			for _, inputs := range args.Get(2).(map[*Epoch][]*Input) {
				otherStored += len(inputs)
			}
		}).Return(nil).Once()
	apps := []appContracts{app, other}

	require.True(t, reader.runBlockScanners(t.Context(), apps, 101))
	require.Len(t, assignments, 500)
	require.Zero(t, otherStored)
	require.Nil(t, reader.inputBudget)
	otherSource.AssertNotCalled(t, "GetNumberOfInputs", mock.Anything, mock.Anything)
	require.Equal(t, EpochStatus_Open, open.Status)
	require.Equal(t, uint64(500), open.InputIndexUpperBound)
	require.Equal(t, uint64(100), app.application.LastEpochCheckBlock)
	repo.AssertNotCalled(t, "GetLastNonOpenEpoch", mock.Anything, mock.Anything)

	require.True(t, reader.runBlockScanners(t.Context(), apps, 101))
	require.Len(t, assignments, 700)
	require.Equal(t, 300, otherStored)
	require.Nil(t, reader.inputBudget)
	for index := uint64(0); index < 700; index++ {
		wantEpoch := uint64(1)
		if index >= 600 {
			wantEpoch = 2
		}
		require.Equal(t, wantEpoch, assignments[index], "input %d", index)
	}
	require.Equal(t, EpochStatus_Closed, open.Status)
	require.Equal(t, uint64(600), open.InputIndexUpperBound)
	require.Equal(t, uint64(101), app.application.LastEpochCheckBlock)
	require.Equal(t, uint64(101), app.application.LastInputCheckBlock)
	repo.AssertExpectations(t)
	source.AssertExpectations(t)
	dave.AssertExpectations(t)
	otherSource.AssertExpectations(t)
}

func TestInputBudgetSharedAcrossApplications(t *testing.T) {
	for _, test := range []struct {
		name      string
		limit     uint64
		failFirst bool
	}{
		{name: "shared and reset", limit: 500},
		{name: "zero is unlimited"},
		{name: "failed request consumes allowance", limit: 500, failFirst: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			source := newMockInputBox()
			reader := &Service{repository: repo, maxInputsPerStep: test.limit}
			reader.Logger = testLogger(t)
			var apps []appContracts
			stored := map[common.Address]int{}
			for i, addr := range []common.Address{app1Addr, app2Addr} {
				application := &Application{
					ID: int64(i + 1), Enabled: true, Status: ApplicationStatus_OK, ConsensusType: Consensus_Authority,
					IApplicationAddress: addr, IInputBoxAddress: inputBoxAddr, IInputBoxBlock: 10, EpochLength: 1000,
					LastInputCheckBlock: 99, LastOutputCheckBlock: 100, LastForecloseCheckBlock: 100,
				}
				apps = append(apps, appContracts{application: application, inputSource: source})
				source.On("GetNumberOfInputs", mock.Anything, addr).Return(big.NewInt(300), nil)
				repo.On("GetNumberOfInputs", t.Context(), addr.Hex()).Return(uint64(0), nil).Once()
				end := uint64(300)
				if i == 1 && test.limit != 0 {
					end = 200
				}
				events := make([]iinputbox.IInputBoxInputAdded, end)
				for index := range events {
					events[index] = makeInputEvent(addr, uint64(index), 100)
				}
				logs := source.On("RetrieveInputs", mock.Anything, []common.Address{addr}, inputIndexRange(0, end)).Once()
				if i == 0 && test.failFirst {
					logs.Return([]iinputbox.IInputBoxInputAdded(nil), errors.New("RPC failure"))
					continue
				}
				logs.Return(events, nil)
				repo.On("GetEpoch", t.Context(), addr.Hex(), uint64(0)).Return((*Epoch)(nil), nil)
				cursor := uint64(100)
				if end < 300 {
					cursor = 99
				}
				repo.On("CreateEpochsAndInputs", t.Context(), addr.Hex(), mock.Anything, cursor).
					Run(func(args mock.Arguments) {
						for _, inputs := range args.Get(2).(map[*Epoch][]*Input) {
							stored[addr] += len(inputs)
						}
						application.LastInputCheckBlock = args.Get(3).(uint64)
					}).Return(nil).Once()
			}
			require.Equal(t, !test.failFirst, reader.runBlockScanners(t.Context(), apps, 100))
			require.Nil(t, reader.inputBudget)
			if test.limit == 0 {
				require.Equal(t, 600, stored[app1Addr]+stored[app2Addr])
			} else {
				require.Equal(t, 200, stored[app2Addr])
				require.Equal(t, uint64(99), apps[1].application.LastInputCheckBlock)
				if !test.failFirst {
					require.Equal(t, 500, stored[app1Addr]+stored[app2Addr])
					repo.On("GetNumberOfInputs", t.Context(), app2Addr.Hex()).Return(uint64(200), nil).Once()
					source.On("RetrieveInputs", mock.Anything, []common.Address{app2Addr}, inputIndexRange(200, 300)).
						Return(func() []iinputbox.IInputBoxInputAdded {
							events := make([]iinputbox.IInputBoxInputAdded, 100)
							for i := range events {
								events[i] = makeInputEvent(app2Addr, uint64(i)+200, 100)
							}
							return events
						}(), nil).Once()
					repo.On("CreateEpochsAndInputs", t.Context(), app2Addr.Hex(), mock.Anything, uint64(100)).
						Run(func(args mock.Arguments) {
							for _, inputs := range args.Get(2).(map[*Epoch][]*Input) {
								stored[app2Addr] += len(inputs)
							}
						}).Return(nil).Once()
					require.True(t, reader.runBlockScanners(t.Context(), apps, 100))
					require.Equal(t, 600, stored[app1Addr]+stored[app2Addr])
					require.Nil(t, reader.inputBudget)
				}
			}
			repo.AssertExpectations(t)
			source.AssertExpectations(t)
		})
	}
}

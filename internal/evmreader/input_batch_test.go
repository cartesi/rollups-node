// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"io"
	"log/slog"
	"math"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Honor the RPC's index filter, including when every input is in one block.
type batchInputSource struct {
	count    uint64
	requests [][]uint64
	failure  error
}

func (s *batchInputSource) GetNumberOfInputs(*bind.CallOpts, common.Address) (*big.Int, error) {
	return new(big.Int).SetUint64(s.count), nil
}
func (s *batchInputSource) RetrieveInputs(opts *bind.FilterOpts, addresses []common.Address, indices []*big.Int) ([]iinputbox.IInputBoxInputAdded, error) {
	if s.failure != nil {
		return nil, s.failure
	}
	var events []iinputbox.IInputBoxInputAdded
	var requested []uint64
	for _, index := range indices {
		requested = append(requested, index.Uint64())
		events = append(events, makeInputEvent(addresses[0], index.Uint64(), 19))
	}
	s.requests = append(s.requests, requested)
	return events, nil
}

func TestInputBatchResumesSameBlockAndKeepsEpochOpen(t *testing.T) {
	source := &batchInputSource{count: 5}
	repo := newMockRepository()
	reader := &Service{repository: repo, maxInputsPerStep: 2}
	reader.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	app := appContracts{application: &Application{ID: 1, IApplicationAddress: app1Addr, EpochLength: 10}, inputSource: source}
	var stored uint64
	var epoch *Epoch
	cursor := uint64(18)
	countCall := repo.On("GetNumberOfInputs", mock.Anything, app1Addr.String()).Return(uint64(0), nil)
	countCall.Run(func(mock.Arguments) { countCall.ReturnArguments[0] = stored })
	epochCall := repo.On("GetEpoch", mock.Anything, app1Addr.String(), uint64(1)).Return((*Epoch)(nil), nil)
	epochCall.Run(func(mock.Arguments) {
		epochCall.ReturnArguments[0] = (*Epoch)(nil)
		if epoch != nil {
			copy := *epoch
			epochCall.ReturnArguments[0] = &copy
		}
	})
	repo.On("CreateEpochsAndInputs", mock.Anything, app1Addr.String(), mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		batch := args.Get(2).(map[*Epoch][]*Input)
		require.Len(t, batch, 1)
		for e, inputs := range batch {
			require.LessOrEqual(t, len(inputs), 2)
			for _, input := range inputs {
				require.Equal(t, stored, input.Index)
				stored++
			}
			copy := *e
			epoch = &copy
		}
		cursor = args.Get(3).(uint64)
	}).Return(nil)
	for step := 0; step < 3; step++ {
		remaining := uint64(2)
		reader.inputBudget = &remaining
		require.NoError(t, reader.readAndStoreApplicationInputs(t.Context(), cursor, 25, app))
		if step < 2 {
			require.Equal(t, uint64(18), cursor)
			require.Equal(t, EpochStatus_Open, epoch.Status)
		}
	}
	require.Equal(t, uint64(5), stored)
	require.Equal(t, uint64(25), cursor)
	require.Equal(t, EpochStatus_Closed, epoch.Status)
	require.Equal(t, uint64(0), epoch.InputIndexLowerBound)
	require.Equal(t, uint64(5), epoch.InputIndexUpperBound)
	require.Equal(t, [][]uint64{{0, 1}, {2, 3}, {4}}, source.requests)
	repo.AssertExpectations(t)
}

func TestInputBudgetSharedAcrossApplications(t *testing.T) {
	remaining := uint64(3)
	reader := &Service{maxInputsPerStep: 3, inputBudget: &remaining}
	first := &batchInputSource{count: 2}
	second := &batchInputSource{count: 4}
	app := appContracts{application: &Application{IApplicationAddress: app1Addr}, inputSource: first}
	inputs, _, cursor, err := reader.fetchInputBatch(t.Context(), app, 19, 25, 0, math.MaxUint64)
	require.NoError(t, err)
	require.Len(t, inputs, 2)
	require.Equal(t, uint64(25), cursor)
	app.inputSource = second
	inputs, next, cursor, err := reader.fetchInputBatch(t.Context(), app, 19, 25, 0, math.MaxUint64)
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	require.Equal(t, uint64(1), next)
	require.Equal(t, uint64(18), cursor)
	_, _, _, err = reader.fetchInputBatch(t.Context(), app, 19, 25, 1, math.MaxUint64)
	require.ErrorIs(t, err, errInputBudgetExhausted)
	require.Len(t, second.requests, 1)
}

func TestInputBatchFailureLeavesBudgetRetryable(t *testing.T) {
	remaining := uint64(2)
	reader := &Service{maxInputsPerStep: 2, inputBudget: &remaining}
	failure := errors.New("RPC failed")
	source := &batchInputSource{count: 5, failure: failure}
	app := appContracts{application: &Application{IApplicationAddress: app1Addr}, inputSource: source}
	_, _, _, err := reader.fetchInputBatch(t.Context(), app, 19, 25, 0, math.MaxUint64)
	require.ErrorIs(t, err, failure)
	require.Equal(t, uint64(2), remaining)
}

func TestSealedEpochInputBatchResumesBeforeClosing(t *testing.T) {
	source := &batchInputSource{count: 5}
	repo := newMockRepository()
	reader := &Service{repository: repo, maxInputsPerStep: 2}
	reader.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	app := appContracts{application: &Application{ID: 1, IApplicationAddress: app1Addr, IInputBoxBlock: 1}, inputSource: source}
	event := &idaveconsensus.IDaveConsensusEpochSealed{EpochNumber: big.NewInt(0), InputIndexLowerBound: big.NewInt(0), InputIndexUpperBound: big.NewInt(5), Raw: types.Log{BlockNumber: 20}}
	var stored uint64
	var epoch *Epoch
	countCall := repo.On("GetNumberOfInputs", mock.Anything, app1Addr.String()).Return(uint64(0), nil)
	countCall.Run(func(mock.Arguments) { countCall.ReturnArguments[0] = stored })
	epochCall := repo.On("GetEpoch", mock.Anything, app1Addr.String(), uint64(0)).Return((*Epoch)(nil), nil)
	epochCall.Run(func(mock.Arguments) {
		epochCall.ReturnArguments[0] = (*Epoch)(nil)
		if epoch != nil {
			copy := *epoch
			epochCall.ReturnArguments[0] = &copy
		}
	})
	repo.On("CreateEpochsAndInputs", mock.Anything, app1Addr.String(), mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		for e, inputs := range args.Get(2).(map[*Epoch][]*Input) {
			require.LessOrEqual(t, len(inputs), 2)
			for _, input := range inputs {
				require.Equal(t, stored, input.Index)
				stored++
			}
			copy := *e
			epoch = &copy
		}
	}).Return(nil)
	for step := 0; step < 3; step++ {
		remaining := uint64(2)
		reader.inputBudget = &remaining
		err := reader.processSealedEpochEvent(t.Context(), app, event)
		if step < 2 {
			require.ErrorIs(t, err, errInputBudgetExhausted)
			require.Equal(t, EpochStatus_Open, epoch.Status)
		} else {
			require.NoError(t, err)
			require.Equal(t, EpochStatus_Closed, epoch.Status)
		}
	}
	require.Equal(t, uint64(5), stored)
	require.Equal(t, uint64(5), epoch.InputIndexUpperBound)
	repo.AssertExpectations(t)
}

func TestMaxInputsPerStepConfiguration(t *testing.T) {
	config.SetDefaults()
	value, err := config.GetEvmReaderMaxInputsPerStep()
	require.NoError(t, err)
	require.Equal(t, uint64(500), value)
	t.Setenv("CARTESI_EVM_READER_MAX_INPUTS_PER_STEP", "17")
	value, err = config.GetEvmReaderMaxInputsPerStep()
	require.NoError(t, err)
	require.Equal(t, uint64(17), value)
	require.Equal(t, value, (&config.NodeConfig{EvmReaderMaxInputsPerStep: value}).ToEvmreaderConfig().EvmReaderMaxInputsPerStep)
	t.Setenv("CARTESI_EVM_READER_MAX_INPUTS_PER_STEP", "-1")
	_, err = config.GetEvmReaderMaxInputsPerStep()
	require.Error(t, err)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func (s *EvmReaderSuite) TestItReadsInputsFromNewBlocksFilteredByDA() {
	wsClient := FakeWSEthClient{}
	s.evmReader.wsClient = &wsClient

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header0)
	wsClient.fireNewHead(&header1)
	wsClient.fireNewHead(&header2)
	wsClient.flushHeaders()

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 3)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 9)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestItReadsInputsFromNewFinalizedBlocks() {
	wsClient := FakeWSEthClient{}
	s.evmReader.wsClient = &wsClient
	s.evmReader.defaultBlock = DefaultBlock_Finalized

	s.client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()
	s.client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header1, nil).Once()
	s.client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header2, nil).Once()

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header3)
	wsClient.fireNewHead(&header3)
	wsClient.fireNewHead(&header3)
	wsClient.flushHeaders()

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 3)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 9)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestItUpdatesLastInputCheckBlockWhenThereIsNoInputs() {
	wsClient := FakeWSEthClient{}
	s.evmReader.wsClient = &wsClient

	// Prepare repository
	s.repository.Unset("UpdateEventLastCheckBlock")
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Return(nil).Times(2)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(4)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Once().Return(nil)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(2)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Once().Return(nil)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(2)

	s.repository.Unset("GetNumberOfInputs")
	s.repository.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Times(3)
	s.repository.Unset("CreateEpochsAndInputs")

	// Prepare sequence of inputs
	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.Unset("GetNumberOfInputs")
	s.inputBox.On(
		"GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(0), nil)

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header0)
	wsClient.fireNewHead(&header1)
	wsClient.fireNewHead(&header2)
	wsClient.flushHeaders()

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 0)
	s.inputBox.AssertExpectations(s.T())

	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestItReadsMultipleInputsFromSingleNewBlock() {

	wsClient := FakeWSEthClient{}
	s.evmReader.wsClient = &wsClient

	s.applicationContract1.Unset("GetDeploymentBlockNumber")
	s.applicationContract1.Unset("GetNumberOfExecutedOutputs")
	s.applicationContract1.On("GetNumberOfExecutedOutputs",
		mock.Anything,
	).Return(new(big.Int).SetUint64(0), nil).Once()

	// Prepare sequence of inputs
	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x13 }),
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{inputAddedEvent2, inputAddedEvent3}, nil)

	s.inputBox.Unset("GetNumberOfInputs")
	s.inputBox.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(2), nil)

	s.contractFactory = newMockAdapterFactory().SetupDefaultBehaviorSingleApp(s.applicationContract1, s.inputBox)
	s.evmReader.adapterFactory = s.contractFactory

	// Prepare Repo
	s.repository.Unset("ListApplications")
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		Name:                 "my-app-1",
		IApplicationAddress:  app1Addr,
		IConsensusAddress:    consensusAddr,
		IInputBoxAddress:     inputBoxAddr,
		DataAvailability:     DataAvailability_InputBox[:],
		IInputBoxBlock:       0x10,
		EpochLength:          10,
		LastInputCheckBlock:  0x12,
		LastOutputCheckBlock: 0x12,
	}}, uint64(1), nil).Once()
	// Catch-all for sentinel / extra headers
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{}, uint64(0), nil)

	s.repository.Unset("CreateEpochsAndInputs")
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		var epochInputMap map[*Epoch][]*Input
		obj := arguments.Get(2)
		epochInputMap, ok := obj.(map[*Epoch][]*Input)
		s.Require().True(ok)
		s.Require().Equal(1, len(epochInputMap))
		for _, inputs := range epochInputMap {
			s.Require().Equal(2, len(inputs))
			break
		}
	}).Return(nil)

	s.repository.Unset("UpdateEventLastCheckBlock")
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Once()

	s.repository.Unset("GetNumberOfInputs")
	s.repository.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Once()

	s.repository.Unset("GetEpoch")
	s.repository.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		uint64(1)).Return(nil, nil).Once()

	s.repository.Unset("GetNumberOfExecutedOutputs")
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Once()

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header2)
	// Give a time for
	wsClient.flushHeaders()

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 1)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
	s.inputBox.AssertExpectations(s.T())

	s.applicationContract1.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestItStartsWhenLastProcessedBlockIsTheMostRecentBlock() {
	wsClient := FakeWSEthClient{}
	s.evmReader.wsClient = &wsClient

	// Prepare Repo
	s.repository.Unset("ListApplications")
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		Name:                 "my-app-1",
		IApplicationAddress:  app1Addr,
		IConsensusAddress:    consensusAddr,
		IInputBoxAddress:     inputBoxAddr,
		DataAvailability:     DataAvailability_InputBox[:],
		IInputBoxBlock:       0x10,
		EpochLength:          10,
		LastInputCheckBlock:  0x13,
		LastOutputCheckBlock: 0x13,
	}}, uint64(1), nil).Once()
	// Catch-all for sentinel / extra headers
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{}, uint64(0), nil)

	s.repository.Unset("CreateEpochsAndInputs")
	s.repository.Unset("UpdateEventLastCheckBlock")
	s.repository.Unset("GetEpoch")
	s.repository.Unset("GetNumberOfInputs")
	s.repository.Unset("GetNumberOfExecutedOutputs")

	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.Unset("GetNumberOfInputs")

	s.applicationContract1.Unset("GetDeploymentBlockNumber")
	s.applicationContract1.Unset("RetrieveOutputExecutionEvents")
	s.applicationContract1.Unset("GetNumberOfExecutedOutputs")

	s.contractFactory.Unset("CreateAdapters")
	s.contractFactory.On("CreateAdapters",
		mock.Anything,
	).Return(s.applicationContract1, s.inputBox, nil, nil).Once()

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header2)
	wsClient.flushHeaders()

	s.repository.AssertExpectations(s.T())
	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())
}

// TestCheckpointNotAdvancedOnFetchFailure is a regression test for a bug where
// readInputsFromBlockchain swallowed per-app fetch errors, and the caller then
// advanced LastInputCheckBlock for failed apps — permanently skipping their inputs.
func (s *EvmReaderSuite) TestCheckpointNotAdvancedOnFetchFailure() {
	require := require.New(s.T())

	okAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	failAddr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	ibAddr := common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3")

	appOk := &Application{
		ID:                  1,
		Name:                "app-ok",
		IApplicationAddress: okAddr,
		IInputBoxAddress:    ibAddr,
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}
	appFail := &Application{
		ID:                  2,
		Name:                "app-fail",
		IApplicationAddress: failAddr,
		IInputBoxAddress:    ibAddr,
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}

	// appOk's inputSource succeeds with no inputs (GetNumberOfInputs returns 0 at both blocks)
	inputSource1 := &MockInputBox{}
	inputSource1.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)

	// appFail's inputSource fails
	inputSource2 := &MockInputBox{}
	inputSource2.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return((*big.Int)(nil), errors.New("RPC connection refused"))

	apps := []appContracts{
		{application: appOk, inputSource: inputSource1},
		{application: appFail, inputSource: inputSource2},
	}

	// Repository: appOk has 0 inputs in DB
	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, okAddr.String()).
		Return(uint64(0), nil)
	repo.On("GetNumberOfInputs", mock.Anything, failAddr.String()).
		Return(uint64(0), nil)

	// GetEpoch is called for appOk (which was successfully fetched with 0 inputs)
	// calculateEpochIndex(10, 100) = 10
	repo.On("GetEpoch", mock.Anything, okAddr.String(), uint64(10)).
		Return(nil, nil)

	// Expect UpdateEventLastCheckBlock to be called with ONLY appOk's ID.
	// appFail failed to fetch — its checkpoint must NOT be advanced.
	repo.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.MatchedBy(func(ids []int64) bool {
			// Must contain only appOk (ID=1), not appFail (ID=2)
			for _, id := range ids {
				if id == appFail.ID {
					return false
				}
			}
			return len(ids) == 1 && ids[0] == appOk.ID
		}),
		MonitoredEvent_InputAdded,
		uint64(110),
	).Return(nil).Once()

	s.evmReader.repository = repo

	err := s.evmReader.readAndStoreInputs(s.ctx, 100, 110, apps)
	require.NoError(err)

	repo.AssertExpectations(s.T())
}

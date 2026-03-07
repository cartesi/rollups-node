// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"math/big"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func (s *EvmReaderSuite) TestItReadsInputsFromNewBlocksFilteredByDA() {
	wsClient := FakeWSEhtClient{}
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
	time.Sleep(time.Second)

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
	wsClient := FakeWSEhtClient{}
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
	time.Sleep(time.Second)

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
	wsClient := FakeWSEhtClient{}
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
	).Return(new(big.Int).SetUint64(0), nil).Times(4)

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
	time.Sleep(time.Second)

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

	wsClient := FakeWSEhtClient{}
	s.evmReader.wsClient = &wsClient

	s.applicationContract1.Unset("GetDeploymentBlockNumber")
	s.applicationContract1.Unset("GetNumberOfExecutedOutputs")
	s.applicationContract1.On("GetNumberOfExecutedOutputs",
		mock.Anything,
	).Return(new(big.Int).SetUint64(0), nil).Once()

	// Prepare sequence of inputs
	s.inputBox.Unset("RetrieveInputs")
	events_2 := []iinputbox.IInputBoxInputAdded{inputAddedEvent2, inputAddedEvent3}
	mostRecentBlockNumber_2 := uint64(0x13)
	retrieveInputsOpts_2 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x13,
		End:     &mostRecentBlockNumber_2,
	}
	s.inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_2,
		mock.Anything,
		mock.Anything,
	).Return(events_2, nil)

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
		IApplicationAddress:  common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
		IInputBoxAddress:     common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
		DataAvailability:     DataAvailability_InputBox[:],
		IInputBoxBlock:       0x10,
		EpochLength:          10,
		LastInputCheckBlock:  0x12,
		LastOutputCheckBlock: 0x12,
	}}, uint64(1), nil).Once()

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
	time.Sleep(1 * time.Second)

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 1)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
	s.inputBox.AssertExpectations(s.T())

	s.applicationContract1.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestItStartsWhenLastProcessedBlockIsTheMostRecentBlock() {
	wsClient := FakeWSEhtClient{}
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
		IApplicationAddress:  common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
		IInputBoxAddress:     common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
		DataAvailability:     DataAvailability_InputBox[:],
		IInputBoxBlock:       0x10,
		EpochLength:          10,
		LastInputCheckBlock:  0x13,
		LastOutputCheckBlock: 0x13,
	}}, uint64(1), nil).Once()

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
	).Return(s.applicationContract1, s.inputBox, nil).Once()

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
	time.Sleep(1 * time.Second)

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

	app1Addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	app2Addr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	inputBoxAddr := common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3")

	app1 := &Application{
		ID:                  1,
		Name:                "app-ok",
		IApplicationAddress: app1Addr,
		IInputBoxAddress:    inputBoxAddr,
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}
	app2 := &Application{
		ID:                  2,
		Name:                "app-fail",
		IApplicationAddress: app2Addr,
		IInputBoxAddress:    inputBoxAddr,
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}

	// app1's inputSource succeeds with no inputs (GetNumberOfInputs returns 0 at both blocks)
	inputSource1 := &MockInputBox{}
	inputSource1.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)

	// app2's inputSource fails
	inputSource2 := &MockInputBox{}
	inputSource2.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return((*big.Int)(nil), errors.New("RPC connection refused"))

	apps := []appContracts{
		{application: app1, inputSource: inputSource1},
		{application: app2, inputSource: inputSource2},
	}

	// Repository: app1 has 0 inputs in DB
	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, app1Addr.String()).
		Return(uint64(0), nil)
	repo.On("GetNumberOfInputs", mock.Anything, app2Addr.String()).
		Return(uint64(0), nil)

	// GetEpoch is called for app1 (which was successfully fetched with 0 inputs)
	// calculateEpochIndex(10, 100) = 10
	repo.On("GetEpoch", mock.Anything, app1Addr.String(), uint64(10)).
		Return(nil, nil)

	// Expect UpdateEventLastCheckBlock to be called with ONLY app1's ID.
	// app2 failed to fetch — its checkpoint must NOT be advanced.
	repo.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.MatchedBy(func(ids []int64) bool {
			// Must contain only app1 (ID=1), not app2 (ID=2)
			for _, id := range ids {
				if id == app2.ID {
					return false
				}
			}
			return len(ids) == 1 && ids[0] == app1.ID
		}),
		MonitoredEvent_InputAdded,
		uint64(110),
	).Return(nil).Once()

	s.evmReader.repository = repo

	err := s.evmReader.readAndStoreInputs(s.ctx, 100, 110, apps)
	require.NoError(err)

	repo.AssertExpectations(s.T())
}

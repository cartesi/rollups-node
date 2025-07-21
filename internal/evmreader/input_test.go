// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
)

func (s *EvmReaderSuite) TestItReadsInputsFromNewBlocksFilteredByDA() {
	//New EVM Reader
	wsClient := FakeWSEhtClient{}
	s.evmReader.wsClient = &wsClient

	otherDA := DataAvailability_InputBox
	otherDA[0]++

	// Prepare repository
	s.repository.Unset("ListApplications")
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		IApplicationAddress: common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		IInputBoxAddress:    common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
		DataAvailability:    DataAvailability_InputBox[:],
		IInputBoxBlock:      0x10,
		EpochLength:         10,
		LastInputCheckBlock: 0x00,
	}}, uint64(1), nil).Once()
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		IApplicationAddress: common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		IInputBoxAddress:    common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
		DataAvailability:    otherDA[:],
		IInputBoxBlock:      0x10,
		EpochLength:         10,
		LastInputCheckBlock: 0x11,
	}}, uint64(1), nil).Once()

	s.repository.Unset("UpdateEventLastCheckBlock")
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
	).Once().Return(nil)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Once().Return(nil)

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header1, nil).Once()
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header2, nil).Once()

	inputBox := newMockInputBox()
	applicationContract := &MockApplicationContract{}
	applicationContract.On("RetrieveOutputExecutionEvents",
		mock.Anything,
	).Return([]*iapplication.IApplicationOutputExecuted{}, nil)

	s.contractFactory.Unset("CreateAdapters")
	s.contractFactory.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(applicationContract, inputBox, nil)

	// Prepare sequence of inputs
	inputBox.Unset("RetrieveInputs")
	events_0 := []iinputbox.IInputBoxInputAdded{inputAddedEvent0}
	mostRecentBlockNumber_0 := uint64(0x11)
	retrieveInputsOpts_0 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x10,
		End:     &mostRecentBlockNumber_0,
	}
	inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_0,
		mock.Anything,
		mock.Anything,
	).Return(events_0, nil)

	events_1 := []iinputbox.IInputBoxInputAdded{inputAddedEvent1}
	mostRecentBlockNumber_1 := uint64(0x12)
	retrieveInputsOpts_1 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x12,
		End:     &mostRecentBlockNumber_1,
	}
	inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_1,
		mock.Anything,
		mock.Anything,
	).Return(events_1, nil)

	inputBox.Unset("GetNumberOfInputs")
	inputBox.On(
		"GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(2), nil)

	applicationContract.On(
		"GetDeploymentBlockNumber",
		mock.Anything,
	).Return(new(big.Int).SetUint64(10), nil)

	inputBox.On(
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
	time.Sleep(time.Second)

	// retrieve inputs only for the application with: DataAvailability_InputBox
	inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"CreateEpochsAndInputs",
		1,
	)
}

func (s *EvmReaderSuite) TestItUpdatesLastInputCheckBlockWhenThereIsNoInputs() {
	wsClient := FakeWSEhtClient{}
	s.evmReader.wsClient = &wsClient

	// Prepare repository
	s.repository.Unset("ListApplications")
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		IApplicationAddress: common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		IInputBoxAddress:    common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
		DataAvailability:    DataAvailability_InputBox[:],
		IInputBoxBlock:      0x10,
		EpochLength:         10,
		LastInputCheckBlock: 0x00,
	}}, uint64(1), nil).Once()
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		IApplicationAddress: common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		IInputBoxAddress:    common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
		DataAvailability:    DataAvailability_InputBox[:],
		IInputBoxBlock:      0x10,
		EpochLength:         10,
		LastInputCheckBlock: 0x11,
	}}, uint64(1), nil).Once()

	s.repository.Unset("UpdateEventLastCheckBlock")
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Once().Return(nil)
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
	).Once().Return(nil)
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
	).Once().Return(nil)

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header1, nil).Once()
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header2, nil).Once()

	inputBox := newMockInputBox()
	// Setup adapter factory
	s.contractFactory.Unset("CreateAdapters")
	applicationContract := &MockApplicationContract{}
	applicationContract.On("RetrieveOutputExecutionEvents",
		mock.Anything,
	).Return([]*iapplication.IApplicationOutputExecuted{}, nil)

	s.contractFactory.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(applicationContract, inputBox, nil)

	// Prepare sequence of inputs
	inputBox.Unset("RetrieveInputs")
	events_0 := []iinputbox.IInputBoxInputAdded{}
	mostRecentBlockNumber_0 := uint64(0x11)
	retrieveInputsOpts_0 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x10,
		End:     &mostRecentBlockNumber_0,
	}
	inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_0,
		mock.Anything,
		mock.Anything,
	).Return(events_0, nil)

	inputBox.Unset("GetNumberOfInputs")
	inputBox.On(
		"GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(1), nil)

	applicationContract.On(
		"GetDeploymentBlockNumber",
		mock.Anything,
	).Return(new(big.Int).SetUint64(10), nil)

	inputBox.On(
		"GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(0), nil)

	events_1 := []iinputbox.IInputBoxInputAdded{}
	mostRecentBlockNumber_1 := uint64(0x12)
	retrieveInputsOpts_1 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x12,
		End:     &mostRecentBlockNumber_1,
	}
	inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_1,
		mock.Anything,
		mock.Anything,
	).Return(events_1, nil)

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
	time.Sleep(time.Second)

	inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 2)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"CreateEpochsAndInputs",
		2,
	)
}

func (s *EvmReaderSuite) TestItReadsMultipleInputsFromSingleNewBlock() {

	//New EVM Reader
	wsClient := FakeWSEhtClient{}
	s.evmReader.wsClient = &wsClient

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header2, nil).Once()

	inputBox := newMockInputBox()
	s.contractFactory.Unset("CreateAdapters")
	applicationContract := &MockApplicationContract{}
	applicationContract.On("RetrieveOutputExecutionEvents",
		mock.Anything,
	).Return([]*iapplication.IApplicationOutputExecuted{}, nil)

	s.contractFactory.On("CreateAdapters",
		mock.Anything,
		mock.Anything,
	).Return(applicationContract, inputBox, nil)

	// Prepare sequence of inputs
	inputBox.Unset("RetrieveInputs")
	events_2 := []iinputbox.IInputBoxInputAdded{inputAddedEvent2, inputAddedEvent3}
	mostRecentBlockNumber_2 := uint64(0x13)
	retrieveInputsOpts_2 := bind.FilterOpts{
		Context: s.ctx,
		Start:   0x13,
		End:     &mostRecentBlockNumber_2,
	}
	inputBox.On(
		"RetrieveInputs",
		&retrieveInputsOpts_2,
		mock.Anything,
		mock.Anything,
	).Return(events_2, nil)

	inputBox.Unset("GetNumberOfInputs")
	inputBox.On(
		"GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Return(new(big.Int).SetUint64(2), nil)

	applicationContract.On(
		"GetDeploymentBlockNumber",
		mock.Anything,
	).Return(new(big.Int).SetUint64(10), nil)

	// Prepare Repo
	s.repository.Unset("ListApplications")
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		IApplicationAddress: common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:   common.HexToAddress("0xdeadbeef"),
		IInputBoxAddress:    common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
		DataAvailability:    DataAvailability_InputBox[:],
		IInputBoxBlock:      0x10,
		EpochLength:         10,
		LastInputCheckBlock: 0x12,
	}}, uint64(1), nil).Once()
	s.repository.Unset("CreateEpochsAndInputs")
	s.repository.On(
		"CreateEpochsAndInputs",
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
	).Once().Return(nil)

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

	inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"CreateEpochsAndInputs",
		1,
	)
}

func (s *EvmReaderSuite) TestItStartsWhenLasProcessedBlockIsTheMostRecentBlock() {
	//New EVM Reader
	wsClient := FakeWSEhtClient{}
	s.evmReader.wsClient = &wsClient

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()

	// Prepare Repo
	s.repository.Unset("ListApplications")
	s.repository.On(
		"ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		IApplicationAddress:  common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
		IInputBoxAddress:     common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3"),
		DataAvailability:     DataAvailability_InputBox[:],
		IInputBoxBlock:       0x10,
		EpochLength:          10,
		LastInputCheckBlock:  0x13,
		LastOutputCheckBlock: 0x13,
	}}, uint64(1), nil).Once()

	s.repository.Unset("UpdateEventLastCheckBlock")
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
	).Once().Return(nil)

	inputBox := newMockInputBox()
	s.contractFactory.Unset("NewInputSource")
	s.contractFactory.On("NewInputSource",
		mock.Anything,
	).Return(inputBox, nil)

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

	inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 0)
	s.repository.AssertNumberOfCalls(
		s.T(),
		"CreateEpochsAndInputs",
		0,
	)
}

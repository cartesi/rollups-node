// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	appcontract "github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
)

func (s *EvmReaderSuite) TestOutputExecution() {

	wsClient := FakeWSEhtClient{}

	//New EVM Reader
	evmReader := NewEvmReader(
		s.client,
		&wsClient,
		s.inputBox,
		s.repository,
		0x10,
		DefaultBlockStatusLatest,
		s.contractFactory,
	)

	// Prepare repository
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:      common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
		LastOutputCheckBlock: 0x10,
	}}, nil).Once()
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:      common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E"),
		IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
		LastOutputCheckBlock: 0x11,
	}}, nil).Once()

	s.repository.Unset("UpdateOutputExecutionTransaction")
	s.repository.On("UpdateOutputExecutionTransaction",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		obj := arguments.Get(2)
		claims, ok := obj.([]*Output)
		s.Require().True(ok)
		s.Require().Equal(0, len(claims))

		obj = arguments.Get(3)
		lastOutputCheck, ok := obj.(uint64)
		s.Require().True(ok)
		s.Require().Equal(uint64(17), lastOutputCheck)

	}).Return(nil)
	s.repository.On("UpdateOutputExecutionTransaction",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		obj := arguments.Get(2)
		claims, ok := obj.([]*Output)
		s.Require().True(ok)
		s.Require().Equal(0, len(claims))

		obj = arguments.Get(3)
		lastOutputCheck, ok := obj.(uint64)
		s.Require().True(ok)
		s.Require().Equal(uint64(18), lastOutputCheck)

	}).Return(nil)

	//No Inputs
	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.On("RetrieveInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{}, nil)

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

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header0)
	wsClient.fireNewHead(&header1)
	time.Sleep(1 * time.Second)

	s.repository.AssertNumberOfCalls(
		s.T(),
		"UpdateOutputExecutionTransaction",
		2,
	)

}

func (s *EvmReaderSuite) TestReadOutputExecution() {

	appAddress := common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")

	// Contract Factory

	applicationContract := &MockApplicationContract{}

	contractFactory := newEmvReaderContractFactory()

	contractFactory.Unset("NewApplication")
	contractFactory.On("NewApplication",
		mock.Anything,
	).Return(applicationContract, nil)

	//New EVM Reader
	wsClient := FakeWSEhtClient{}
	evmReader := NewEvmReader(
		s.client,
		&wsClient,
		s.inputBox,
		s.repository,
		0x00,
		DefaultBlockStatusLatest,
		contractFactory,
	)

	// Prepare Output Executed Events
	outputExecution0 := &appcontract.IApplicationOutputExecuted{
		OutputIndex: 1,
		Output:      common.Hex2Bytes("AABBCCDDEE"),
		Raw: types.Log{
			TxHash: common.HexToHash("0xdeadbeef"),
		},
	}

	outputExecutionEvents := []*appcontract.IApplicationOutputExecuted{outputExecution0}
	applicationContract.On("RetrieveOutputExecutionEvents",
		mock.Anything,
	).Return(outputExecutionEvents, nil).Once()

	applicationContract.On("GetConsensus",
		mock.Anything,
	).Return(common.HexToAddress("0xdeadbeef"), nil)

	// Prepare repository
	s.repository.Unset("GetAllRunningApplications")
	s.repository.On(
		"GetAllRunningApplications",
		mock.Anything,
	).Return([]Application{{
		ContractAddress:      appAddress,
		IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
		LastOutputCheckBlock: 0x10,
	}}, nil).Once()

	output := &Output{
		Index:   1,
		RawData: common.Hex2Bytes("AABBCCDDEE"),
	}

	s.repository.Unset("GetOutput")
	s.repository.On("GetOutput",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(output, nil)

	s.repository.Unset("UpdateOutputExecutionTransaction")
	s.repository.On("UpdateOutputExecutionTransaction",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Once().Run(func(arguments mock.Arguments) {
		obj := arguments.Get(2)
		outputs, ok := obj.([]*Output)
		s.Require().True(ok)
		s.Require().Equal(1, len(outputs))
		output := outputs[0]
		s.Require().NotNil(output)
		s.Require().Equal(uint64(1), output.Index)
		s.Require().Equal(common.HexToHash("0xdeadbeef"), *output.TransactionHash)

	}).Return(nil)

	//No Inputs
	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.On("RetrieveInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{}, nil)

	// Prepare Client
	s.client.Unset("HeaderByNumber")
	s.client.On(
		"HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&header0, nil).Once()

	// Start service
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
		break
	case err := <-errChannel:
		s.FailNow("unexpected error signal", err)
	}

	wsClient.fireNewHead(&header0)
	time.Sleep(1 * time.Second)

	s.repository.AssertNumberOfCalls(
		s.T(),
		"UpdateOutputExecutionTransaction",
		1,
	)

}

func (s *EvmReaderSuite) TestCheckOutputFails() {
	s.Run("whenRetrieveOutputsFails", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		appAddress := common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")

		// Contract Factory

		applicationContract := &MockApplicationContract{}

		contractFactory := newEmvReaderContractFactory()

		contractFactory.Unset("NewApplication")
		contractFactory.On("NewApplication",
			mock.Anything,
		).Return(applicationContract, nil)

		//New EVM Reader
		client := newMockEthClient()
		wsClient := FakeWSEhtClient{}
		inputBox := newMockInputBox()
		repository := newMockRepository()
		evmReader := NewEvmReader(
			client,
			&wsClient,
			inputBox,
			repository,
			0x00,
			DefaultBlockStatusLatest,
			contractFactory,
		)

		applicationContract.On("RetrieveOutputExecutionEvents",
			mock.Anything,
		).Return([]*appcontract.IApplicationOutputExecuted{}, errors.New("No outputs for you"))

		applicationContract.On("GetConsensus",
			mock.Anything,
		).Return(common.HexToAddress("0xdeadbeef"), nil)

		// Prepare repository
		repository.Unset("GetAllRunningApplications")
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:      appAddress,
			IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
			LastOutputCheckBlock: 0x10,
		}}, nil).Once()

		output := &Output{
			Index:   1,
			RawData: common.Hex2Bytes("AABBCCDDEE"),
		}

		repository.Unset("GetOutput")
		repository.On("GetOutput",
			mock.Anything,
			mock.Anything,
			mock.Anything).Return(output, nil)

		repository.Unset("UpdateOutputExecutionTransaction")
		repository.On("UpdateOutputExecutionTransaction",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Once().Return(nil)

		//No Inputs
		inputBox.Unset("RetrieveInputs")
		inputBox.On("RetrieveInputs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]iinputbox.IInputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		// Start service
		ready := make(chan struct{}, 1)
		errChannel := make(chan error, 1)

		go func() {
			errChannel <- evmReader.Run(ctx, ready)
		}()

		select {
		case <-ready:
			break
		case err := <-errChannel:
			s.FailNow("unexpected error signal", err)
		}

		wsClient.fireNewHead(&header0)
		time.Sleep(1 * time.Second)

		s.repository.AssertNumberOfCalls(
			s.T(),
			"UpdateOutputExecutionTransaction",
			0,
		)

	})

	s.Run("whenGetOutputsFails", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		appAddress := common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")

		// Contract Factory

		applicationContract := &MockApplicationContract{}

		contractFactory := newEmvReaderContractFactory()

		contractFactory.Unset("NewApplication")
		contractFactory.On("NewApplication",
			mock.Anything,
		).Return(applicationContract, nil)

		//New EVM Reader
		client := newMockEthClient()
		wsClient := FakeWSEhtClient{}
		inputBox := newMockInputBox()
		repository := newMockRepository()
		evmReader := NewEvmReader(
			client,
			&wsClient,
			inputBox,
			repository,
			0x00,
			DefaultBlockStatusLatest,
			contractFactory,
		)

		// Prepare Output Executed Events
		outputExecution0 := &appcontract.IApplicationOutputExecuted{
			OutputIndex: 1,
			Output:      common.Hex2Bytes("AABBCCDDEE"),
			Raw: types.Log{
				TxHash: common.HexToHash("0xdeadbeef"),
			},
		}

		outputExecutionEvents := []*appcontract.IApplicationOutputExecuted{outputExecution0}
		applicationContract.On("RetrieveOutputExecutionEvents",
			mock.Anything,
		).Return(outputExecutionEvents, nil).Once()

		applicationContract.On("GetConsensus",
			mock.Anything,
		).Return(common.HexToAddress("0xdeadbeef"), nil)

		// Prepare repository
		repository.Unset("GetAllRunningApplications")
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:      appAddress,
			IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
			LastOutputCheckBlock: 0x10,
		}}, nil).Once()

		repository.Unset("GetOutput")
		repository.On("GetOutput",
			mock.Anything,
			mock.Anything,
			mock.Anything).Return(nil, errors.New("no output for you"))

		repository.Unset("UpdateOutputExecutionTransaction")
		repository.On("UpdateOutputExecutionTransaction",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Once().Return(nil)

		//No Inputs
		inputBox.Unset("RetrieveInputs")
		inputBox.On("RetrieveInputs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]iinputbox.IInputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		// Start service
		ready := make(chan struct{}, 1)
		errChannel := make(chan error, 1)

		go func() {
			errChannel <- evmReader.Run(ctx, ready)
		}()

		select {
		case <-ready:
			break
		case err := <-errChannel:
			s.FailNow("unexpected error signal", err)
		}

		wsClient.fireNewHead(&header0)
		time.Sleep(1 * time.Second)

		repository.AssertNumberOfCalls(
			s.T(),
			"UpdateOutputExecutionTransaction",
			0,
		)

	})

	s.Run("whenOutputMismatch", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		appAddress := common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")

		// Contract Factory

		applicationContract := &MockApplicationContract{}

		contractFactory := newEmvReaderContractFactory()

		contractFactory.Unset("NewApplication")
		contractFactory.On("NewApplication",
			mock.Anything,
		).Return(applicationContract, nil)

		//New EVM Reader
		client := newMockEthClient()
		wsClient := FakeWSEhtClient{}
		inputBox := newMockInputBox()
		repository := newMockRepository()
		evmReader := NewEvmReader(
			client,
			&wsClient,
			inputBox,
			repository,
			0x00,
			DefaultBlockStatusLatest,
			contractFactory,
		)

		// Prepare Output Executed Events
		outputExecution0 := &appcontract.IApplicationOutputExecuted{
			OutputIndex: 1,
			Output:      common.Hex2Bytes("AABBCCDDEE"),
			Raw: types.Log{
				TxHash: common.HexToHash("0xdeadbeef"),
			},
		}

		outputExecutionEvents := []*appcontract.IApplicationOutputExecuted{outputExecution0}
		applicationContract.On("RetrieveOutputExecutionEvents",
			mock.Anything,
		).Return(outputExecutionEvents, nil).Once()

		applicationContract.On("GetConsensus",
			mock.Anything,
		).Return(common.HexToAddress("0xdeadbeef"), nil)

		// Prepare repository
		repository.Unset("GetAllRunningApplications")
		repository.On(
			"GetAllRunningApplications",
			mock.Anything,
		).Return([]Application{{
			ContractAddress:      appAddress,
			IConsensusAddress:    common.HexToAddress("0xdeadbeef"),
			LastOutputCheckBlock: 0x10,
		}}, nil).Once()

		output := &Output{
			Index:   1,
			RawData: common.Hex2Bytes("FFBBCCDDEE"),
		}

		repository.Unset("GetOutput")
		repository.On("GetOutput",
			mock.Anything,
			mock.Anything,
			mock.Anything).Return(output, nil)

		repository.Unset("UpdateOutputExecutionTransaction")
		repository.On("UpdateOutputExecutionTransaction",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Once().Return(nil)

		//No Inputs
		inputBox.Unset("RetrieveInputs")
		inputBox.On("RetrieveInputs",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return([]iinputbox.IInputBoxInputAdded{}, nil)

		// Prepare Client
		client.Unset("HeaderByNumber")
		client.On(
			"HeaderByNumber",
			mock.Anything,
			mock.Anything,
		).Return(&header0, nil).Once()

		// Start service
		ready := make(chan struct{}, 1)
		errChannel := make(chan error, 1)

		go func() {
			errChannel <- evmReader.Run(ctx, ready)
		}()

		select {
		case <-ready:
			break
		case err := <-errChannel:
			s.FailNow("unexpected error signal", err)
		}

		wsClient.fireNewHead(&header0)
		time.Sleep(1 * time.Second)

		repository.AssertNumberOfCalls(
			s.T(),
			"UpdateOutputExecutionTransaction",
			0,
		)

	})
}

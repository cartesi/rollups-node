// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
)

// Prepare Output Executed Events
var output0 = &Output{
	Index:   0,
	RawData: common.Hex2Bytes("AABBCCDDEE"),
}
var output1 = &Output{
	Index:   1,
	RawData: common.Hex2Bytes("AABBCCDDEE"),
}
var outputExecution0 = &iapplication.IApplicationOutputExecuted{
	OutputIndex: output0.Index,
	Output:      output0.RawData,
	Raw: types.Log{
		TxHash: common.HexToHash("0xdeadbeef"),
	},
}
var outputExecution1 = &iapplication.IApplicationOutputExecuted{
	OutputIndex: output1.Index,
	Output:      output1.RawData,
	Raw: types.Log{
		TxHash: common.HexToHash("0xbeefbeef"),
	},
}

func (s *EvmReaderSuite) setupOutputExecution() {
	// On-chain state: 0 executed outputs before 0x11, 1 at 0x11-0x12, 2 from 0x13
	s.applicationContract1.Unset("GetNumberOfExecutedOutputs")
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockRange(0, 0x11)).
		Return(new(big.Int).SetUint64(0), nil)
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockRange(0x11, 0x13)).
		Return(new(big.Int).SetUint64(1), nil)
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockFrom(0x13)).
		Return(new(big.Int).SetUint64(2), nil)

	s.applicationContract1.On("RetrieveOutputExecutionEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x11 }),
	).Return([]*iapplication.IApplicationOutputExecuted{outputExecution0}, nil)
	s.applicationContract1.On("RetrieveOutputExecutionEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x13 }),
	).Return([]*iapplication.IApplicationOutputExecuted{outputExecution1}, nil)

	s.repository.Unset("UpdateEventLastCheckBlock")
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Return(nil).Times(1)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(6)

	s.repository.Unset("GetOutput")
	s.repository.On("GetOutput",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(output0, nil).Once()
	s.repository.On("GetOutput",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(output1, nil).Once()

	s.repository.Unset("GetNumberOfExecutedOutputs")
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Twice()
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(1), nil).Once()
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Once()
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(1), nil).Once()
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Once()

	s.repository.Unset("UpdateOutputsExecution")
	s.repository.On("UpdateOutputsExecution",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Run(func(arguments mock.Arguments) {
		obj := arguments.Get(2)
		outputs, ok := obj.([]*Output)
		s.Require().True(ok)
		s.Require().Equal(1, len(outputs))
		output := outputs[0]
		s.Require().NotNil(output)
		s.Require().Equal(uint64(0), output.Index)
		s.Require().Equal(outputExecution0.Raw.TxHash, *output.ExecutionTransactionHash)
	}).Return(nil).Once()
	s.repository.On("UpdateOutputsExecution",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Run(func(arguments mock.Arguments) {
		obj := arguments.Get(2)
		outputs, ok := obj.([]*Output)
		s.Require().True(ok)
		s.Require().Equal(1, len(outputs))
		output := outputs[0]
		s.Require().NotNil(output)
		s.Require().Equal(uint64(1), output.Index)
		s.Require().Equal(outputExecution1.Raw.TxHash, *output.ExecutionTransactionHash)
	}).Return(nil).Once()
}

func (s *EvmReaderSuite) TestOutputExecution() {
	wsClient := FakeWSEthClient{}
	s.evmReader.wsClient = &wsClient

	s.setupOutputExecution()

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

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 2)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

}

func (s *EvmReaderSuite) TestOutputExecutionOnFinalizedBlocks() {
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

	s.setupOutputExecution()

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

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 2)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

}

func (s *EvmReaderSuite) TestCheckOutputFailsWhenRetrieveOutputsFails() {
	wsClient := FakeWSEthClient{}
	s.evmReader.wsClient = &wsClient

	s.setupOutputExecution()

	s.applicationContract1.Unset("RetrieveOutputExecutionEvents")
	s.applicationContract1.On("RetrieveOutputExecutionEvents",
		mock.Anything,
	).Return([]*iapplication.IApplicationOutputExecuted{}, errors.New("No outputs for you"))

	// On-chain state: same as setupOutputExecution. Retrieval fails but oracle still works.
	s.applicationContract1.Unset("GetNumberOfExecutedOutputs")
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockRange(0, 0x11)).
		Return(new(big.Int).SetUint64(0), nil)
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockRange(0x11, 0x13)).
		Return(new(big.Int).SetUint64(1), nil)
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockFrom(0x13)).
		Return(new(big.Int).SetUint64(2), nil)

	apps := copyApplications(applications)
	s.repository.Unset("ListApplications")
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()

	apps = copyApplications(applications)
	apps[0].LastInputCheckBlock = 0x11
	apps[0].LastOutputCheckBlock = 0x0F
	apps[1].LastOutputCheckBlock = 0x11
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()

	apps = copyApplications(applications)
	apps[0].LastInputCheckBlock = 0x12
	apps[0].LastOutputCheckBlock = 0x0F
	apps[1].LastOutputCheckBlock = 0x12
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()
	// Catch-all for sentinel / extra headers
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{}, uint64(0), nil)

	s.repository.Unset("GetNumberOfExecutedOutputs")
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Times(6)

	s.repository.Unset("GetOutput")
	s.repository.Unset("UpdateOutputsExecution")

	s.repository.Unset("UpdateEventLastCheckBlock")
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Return(nil).Times(1)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(5)

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

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

}

func (s *EvmReaderSuite) TestCheckOutputFailsWhenGetOutputsFails() {
	wsClient := FakeWSEthClient{}
	s.evmReader.wsClient = &wsClient

	s.setupOutputExecution()

	s.repository.Unset("GetOutput")
	s.repository.On("GetOutput",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil, errors.New("no output for you")).Times(3)

	// On-chain state: same as setupOutputExecution. GetOutput fails but oracle still works.
	s.applicationContract1.Unset("GetNumberOfExecutedOutputs")
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockRange(0, 0x11)).
		Return(new(big.Int).SetUint64(0), nil)
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockRange(0x11, 0x13)).
		Return(new(big.Int).SetUint64(1), nil)
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockFrom(0x13)).
		Return(new(big.Int).SetUint64(2), nil)

	s.applicationContract1.Unset("RetrieveOutputExecutionEvents")
	s.applicationContract1.On("RetrieveOutputExecutionEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x11 }),
	).Return([]*iapplication.IApplicationOutputExecuted{outputExecution0}, nil)
	s.applicationContract1.On("RetrieveOutputExecutionEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x13 }),
	).Return([]*iapplication.IApplicationOutputExecuted{outputExecution1}, nil)

	apps := copyApplications(applications)
	s.repository.Unset("ListApplications")
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()

	apps = copyApplications(applications)
	apps[0].LastInputCheckBlock = 0x11
	apps[0].LastOutputCheckBlock = 0x0F
	apps[1].LastOutputCheckBlock = 0x11
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()

	apps = copyApplications(applications)
	apps[0].LastInputCheckBlock = 0x12
	apps[0].LastOutputCheckBlock = 0x0F
	apps[1].LastOutputCheckBlock = 0x12
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()
	// Catch-all for sentinel / extra headers
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{}, uint64(0), nil)

	s.repository.Unset("GetNumberOfExecutedOutputs")
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Times(6)

	s.repository.Unset("UpdateOutputsExecution")

	s.repository.Unset("UpdateEventLastCheckBlock")
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Return(nil).Times(1)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(5)

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

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) setupOutputMismatchTest() {
	s.client = newMockEthClient()
	s.repository = newMockRepository()
	s.applicationContract1 = newMockApplicationContract()
	s.inputBox = newMockInputBox()
	s.contractFactory = newMockAdapterFactory()

	s.evmReader = &Service{
		client:                              s.client,
		wsClient:                            s.wsClient,
		repository:                          s.repository,
		defaultBlock:                        DefaultBlock_Latest,
		adapterFactory:                      s.contractFactory,
		hasEnabledApps:                      true,
		inputReaderEnabled:                  true,
		blockchainMaxRetries:                0,
		blockchainSubscriptionRetryInterval: time.Second,
		wsLivenessTimeout:                   120 * time.Second,
	}

	logLevel, err := config.GetLogLevel()
	s.Require().NoError(err)

	serviceArgs := &service.CreateInfo{Name: "evm-reader", Impl: s.evmReader, LogLevel: logLevel}
	err = service.Create(context.Background(), serviceArgs, &s.evmReader.Service)
	s.Require().NoError(err)

	apps := copyApplications(applications)
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(2), nil).Once()

	apps = copyApplications(applications[1:2])
	apps[0].LastOutputCheckBlock = 0x11
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(1), nil).Once()

	apps = copyApplications(applications[1:2])
	apps[0].LastOutputCheckBlock = 0x12
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return(apps, uint64(1), nil).Once()
	// Catch-all for sentinel / extra headers
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{}, uint64(0), nil)

	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Return(nil).Times(1)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(5)

	s.repository.On("GetNumberOfInputs",
		mock.Anything,
		mock.Anything,
	).Once().Return(uint64(0), nil)

	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		mock.Anything,
	).Return(uint64(0), nil).Times(4)

	s.repository.On("CreateEpochsAndInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil).Once()

	s.repository.On("GetEpoch",
		mock.Anything,
		mock.Anything,
		uint64(0)).Return(nil, nil).Once()

	output := &Output{
		Index:   1,
		RawData: common.Hex2Bytes("FFBBCCDDEE"),
	}
	s.repository.On("GetOutput",
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(output, nil).Once()

	s.repository.On("UpdateApplicationState",
		mock.Anything,
		applications[0].ID,
		ApplicationState_Inoperable,
		mock.Anything,
	).Return(nil).Once()

	s.applicationContract1.On("GetDeploymentBlockNumber",
		mock.Anything,
	).Return(new(big.Int).SetUint64(0x10), nil).Once()

	// On-chain state: 0 executed outputs before 0x11, 1 from 0x11
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockRange(0, 0x11)).
		Return(new(big.Int).SetUint64(0), nil)
	s.applicationContract1.On("GetNumberOfExecutedOutputs", blockFrom(0x11)).
		Return(new(big.Int).SetUint64(1), nil)

	s.applicationContract1.On("RetrieveOutputExecutionEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x11 }),
	).Return([]*iapplication.IApplicationOutputExecuted{outputExecution0}, nil)

	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x11 }),
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{inputAddedEvent0}, nil)

	// On-chain: 0 inputs before block 0x11, 1 from block 0x11
	s.inputBox.On("GetNumberOfInputs", blockRange(0, 0x11), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	s.inputBox.On("GetNumberOfInputs", blockFrom(0x11), mock.Anything).
		Return(new(big.Int).SetUint64(1), nil)

	s.contractFactory.On("CreateAdapters",
		mock.MatchedBy(func(app *Application) bool {
			return app.IApplicationAddress == applications[0].IApplicationAddress
		}),
	).Return(s.applicationContract1, s.inputBox, nil, nil)
	s.contractFactory.On("CreateAdapters",
		mock.MatchedBy(func(app *Application) bool {
			return app.IApplicationAddress == applications[1].IApplicationAddress
		}),
	).Return(s.applicationContract2, nil, nil, nil)
}

func (s *EvmReaderSuite) TestCheckOutputFailsWhenOutputMismatches() {
	s.setupOutputMismatchTest()

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

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

}

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
	s.client.EnqueueNewHead(0x11).Once()
	s.client.EnqueueNewHead(0x12).Once()
	s.client.EnqueueNewHead(0x13).Once()
	called, blocked := newBlockedCallNotification(s.client.EnqueueNewHead(0x13))

	go s.evmReader.Serve() //nolint: errcheck

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 3)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 9)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

	close(blocked) // release blocked calls
}

func (s *EvmReaderSuite) TestItUpdatesLastInputCheckBlockWhenThereIsNoInputs() {
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

	s.client.EnqueueNewHead(0x11).Once()
	s.client.EnqueueNewHead(0x12).Once()
	s.client.EnqueueNewHead(0x13).Once()
	called, blocked := newBlockedCallNotification(s.client.EnqueueNewHead(0x13))

	// Start service
	go s.evmReader.Serve() //nolint: errcheck

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 0)
	s.inputBox.AssertExpectations(s.T())

	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

	close(blocked) // release blocked connection
}

func (s *EvmReaderSuite) TestItReadsMultipleInputsFromSingleNewBlock() {
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
	s.evmReader.resolver = newApplicationAdapterResolver(s.evmReader.Logger, s.contractFactory)

	// Prepare Repo
	s.repository.Unset("ListApplications")
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		Name:                    "my-app-1",
		IApplicationAddress:     app1Addr,
		IConsensusAddress:       consensusAddr,
		IInputBoxAddress:        inputBoxAddr,
		DataAvailability:        DataAvailability_InputBox[:],
		Enabled:                 true,
		Status:                  ApplicationStatus_OK,
		IInputBoxBlock:          0x10,
		EpochLength:             10,
		LastInputCheckBlock:     0x12,
		LastOutputCheckBlock:    0x12,
		LastForecloseCheckBlock: 0x13,
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

	s.client.EnqueueNewHead(0x13).Once()
	called, blocked := newBlockedCallNotification(s.client.EnqueueNewHead(0x13))

	// Start service
	go s.evmReader.Serve() //nolint: errcheck

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 1)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
	s.inputBox.AssertExpectations(s.T())

	s.applicationContract1.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

	close(blocked) // release blocked connection
}

func (s *EvmReaderSuite) TestItStartsWhenLastProcessedBlockIsTheMostRecentBlock() {
	// Prepare Repo
	s.repository.Unset("ListApplications")
	s.repository.On("ListApplications",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		false,
	).Return([]*Application{{
		Name:                    "my-app-1",
		IApplicationAddress:     app1Addr,
		IConsensusAddress:       consensusAddr,
		IInputBoxAddress:        inputBoxAddr,
		DataAvailability:        DataAvailability_InputBox[:],
		Enabled:                 true,
		Status:                  ApplicationStatus_OK,
		IInputBoxBlock:          0x10,
		EpochLength:             10,
		LastInputCheckBlock:     0x13,
		LastOutputCheckBlock:    0x13,
		LastForecloseCheckBlock: 0x13,
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

	s.client.EnqueueNewHead(0x13).Once()
	called, blocked := newBlockedCallNotification(s.client.EnqueueNewHead(0x13))

	// Start service
	go s.evmReader.Serve() //nolint: errcheck

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.repository.AssertExpectations(s.T())
	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

	close(blocked) // release blocked connection
}

func (s *EvmReaderSuite) TestCatchUpForeclosedInputsScansThroughForecloseBlock() {
	app := &Application{
		ID:                  42,
		Name:                "foreclosed-app",
		IApplicationAddress: app1Addr,
		IConsensusAddress:   consensusAddr,
		IInputBoxAddress:    inputBoxAddr,
		DataAvailability:    DataAvailability_InputBox[:],
		Enabled:             true,
		Status:              ApplicationStatus_OK,
		IInputBoxBlock:      1,
		EpochLength:         10,
		LastInputCheckBlock: 201,
		ForecloseBlock:      202,
	}

	s.repository.Unset("GetNumberOfInputs")
	s.repository.On("GetNumberOfInputs",
		mock.Anything,
		app.IApplicationAddress.String(),
	).Return(uint64(0), nil).Once()

	s.inputBox.Unset("GetNumberOfInputs")
	s.inputBox.On("GetNumberOfInputs",
		mock.Anything,
		app.IApplicationAddress,
	).Return(new(big.Int).SetUint64(0), nil).Maybe()
	s.inputBox.Unset("RetrieveInputs")

	s.repository.Unset("GetEpoch")
	s.repository.On("GetEpoch",
		mock.Anything,
		app.IApplicationAddress.String(),
		uint64(20),
	).Return(nil, nil).Once()

	s.repository.Unset("CreateEpochsAndInputs")
	s.repository.Unset("UpdateEventLastCheckBlock")
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		[]int64{app.ID},
		MonitoredEvent_InputAdded,
		app.ForecloseBlock,
	).Return(nil).Once()

	s.evmReader.scanIConsensusInputs(s.ctx, []appContracts{{
		application: app,
		inputSource: s.inputBox,
	}}, app.ForecloseBlock+10)

	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 1)
	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 0)
}

// A successful same-block InputAdded event is valid pre-foreclosure work. If a
// later AddInput transaction in the same block reverts after foreclosure, there
// is no InputAdded event for the node to index.
func (s *EvmReaderSuite) TestCatchUpForeclosedInputsStoresSameBlockInput() {
	app := &Application{
		ID:                  42,
		Name:                "foreclosed-app",
		IApplicationAddress: app1Addr,
		IConsensusAddress:   consensusAddr,
		IInputBoxAddress:    inputBoxAddr,
		DataAvailability:    DataAvailability_InputBox[:],
		Enabled:             true,
		Status:              ApplicationStatus_OK,
		IInputBoxBlock:      1,
		EpochLength:         10,
		LastInputCheckBlock: 201,
		ForecloseBlock:      202,
	}

	sameBlockInput := makeInputEvent(app.IApplicationAddress, 0, app.ForecloseBlock)

	s.repository.Unset("GetNumberOfInputs")
	s.repository.On("GetNumberOfInputs",
		mock.Anything,
		app.IApplicationAddress.String(),
	).Return(uint64(0), nil).Once()

	s.inputBox.Unset("GetNumberOfInputs")
	s.inputBox.On("GetNumberOfInputs",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() == app.ForecloseBlock
		}),
		app.IApplicationAddress,
	).Return(new(big.Int).SetUint64(1), nil).Twice()

	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == app.ForecloseBlock &&
				opts.End != nil &&
				*opts.End == app.ForecloseBlock
		}),
		[]common.Address{app.IApplicationAddress},
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{sameBlockInput}, nil).Once()

	s.repository.Unset("GetEpoch")
	s.repository.On("GetEpoch",
		mock.Anything,
		app.IApplicationAddress.String(),
		uint64(20),
	).Return(nil, nil).Once()

	s.repository.Unset("CreateEpochsAndInputs")
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything,
		app.IApplicationAddress.String(),
		mock.Anything,
		app.ForecloseBlock,
	).Run(func(arguments mock.Arguments) {
		epochInputMap, ok := arguments.Get(2).(map[*Epoch][]*Input)
		s.Require().True(ok)
		s.Require().Len(epochInputMap, 1)

		for epoch, inputs := range epochInputMap {
			s.Require().Equal(uint64(20), epoch.Index)
			s.Require().Len(inputs, 1)
			s.Require().Equal(uint64(0), inputs[0].Index)
			s.Require().Equal(app.ForecloseBlock, inputs[0].BlockNumber)
			s.Require().Equal(sameBlockInput.Raw.TxHash, inputs[0].TransactionReference)
		}
	}).Return(nil).Once()

	s.repository.Unset("UpdateEventLastCheckBlock")

	s.evmReader.scanIConsensusInputs(s.ctx, []appContracts{{
		application: app,
		inputSource: s.inputBox,
	}}, app.ForecloseBlock+10)

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 1)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	s.inputBox.AssertNumberOfCalls(s.T(), "RetrieveInputs", 1)
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

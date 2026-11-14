// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"errors"
	"log/slog"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
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
	).Return(nil).Times(5)
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
	s.client.EnqueueNewHead(0x11).Once()
	s.client.EnqueueNewHead(0x12).Once()
	s.client.EnqueueNewHead(0x13).Once()
	called, blocked := newBlockedCallNotification(s.client.EnqueueNewHead(0x13))

	s.setupOutputExecution()

	// Start service
	go s.evmReader.Serve(s.ctx) //nolint: errcheck

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 2)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

	close(blocked) // release blocked connection
}

func (s *EvmReaderSuite) TestOutputExecutionOnFinalizedBlocks() {
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

	s.client.EnqueueNewHead(0x11).Once()
	s.client.EnqueueNewHead(0x12).Once()
	called, blocked := newBlockedCallNotification(s.client.EnqueueNewHead(0x13))

	// Start service
	go s.evmReader.Serve(s.ctx) //nolint: errcheck

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 2)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

	close(blocked) // release blocked connection
}

// TestOutputExecutionContinuesForForeclosedApps verifies foreclosure does not
// block output-execution scanning: Application.executeOutput stays callable
// after foreclosure, so a foreclosed app's OutputExecuted events must still be
// observed and persisted. buildBlockScanPlan routes foreclosed apps into
// outputTargets (see block_scan_plan_test.go); this exercises the consuming
// scan with a foreclosed app.
func (s *EvmReaderSuite) TestOutputExecutionContinuesForForeclosedApps() {
	s.repository = newMockRepository()
	s.evmReader.repository = s.repository
	applicationContract := newMockApplicationContract()

	foreclosedApp := copyApplications(applications)[0]
	foreclosedApp.ID = 1
	foreclosedApp.ForecloseBlock = 0x12
	foreclosedApp.LastOutputCheckBlock = 0x12

	applicationContract.On("GetNumberOfExecutedOutputs", blockFrom(0x13)).
		Return(new(big.Int).SetUint64(1), nil)

	applicationContract.On("RetrieveOutputExecutionEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x13 }),
	).Return([]*iapplication.IApplicationOutputExecuted{outputExecution0}, nil).Once()

	s.repository.Unset("GetNumberOfExecutedOutputs")
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		foreclosedApp.IApplicationAddress.String(),
	).Return(uint64(0), nil).Once()

	s.repository.Unset("GetOutput")
	s.repository.On("GetOutput",
		mock.Anything,
		foreclosedApp.IApplicationAddress.Hex(),
		outputExecution0.OutputIndex,
	).Return(output0, nil).Once()

	s.repository.Unset("UpdateOutputsExecution")
	s.repository.On("UpdateOutputsExecution",
		mock.Anything,
		foreclosedApp.IApplicationAddress.Hex(),
		mock.MatchedBy(func(outputs []*Output) bool {
			return len(outputs) == 1 &&
				outputs[0].Index == outputExecution0.OutputIndex &&
				outputs[0].ExecutionTransactionHash != nil &&
				*outputs[0].ExecutionTransactionHash == outputExecution0.Raw.TxHash
		}),
		uint64(0x13),
	).Return(nil).Once()

	s.evmReader.checkForOutputExecution(s.ctx, []appContracts{
		{application: foreclosedApp, applicationContract: applicationContract},
	}, 0x13)

	s.repository.AssertExpectations(s.T())
	applicationContract.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestOutputExecutionSkipsAppsWithoutPendingExecutableOutputs() {
	repo := newMockRepository()
	repo.Unset("GetNumberOfPendingExecutableOutputs")
	repo.On("GetNumberOfPendingExecutableOutputs",
		mock.Anything,
		app1Addr.String(),
	).Return(uint64(0), nil).Once()
	s.evmReader.repository = repo

	applicationContract := newMockApplicationContract()
	app := appContracts{
		application: &Application{
			ID:                   1,
			Name:                 "test-app",
			IApplicationAddress:  app1Addr,
			LastOutputCheckBlock: 0x12,
		},
		applicationContract: applicationContract,
	}

	s.evmReader.checkForOutputExecution(s.ctx, []appContracts{app}, 0x13)

	repo.AssertNumberOfCalls(s.T(), "GetNumberOfExecutedOutputs", 0)
	repo.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	repo.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	repo.AssertExpectations(s.T())
	applicationContract.AssertNumberOfCalls(s.T(), "GetNumberOfExecutedOutputs", 0)
}

// TestOutputExecutionWaitsWhenOutputRowMissing verifies the OutputExecuted-
// before-its-row edge: when an OutputExecuted event is seen but the output row
// is not yet in the DB, the scan must not advance the cursor or persist —
// it waits so the next tick can retry once the row lands.
func (s *EvmReaderSuite) TestOutputExecutionWaitsWhenOutputRowMissing() {
	s.repository = newMockRepository()
	s.evmReader.repository = s.repository
	applicationContract := newMockApplicationContract()

	foreclosedApp := copyApplications(applications)[0]
	foreclosedApp.ID = 1
	foreclosedApp.ForecloseBlock = 0x12
	foreclosedApp.LastOutputCheckBlock = 0x12

	applicationContract.On("GetNumberOfExecutedOutputs", blockFrom(0x13)).
		Return(new(big.Int).SetUint64(1), nil)

	applicationContract.On("RetrieveOutputExecutionEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 0x13 }),
	).Return([]*iapplication.IApplicationOutputExecuted{outputExecution0}, nil).Once()

	s.repository.Unset("GetNumberOfExecutedOutputs")
	s.repository.On("GetNumberOfExecutedOutputs",
		mock.Anything,
		foreclosedApp.IApplicationAddress.String(),
	).Return(uint64(0), nil).Once()

	s.repository.Unset("GetOutput")
	s.repository.On("GetOutput",
		mock.Anything,
		foreclosedApp.IApplicationAddress.Hex(),
		outputExecution0.OutputIndex,
	).Return((*Output)(nil), nil).Once()

	s.repository.Unset("UpdateOutputsExecution")
	s.repository.Unset("UpdateEventLastCheckBlock")

	s.evmReader.checkForOutputExecution(s.ctx, []appContracts{
		{application: foreclosedApp, applicationContract: applicationContract},
	}, 0x13)

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	s.repository.AssertExpectations(s.T())
	applicationContract.AssertExpectations(s.T())
}

// OutputExecuted has no accepted-root identity. Different bytes can come from a later IOR root.
func (s *EvmReaderSuite) TestOutputMismatchIsDiagnosticForEveryHealth() {
	for _, status := range ApplicationStatusAllValues {
		s.Run(status.String(), func() {
			s.repository = newMockRepository()
			s.evmReader.repository = s.repository
			var output bytes.Buffer
			previousLogger := s.evmReader.Logger
			s.evmReader.Logger = slog.New(slog.NewTextHandler(&output, nil))
			defer func() { s.evmReader.Logger = previousLogger }()
			contract := newMockApplicationContract()
			app := copyApplications(applications)[0]
			app.ID, app.Status = 1, status
			app.ForecloseBlock, app.LastOutputCheckBlock = 0x12, 0x12
			mismatch := &Output{Index: outputExecution0.OutputIndex, RawData: common.Hex2Bytes("FFBBCCDDEE")}
			matching := &Output{Index: outputExecution1.OutputIndex, RawData: outputExecution1.Output}
			contract.On("GetNumberOfExecutedOutputs", blockFrom(0x13)).Return(new(big.Int).SetUint64(2), nil)
			contract.On("RetrieveOutputExecutionEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == 0x13
			})).Return([]*iapplication.IApplicationOutputExecuted{outputExecution0, outputExecution1}, nil).Once()
			s.repository.On("GetNumberOfExecutedOutputs", mock.Anything, app.IApplicationAddress.Hex()).Return(uint64(0), nil).Once()
			s.repository.On("GetOutput", mock.Anything, app.IApplicationAddress.Hex(), mismatch.Index).Return(mismatch, nil).Once()
			s.repository.On("GetOutput", mock.Anything, app.IApplicationAddress.Hex(), matching.Index).Return(matching, nil).Once()
			s.repository.On("UpdateOutputsExecution", mock.Anything, app.IApplicationAddress.Hex(),
				mock.MatchedBy(func(outputs []*Output) bool {
					return len(outputs) == 1 && outputs[0] == matching && outputs[0].ExecutionTransactionHash != nil &&
						*outputs[0].ExecutionTransactionHash == outputExecution1.Raw.TxHash
				}), uint64(0x13)).Return(nil).Once()
			s.evmReader.checkForOutputExecution(s.ctx, []appContracts{{application: app, applicationContract: contract}}, 0x13)
			s.Equal(status, app.Status)
			s.Contains(output.String(), "accepted-root provenance is unavailable")
			s.Contains(output.String(), "local_output_hash=")
			s.Contains(output.String(), "chain_output_hash=")
			s.Nil(mismatch.ExecutionTransactionHash)
			s.repository.AssertNotCalled(s.T(), "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			s.repository.AssertExpectations(s.T())
			contract.AssertExpectations(s.T())
		})
	}
}

func (s *EvmReaderSuite) TestCheckOutputFailsWhenRetrieveOutputsFails() {
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
	apps[1].LastInputCheckBlock = 0x11
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
	apps[1].LastInputCheckBlock = 0x12
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
	).Return(nil).Times(5)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(5)

	s.client.EnqueueNewHead(0x11).Once()
	s.client.EnqueueNewHead(0x12).Once()
	s.client.EnqueueNewHead(0x13).Once()
	called, blocked := newBlockedCallNotification(s.client.EnqueueNewHead(0x13))
	// Start service
	go s.evmReader.Serve(s.ctx) //nolint: errcheck

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

	close(blocked) // release blocked calls
}

func (s *EvmReaderSuite) TestCheckOutputFailsWhenGetOutputsFails() {
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
	apps[1].LastInputCheckBlock = 0x11
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
	apps[1].LastInputCheckBlock = 0x12
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
	).Return(nil).Times(5)
	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(5)

	s.client.EnqueueNewHead(0x11).Once()
	s.client.EnqueueNewHead(0x12).Once()
	s.client.EnqueueNewHead(0x13).Once()
	called, blocked := newBlockedCallNotification(s.client.EnqueueNewHead(0x13))

	// Start service
	go s.evmReader.Serve(s.ctx) //nolint: errcheck

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.repository.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	s.repository.AssertExpectations(s.T())

	s.inputBox.AssertExpectations(s.T())
	s.applicationContract1.AssertExpectations(s.T())
	s.applicationContract2.AssertExpectations(s.T())
	s.contractFactory.AssertExpectations(s.T())
	s.client.AssertExpectations(s.T())

	close(blocked) // release blocked calls
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	daveOpenScan   = "open"
	daveSealedScan = "sealed"
)

func TestDaveInputStoreErrorClassification(t *testing.T) {
	const head uint64 = 100
	for _, sealed := range []bool{true, false} {
		path := daveOpenScan
		if sealed {
			path = daveSealedScan
		}
		for _, test := range []struct {
			name    string
			cause   error
			corrupt bool
		}{
			{name: "identity conflict", cause: repository.ErrInputLogIdentityConflict, corrupt: true},
			{name: "retryable database error", cause: errors.New("database unavailable")},
		} {
			t.Run(path+"/"+test.name, func(t *testing.T) {
				repo := newMockRepository()
				r := &Service{repository: repo}
				r.Logger = testLogger(t)
				app := appContracts{
					application: &model.Application{
						ID: 1, Name: "input-store-app", Status: model.ApplicationStatus_OK,
						IApplicationAddress: app1Addr, IConsensusAddress: consensusAddr,
						IInputBoxBlock: head, LastEpochCheckBlock: head - 1, LastInputCheckBlock: head - 1,
					},
				}
				inputEvent := makeInputEvent(app1Addr, 0, head)
				input := &model.Input{
					Index:           0,
					BlockNumber:     head,
					RawData:         inputEvent.Input,
					Status:          model.InputCompletionStatus_None,
					TransactionHash: inputEvent.Raw.TxHash,
				}
				storeErr := fmt.Errorf("insert inputs: %w", test.cause)

				if sealed {
					tournament := common.HexToAddress("0x1234")
					dave := newMockDaveConsensus()
					app.daveConsensus = dave
					app.application.LastEpochCheckBlock = 0
					dave.On("GetDeploymentBlockNumber", blockRange(head, head+1)).
						Return(new(big.Int).SetUint64(head), nil).Once()
					repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).Return(nil, nil).Once()
					dave.On("GetCurrentSealedEpoch", blockRange(head, head+1)).
						Return(makeSealedEpochResult(0, 0, 1, tournament), nil).Twice()
					dave.On("RetrieveSealedEpochs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
						return opts.Start == head && opts.End != nil && *opts.End == head
					})).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
						makeSealedEpochEvent(0, 0, 1, head, tournament),
					}, nil).Once()
					repo.On("ListObservedInputs", t.Context(), app1Addr.Hex(), uint64(0), uint64(1)).
						Return([]*model.Input{input}, nil).Once()
					repo.On("AttributeSealedEpoch", t.Context(), app1Addr.Hex(),
						mock.Anything, mock.Anything).Return(storeErr).Once()
				} else {
					repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(&model.Epoch{
						Index: 0, LastBlock: head - 1, Status: model.EpochStatus_Closed,
					}, nil).Once()
					repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(nil, nil).Once()
					repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID, model.MonitoredEvent_InputAdded).
						Return(head-1, nil).Once()
					repo.On("GetInputObservation", t.Context(), app1Addr.Hex()).
						Return(&model.InputObservation{ThroughBlock: head, NextInputIndex: 1}, nil).Once()
					repo.On("ListObservedInputs", t.Context(), app1Addr.Hex(), uint64(0), uint64(1)).
						Return([]*model.Input{input}, nil).Once()
					repo.On("ProjectDaveOpenInputs", t.Context(), app1Addr.Hex(),
						mock.Anything, mock.Anything, head).Return(storeErr).Once()
				}
				if test.corrupt {
					repo.On("UpdateApplicationStatus", t.Context(), app.application.ID,
						model.ApplicationStatus_Corrupted, mock.Anything).Return(nil).Once()
				}

				var err error
				if sealed {
					err = r.processApplicationSealedEpochs(t.Context(), app, head)
				} else {
					observation, obsErr := repo.GetInputObservation(t.Context(), app1Addr.Hex())
					require.NoError(t, obsErr)
					err = r.projectDaveOpenEpoch(t.Context(), app, head, observation)
				}
				require.ErrorContains(t, err, storeErr.Error())
				if test.corrupt {
					require.Equal(t, model.ApplicationStatus_Corrupted, app.application.Status)
					require.ErrorContains(t, err, path+" epoch")
					require.ErrorContains(t, err, "operator reset required")
				} else {
					require.ErrorIs(t, err, test.cause)
					require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
					repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				}
				if sealed {
					require.Zero(t, app.application.LastEpochCheckBlock)
				} else {
					require.Equal(t, head-1, app.application.LastEpochCheckBlock)
				}
				require.Equal(t, head-1, app.application.LastInputCheckBlock)
				repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertExpectations(t)
				if sealed {
					app.daveConsensus.(*MockDaveConsensus).AssertExpectations(t)
				}
			})
		}
	}
}

func TestDaveRecordedIntegrityFaultKeepsOtherApplicationsScanning(t *testing.T) {
	const head uint64 = 100
	for _, path := range []string{daveSealedScan, daveOpenScan} {
		for _, test := range []struct {
			name           string
			status         model.ApplicationStatus
			statusWriteErr error
		}{
			{name: "new fault", status: model.ApplicationStatus_OK},
			{name: "already corrupted", status: model.ApplicationStatus_Corrupted},
			{name: "already diverged", status: model.ApplicationStatus_Diverged},
			{name: "escalate halted machine", status: model.ApplicationStatus_MachineHalted},
			{name: "failed integrity write", status: model.ApplicationStatus_OK, statusWriteErr: errors.New("write unavailable")},
			{name: "failed terminal escalation", status: model.ApplicationStatus_MachineHalted,
				statusWriteErr: errors.New("write unavailable")},
		} {
			t.Run(path+"/"+test.name, func(t *testing.T) {
				repo := newMockRepository()
				inputBox := newMockInputBox()
				dave := newMockDaveConsensus()
				r := &Service{repository: repo}
				r.Logger = testLogger(t)
				bad := newDaveAppContracts(inputBox, dave)
				bad.application.Status = test.status
				bad.application.IInputBoxBlock = head
				bad.application.LastEpochCheckBlock = head
				bad.application.LastInputCheckBlock = head - 1
				good := newDaveAppContracts(inputBox, nil)
				good.application.ID = 2
				good.application.IApplicationAddress = app2Addr
				good.application.Status = model.ApplicationStatus_OK
				good.application.LastEpochCheckBlock = head
				good.application.LastInputCheckBlock = head - 1

				// Observation already covers the head for both applications and
				// finds nothing pending. The fault is in the epoch store.
				repo.On("GetInputObservation", t.Context(), app1Addr.Hex()).
					Return(&model.InputObservation{ThroughBlock: head, NextInputIndex: 0}, nil).Once()
				repo.On("GetInputObservation", t.Context(), app2Addr.Hex()).
					Return(&model.InputObservation{ThroughBlock: head, NextInputIndex: 0}, nil).Once()

				if path == daveSealedScan {
					tournament := common.HexToAddress("0x1234")
					bad.application.LastEpochCheckBlock = 0
					dave.On("GetDeploymentBlockNumber", blockRange(head, head+1)).
						Return(new(big.Int).SetUint64(head), nil).Once()
					dave.On("GetCurrentSealedEpoch", blockRange(head, head+1)).
						Return(makeSealedEpochResult(0, 0, 0, tournament), nil).Twice()
					dave.On("RetrieveSealedEpochs", mock.Anything).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
						makeSealedEpochEvent(0, 0, 0, head, tournament),
					}, nil).Once()
					repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).Return(nil, nil).Once()
					repo.On("AttributeSealedEpoch", t.Context(), app1Addr.Hex(),
						mock.Anything, mock.Anything).Return(repository.ErrInputLogIdentityConflict).Once()
				} else {
					repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(&model.Epoch{
						Index: 0, LastBlock: head - 1, Status: model.EpochStatus_Closed,
					}, nil).Once()
					repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(nil, nil).Once()
					repo.On("GetEventLastCheckBlock", t.Context(), bad.application.ID, model.MonitoredEvent_InputAdded).
						Return(head-1, nil).Once()
					repo.On("ProjectDaveOpenInputs", t.Context(), app1Addr.Hex(),
						mock.Anything, mock.Anything, head).Return(repository.ErrInputLogIdentityConflict).Once()
				}

				repo.On("GetLastNonOpenEpoch", t.Context(), app2Addr.Hex()).Return(&model.Epoch{
					Index: 0, LastBlock: head - 1, Status: model.EpochStatus_Closed,
				}, nil).Once()
				repo.On("GetEpoch", t.Context(), app2Addr.Hex(), uint64(1)).Return(nil, nil).Once()
				repo.On("GetEventLastCheckBlock", t.Context(), good.application.ID, model.MonitoredEvent_InputAdded).
					Return(head-1, nil).Once()
				repo.On("ProjectDaveOpenInputs", t.Context(), app2Addr.Hex(),
					mock.Anything, mock.Anything, head).Return(nil).Once()

				integrityRecorded := test.status == model.ApplicationStatus_Corrupted || test.status == model.ApplicationStatus_Diverged
				if !integrityRecorded {
					repo.On("UpdateApplicationStatus", t.Context(), bad.application.ID, model.ApplicationStatus_Corrupted, mock.Anything).
						Return(test.statusWriteErr).Once()
				}

				require.Equal(t, test.statusWriteErr == nil,
					r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{bad, good}, head))
				if !integrityRecorded && test.statusWriteErr == nil {
					require.Equal(t, model.ApplicationStatus_Corrupted, bad.application.Status)
				} else {
					require.Equal(t, test.status, bad.application.Status)
				}
				require.Equal(t, model.ApplicationStatus_OK, good.application.Status)
				require.Equal(t, head-1, bad.application.LastInputCheckBlock)
				repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertExpectations(t)
				dave.AssertExpectations(t)
			})
		}
	}
}

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
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDaveInputStoreErrorClassification(t *testing.T) {
	const head uint64 = 100
	for _, sealed := range []bool{true, false} {
		path := "open"
		if sealed {
			path = "sealed"
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
				inputBox := newMockInputBox()
				dave := newMockDaveConsensus()
				r := &Service{
					Service:    service.Service{Logger: testLogger(t)},
					repository: repo,
				}
				app := appContracts{
					application: &model.Application{
						ID: 1, Name: "input-store-app", Status: model.ApplicationStatus_OK,
						IApplicationAddress: app1Addr, IConsensusAddress: consensusAddr,
						IInputBoxBlock: head, LastEpochCheckBlock: head - 1, LastInputCheckBlock: head - 1,
					},
					inputSource: inputBox, daveConsensus: dave,
				}
				checkpoint := head
				if sealed {
					checkpoint--
					tournament := common.HexToAddress("0x1234")
					app.application.LastEpochCheckBlock = 0
					dave.On("GetDeploymentBlockNumber", blockRange(head, head+1)).Return(new(big.Int).SetUint64(head), nil).Once()
					repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).Return(nil, nil).Once()
					dave.On("GetCurrentSealedEpoch", blockRange(head, head+1)).
						Return(makeSealedEpochResult(0, 0, 1, tournament), nil).Twice()
					dave.On("RetrieveSealedEpochs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
						return opts.Start == head && opts.End != nil && *opts.End == head
					})).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
						makeSealedEpochEvent(0, 0, 1, head, tournament),
					}, nil).Once()
				} else {
					repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(&model.Epoch{
						Index: 0, LastBlock: head - 1, Status: model.EpochStatus_Closed,
					}, nil).Once()
					repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(nil, nil).Once()
					repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID, model.MonitoredEvent_InputAdded).
						Return(head-1, nil).Once()
					repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(uint64(0), nil).Once()
				}
				inputBox.On("GetNumberOfInputs", blockRange(head, head+1), app1Addr).Return(big.NewInt(1), nil).Once()
				inputBox.On("RetrieveInputs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
					return opts.Start == head && opts.End != nil && *opts.End == head
				}), []common.Address{app1Addr}, mock.Anything).
					Return([]iinputbox.IInputBoxInputAdded{makeInputEvent(app1Addr, 0, head)}, nil).Once()
				storeErr := fmt.Errorf("insert inputs: %w", test.cause)
				repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(), mock.Anything, checkpoint).
					Return(storeErr).Once()
				if test.corrupt {
					repo.On("UpdateApplicationStatus", t.Context(), app.application.ID,
						model.ApplicationStatus_Corrupted, mock.Anything).Return(nil).Once()
				}

				var err error
				if sealed {
					err = r.processApplicationSealedEpochs(t.Context(), app, head)
				} else {
					err = r.processApplicationOpenEpoch(t.Context(), app, head)
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
				inputBox.AssertExpectations(t)
				dave.AssertExpectations(t)
			})
		}
	}
}

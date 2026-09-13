// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func (s *SealedEpochsSuite) TestDaveScanWaitsForDeploymentAndResumes() {
	const (
		beforeDeployment uint64 = 90
		deploymentBlock  uint64 = 100
	)
	var output bytes.Buffer
	s.evmReader.Logger = slog.New(slog.NewTextHandler(&output, nil))
	app := appContracts{
		application: &model.Application{
			ID: 1, Name: "new-app", Status: model.ApplicationStatus_OK,
			IApplicationAddress: app1Addr, IConsensusAddress: consensusAddr,
			IInputBoxAddress: inputBoxAddr, IInputBoxBlock: 10,
		},
		inputSource: s.inputBox, daveConsensus: s.dave,
	}
	s.dave.On("GetDeploymentBlockNumber", blockRange(beforeDeployment, beforeDeployment+1)).
		Return(new(big.Int), bind.ErrNoCode).Once()

	// The absent contract must not prevent a later application from being scanned.
	otherInputBox := newMockInputBox()
	other := appContracts{
		application: &model.Application{
			ID: 2, Name: "existing-app", Status: model.ApplicationStatus_OK,
			IApplicationAddress: app2Addr, LastEpochCheckBlock: beforeDeployment,
			LastInputCheckBlock: beforeDeployment - 1,
		},
		inputSource: otherInputBox,
	}
	s.repository.On("GetLastNonOpenEpoch", s.ctx, app2Addr.Hex()).Return(&model.Epoch{
		Index: 0, LastBlock: beforeDeployment - 1, Status: model.EpochStatus_Closed,
	}, nil).Once()
	s.repository.On("GetEpoch", s.ctx, app2Addr.Hex(), uint64(1)).Return(nil, nil).Once()
	s.repository.On("GetEventLastCheckBlock", s.ctx, other.application.ID, model.MonitoredEvent_InputAdded).
		Return(beforeDeployment-1, nil).Once()
	s.repository.On("GetNumberOfInputs", s.ctx, app2Addr.Hex()).Return(uint64(0), nil).Once()
	otherInputBox.On("GetNumberOfInputs", blockRange(beforeDeployment, beforeDeployment+1), app2Addr).
		Return(new(big.Int), nil).Once()
	s.repository.On("CreateEpochsAndInputs", s.ctx, app2Addr.Hex(), mock.Anything, beforeDeployment).
		Return(nil).Once()

	s.evmReader.scanDaveConsensusEpochsAndInputs(s.ctx, []appContracts{app, other}, beforeDeployment)
	s.Equal(model.ApplicationStatus_OK, app.application.Status)
	s.Zero(app.application.LastEpochCheckBlock)
	s.Zero(app.application.LastInputCheckBlock)
	s.repository.AssertNotCalled(s.T(), "GetLastNonOpenEpoch", s.ctx, app1Addr.Hex())
	s.repository.AssertNumberOfCalls(s.T(), "UpdateApplicationStatus", 0)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	s.repository.AssertCalled(s.T(), "CreateEpochsAndInputs", s.ctx, app2Addr.Hex(), mock.Anything, beforeDeployment)
	s.Empty(s.inputBox.Calls)
	s.NotContains(output.String(), "level=ERROR")
	s.repository.AssertExpectations(s.T())
	s.dave.AssertExpectations(s.T())
	otherInputBox.AssertExpectations(s.T())

	// The configured head now includes deployment. The same registered app must
	// ingest the constructor's sealed epoch and then create its open epoch.
	tournament := common.HexToAddress("0x1234")
	s.dave.On("GetDeploymentBlockNumber", blockRange(deploymentBlock, deploymentBlock+1)).
		Return(new(big.Int).SetUint64(deploymentBlock), nil).Once()
	s.repository.On("UpdateEventLastCheckBlock", s.ctx, []int64{app.application.ID},
		model.MonitoredEvent_EpochSealed, deploymentBlock).Return(nil).Once()
	sealed := &model.Epoch{
		Index: 0, FirstBlock: app.application.IInputBoxBlock, LastBlock: deploymentBlock,
		Status: model.EpochStatus_Closed, TournamentAddress: &tournament,
	}
	s.repository.On("GetLastNonOpenEpoch", s.ctx, app1Addr.Hex()).Return(sealed, nil).Once()
	s.repository.On("GetEpoch", s.ctx, app1Addr.Hex(), uint64(0)).Return(nil, nil).Once()
	s.repository.On("GetEpoch", s.ctx, app1Addr.Hex(), uint64(1)).Return(nil, nil).Once()
	s.dave.On("GetCurrentSealedEpoch", blockRange(deploymentBlock, deploymentBlock+1)).
		Return(makeSealedEpochResult(0, 0, 0, tournament), nil).Twice()
	s.dave.On("RetrieveSealedEpochs", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == deploymentBlock && opts.End != nil && *opts.End == deploymentBlock
	})).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
		makeSealedEpochEvent(0, 0, 0, deploymentBlock, tournament),
	}, nil).Once()
	s.repository.On("CreateEpochsAndInputs", s.ctx, app1Addr.Hex(),
		mock.MatchedBy(func(epochs map[*model.Epoch][]*model.Input) bool {
			for epoch := range epochs {
				return len(epochs) == 1 && epoch.Index == 0 && epoch.Status == model.EpochStatus_Closed
			}
			return false
		}), deploymentBlock-1).Return(nil).Once()
	s.repository.On("GetEventLastCheckBlock", s.ctx, app.application.ID, model.MonitoredEvent_InputAdded).
		Return(deploymentBlock-1, nil).Once()
	s.repository.On("GetNumberOfInputs", s.ctx, app1Addr.Hex()).Return(uint64(0), nil).Once()
	s.inputBox.On("GetNumberOfInputs", blockRange(deploymentBlock, deploymentBlock+1), app1Addr).
		Return(new(big.Int), nil).Once()
	s.repository.On("CreateEpochsAndInputs", s.ctx, app1Addr.Hex(),
		mock.MatchedBy(func(epochs map[*model.Epoch][]*model.Input) bool {
			for epoch := range epochs {
				return len(epochs) == 1 && epoch.Index == 1 && epoch.Status == model.EpochStatus_Open
			}
			return false
		}), deploymentBlock).Return(nil).Once()

	s.evmReader.scanDaveConsensusEpochsAndInputs(s.ctx, []appContracts{app}, deploymentBlock)
	s.Equal(model.ApplicationStatus_OK, app.application.Status)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateApplicationStatus", 0)
	s.repository.AssertExpectations(s.T())
	s.dave.AssertExpectations(s.T())
	s.inputBox.AssertExpectations(s.T())
}

func TestFailedSealedInitializationWaitsForDeploymentAgain(t *testing.T) {
	const deploymentBlock uint64 = 100
	for _, regressedHead := range []uint64{80, deploymentBlock - 1} {
		t.Run(new(big.Int).SetUint64(regressedHead).String(), func(t *testing.T) {
			repo := newMockRepository()
			dave := newMockDaveConsensus()
			inputBox := newMockInputBox()
			var output bytes.Buffer
			r := &Service{
				Service:    service.Service{Logger: slog.New(slog.NewTextHandler(&output, nil))},
				repository: repo,
			}
			app := newDaveAppContracts(inputBox, dave)
			app.application.IInputBoxBlock = 10
			app.application.Status = model.ApplicationStatus_OK
			tournament := common.HexToAddress("0x1234")
			dave.On("GetDeploymentBlockNumber", blockRange(deploymentBlock, deploymentBlock+1)).
				Return(new(big.Int).SetUint64(deploymentBlock), nil).Twice()
			dave.On("GetCurrentSealedEpoch", blockRange(deploymentBlock, deploymentBlock+1)).
				Return(makeSealedEpochResult(0, 0, 0, tournament), nil)
			filter := mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == deploymentBlock && opts.End != nil && *opts.End == deploymentBlock
			})
			dave.On("RetrieveSealedEpochs", filter).
				Return([]*idaveconsensus.IDaveConsensusEpochSealed{}, errors.New("log retrieval interrupted")).Once()

			r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, deploymentBlock)
			require.Empty(t, repo.Calls, "a failed first scan must not persist the deployment floor")
			require.Zero(t, app.application.LastEpochCheckBlock)
			require.NotContains(t, output.String(), "sync initialized")
			require.Contains(t, output.String(), "log retrieval interrupted")
			output.Reset()

			dave.On("GetDeploymentBlockNumber", blockRange(regressedHead, regressedHead+1)).
				Return(new(big.Int), bind.ErrNoCode).Once()
			r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, regressedHead)
			require.Empty(t, repo.Calls, "the open scan must not run before deployment is visible again")
			require.Zero(t, app.application.LastEpochCheckBlock)
			require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
			require.Empty(t, output.String(), "ordinary deployment lag needs no Info or Error log")

			dave.On("RetrieveSealedEpochs", filter).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
				makeSealedEpochEvent(0, 0, 0, deploymentBlock, tournament),
			}, nil).Once()
			repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).Return(nil, nil).Once()
			repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(), mock.Anything, deploymentBlock-1).Return(nil).Once()
			repo.On("UpdateEventLastCheckBlock", t.Context(), []int64{app.application.ID},
				model.MonitoredEvent_EpochSealed, deploymentBlock).Return(nil).Once()
			repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(&model.Epoch{
				Index: 0, FirstBlock: 10, LastBlock: deploymentBlock, Status: model.EpochStatus_Closed,
			}, nil).Once()
			repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).Return(nil, nil).Once()
			repo.On("GetEventLastCheckBlock", t.Context(), app.application.ID, model.MonitoredEvent_InputAdded).
				Return(deploymentBlock-1, nil).Once()
			repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(uint64(0), nil).Once()
			inputBox.On("GetNumberOfInputs", blockRange(deploymentBlock, deploymentBlock+1), app1Addr).
				Return(new(big.Int), nil).Once()
			repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(), mock.Anything, deploymentBlock).Return(nil).Once()

			r.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, deploymentBlock)
			require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
			require.Contains(t, output.String(), "sync initialized")
			require.NotContains(t, output.String(), "level=ERROR")
			repo.AssertExpectations(t)
			dave.AssertExpectations(t)
			inputBox.AssertExpectations(t)
		})
	}
}

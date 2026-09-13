// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIConsensusPredeploymentObservationDoesNotCorrupt(t *testing.T) {
	const (
		head        uint64 = 90
		epochLength uint64 = 10
	)
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		for _, test := range []struct {
			name          string
			inputBoxBlock uint64
		}{
			{name: "InputBox not yet visible", inputBoxBlock: 100},
			{name: "existing InputBox with no inputs", inputBoxBlock: 10},
		} {
			t.Run(consensus.String()+"/"+test.name, func(t *testing.T) {
				repo := newMockRepository()
				inputBox := newMockInputBox()
				applicationContract := newMockApplicationContract()
				r := &Service{
					Service:    service.Service{Logger: testLogger(t)},
					repository: repo,
				}
				app := appContracts{
					application: &model.Application{
						ID: 1, Name: "new-app", Enabled: true, Status: model.ApplicationStatus_OK, ConsensusType: consensus,
						IApplicationAddress: app1Addr, IConsensusAddress: consensusAddr,
						IInputBoxAddress: inputBoxAddr, IInputBoxBlock: test.inputBoxBlock, EpochLength: epochLength,
					},
					applicationContract: applicationContract, inputSource: inputBox,
				}
				// Registration saw the application at latest, but neither foreclosure
				// nor output observation can see its deployment at this configured head.
				applicationContract.On("GetDeploymentBlockNumber", blockRange(head, head+1)).
					Return(new(big.Int), bind.ErrNoCode).Twice()
				repo.On("UpdateEventLastCheckBlock", t.Context(), []int64{app.application.ID},
					model.MonitoredEvent_InputAdded, test.inputBoxBlock-1).Return(nil).Once()
				if test.inputBoxBlock <= head {
					repo.On("GetNumberOfInputs", t.Context(), app1Addr.Hex()).Return(uint64(0), nil).Once()
					inputBox.On("GetNumberOfInputs", blockRange(test.inputBoxBlock, head+1), app1Addr).
						Return(new(big.Int), nil).Twice()
					repo.On("GetEpoch", t.Context(), app1Addr.Hex(), (test.inputBoxBlock-1)/epochLength).Return(nil, nil).Once()
					repo.On("UpdateEventLastCheckBlock", t.Context(), []int64{app.application.ID},
						model.MonitoredEvent_InputAdded, head).Return(nil).Once()
				}

				r.runBlockScanners(t.Context(), []appContracts{app}, head)
				require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
				require.Zero(t, app.application.LastOutputCheckBlock)
				require.Zero(t, app.application.LastForecloseCheckBlock)
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "CreateEpochsAndInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				inputBox.AssertNotCalled(t, "RetrieveInputs", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertExpectations(t)
				inputBox.AssertExpectations(t)
				applicationContract.AssertExpectations(t)
			})
		}
	}
}

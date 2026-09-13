// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDaveReadinessDistinguishesDeploymentWaitFromScanFailure(t *testing.T) {
	const head uint64 = 100
	for _, test := range []struct {
		name           string
		deploymentWait bool
		initialStatus  model.ApplicationStatus
		statusWriteErr error
	}{
		{name: "deployment not yet visible", deploymentWait: true},
		{name: "missing sealed epoch"},
		{name: "existing integrity failure", initialStatus: model.ApplicationStatus_Diverged},
		{name: "corruption status write fails", statusWriteErr: errors.New("database unavailable")},
		{name: "execution terminal escalation fails", initialStatus: model.ApplicationStatus_MachineHalted,
			statusWriteErr: errors.New("database unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			dave := newMockDaveConsensus()
			client := newMockEthClient()
			app := &model.Application{
				ID: 1, Name: "dave-readiness", Enabled: true, Status: model.ApplicationStatus_OK,
				ConsensusType: model.Consensus_PRT, IApplicationAddress: app1Addr,
				IConsensusAddress: consensusAddr, IInputBoxAddress: inputBoxAddr,
				LastEpochCheckBlock: head, LastInputCheckBlock: head - 1,
				LastOutputCheckBlock: head, LastForecloseCheckBlock: head,
			}
			if test.initialStatus != "" {
				app.Status = test.initialStatus
			}
			client.On("HeaderByNumber", mock.Anything, mock.Anything).
				Return(&types.Header{Number: new(big.Int).SetUint64(head)}, nil)
			repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
				Return([]*model.Application{app}, uint64(1), nil)
			adapters := newMockAdapterFactory()
			adapters.On("CreateAdapters", app).
				Return(newMockApplicationContract(), newMockInputBox(), dave, nil).Once()
			if test.deploymentWait {
				app.LastEpochCheckBlock = 0
				dave.On("GetDeploymentBlockNumber", blockRange(head, head+1)).Return(new(big.Int), bind.ErrNoCode)
			} else {
				repo.On("GetLastNonOpenEpoch", t.Context(), app1Addr.Hex()).Return(nil, nil)
				if app.Status != model.ApplicationStatus_Diverged {
					statusWrite := repo.On("UpdateApplicationStatus", t.Context(), app.ID,
						model.ApplicationStatus_Corrupted, mock.Anything).Return(test.statusWriteErr)
					if test.statusWriteErr == nil {
						statusWrite.Once()
					}
				}
			}
			reader := &Service{client: client, repository: repo,
				defaultBlock: model.DefaultBlock_Finalized, readyMaxStaleness: time.Hour}
			reader.Logger = testLogger(t)
			reader.resolver = newApplicationAdapterResolver(reader.Logger, adapters)

			for cycle := 1; cycle <= maxConsecutiveScanFailures+1; cycle++ {
				reschedule, err := reader.Tick(t.Context())
				require.NoError(t, err)
				require.False(t, reschedule)
				require.Equal(t, test.statusWriteErr == nil || cycle < maxConsecutiveScanFailures, reader.Ready())
			}
			if test.deploymentWait {
				require.Zero(t, reader.consecutiveScanFailures.Load())
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				repo.AssertNotCalled(t, "GetLastNonOpenEpoch", mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			} else {
				if test.statusWriteErr == nil {
					require.Zero(t, reader.consecutiveScanFailures.Load())
					if test.initialStatus == model.ApplicationStatus_Diverged {
						require.Equal(t, test.initialStatus, app.Status)
					} else {
						require.Equal(t, model.ApplicationStatus_Corrupted, app.Status)
					}
				} else {
					require.EqualValues(t, maxConsecutiveScanFailures, reader.consecutiveScanFailures.Load())
					if test.initialStatus != "" {
						require.Equal(t, test.initialStatus, app.Status)
					} else {
						require.Equal(t, model.ApplicationStatus_OK, app.Status)
					}
				}
			}
			repo.AssertExpectations(t)
			dave.AssertExpectations(t)
			client.AssertExpectations(t)
			adapters.AssertExpectations(t)
		})
	}
}

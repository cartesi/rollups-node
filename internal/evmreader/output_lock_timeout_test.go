// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOutputExecutionLockWaitPreservesReadiness(t *testing.T) {
	const head uint64 = 100
	for _, lockWait := range []bool{true, false} {
		t.Run(fmt.Sprintf("lock_wait=%t", lockWait), func(t *testing.T) {
			repo := newMockRepository()
			client := newMockEthClient()
			contract := newMockApplicationContract()
			app := &model.Application{
				ID: 1, Name: "output-lock", Enabled: true, Status: model.ApplicationStatus_OK,
				ConsensusType: model.Consensus_Authority, IApplicationAddress: app1Addr,
				IConsensusAddress: consensusAddr, IInputBoxAddress: inputBoxAddr,
				LastInputCheckBlock: head, LastOutputCheckBlock: head - 1, LastForecloseCheckBlock: head,
			}
			client.On("HeaderByNumber", mock.Anything, mock.Anything).
				Return(&types.Header{Number: new(big.Int).SetUint64(head)}, nil)
			repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
				Return([]*model.Application{app}, uint64(1), nil)
			adapters := newMockAdapterFactory()
			adapters.On("CreateAdapters", app).Return(contract, newMockInputBox(), nil, nil).Once()
			repo.On("GetNumberOfPendingExecutableOutputs", mock.Anything, app1Addr.Hex()).Return(uint64(1), nil)
			repo.On("GetNumberOfExecutedOutputs", mock.Anything, app1Addr.Hex()).Return(uint64(0), nil)
			contract.On("GetNumberOfExecutedOutputs", blockFrom(head)).Return(big.NewInt(1), nil)
			event := *outputExecution0
			event.Raw.BlockNumber = head
			contract.On("RetrieveOutputExecutionEvents", mock.Anything).
				Return([]*iapplication.IApplicationOutputExecuted{&event}, nil)
			output := &model.Output{Index: event.OutputIndex, RawData: bytes.Clone(event.Output)}
			repo.On("GetOutput", mock.Anything, app1Addr.Hex(), event.OutputIndex).Return(output, nil)
			storeErr := errors.New("connection lost")
			if lockWait {
				storeErr = fmt.Errorf("output lock: %w", repository.ErrLockTimeout)
			}
			repo.On("UpdateOutputsExecution", mock.Anything, app1Addr.Hex(), mock.Anything, head).
				Return(storeErr).Times(maxConsecutiveScanFailures)
			var logs bytes.Buffer
			reader := &Service{client: client, repository: repo,
				defaultBlock: model.DefaultBlock_Finalized, readyMaxStaleness: time.Hour}
			reader.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			reader.resolver = newApplicationAdapterResolver(reader.Logger, adapters)
			for cycle := 1; cycle <= maxConsecutiveScanFailures; cycle++ {
				reschedule, err := reader.Tick(t.Context())
				require.NoError(t, err)
				require.False(t, reschedule)
				if lockWait {
					require.Zero(t, reader.consecutiveScanFailures.Load())
				} else {
					require.EqualValues(t, cycle, reader.consecutiveScanFailures.Load())
				}
				require.Equal(t, lockWait || cycle < maxConsecutiveScanFailures, reader.Ready())
				require.Equal(t, head-1, app.LastOutputCheckBlock)
			}
			if lockWait {
				require.Equal(t, maxConsecutiveScanFailures,
					strings.Count(logs.String(), "output execution update waits for a lock; retrying at the next tick"))
				require.Contains(t, logs.String(), `"level":"WARN"`)
				require.NotContains(t, logs.String(), `"level":"ERROR"`)
			} else {
				require.Contains(t, logs.String(), `"level":"ERROR"`)
			}
			repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
			contract.AssertExpectations(t)
			adapters.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

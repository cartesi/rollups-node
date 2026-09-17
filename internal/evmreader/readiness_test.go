// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"testing"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReadinessTracksScanFailures(t *testing.T) {
	for _, failure := range []string{"application list", "input counter", "input logs", "input cursor", "input store", "output query", "foreclosure query", "drive cursor", "withdrawal store"} {
		t.Run(failure, func(t *testing.T) {
			app := &Application{
				ID: 1, Name: "app", Enabled: true, IApplicationAddress: app1Addr, IInputBoxAddress: inputBoxAddr,
				DataAvailability: DataAvailability_InputBox[:], EpochLength: 10,
				Status: ApplicationStatus_OK, LastInputCheckBlock: 100,
				LastOutputCheckBlock: 110, LastForecloseCheckBlock: 110,
			}
			repo := newMockRepository()
			input := newMockInputBox()
			contract := newMockApplicationContract()
			client := newMockEthClient()
			client.On("HeaderByNumber", mock.Anything, mock.Anything).Return(&types.Header{Number: big.NewInt(110)}, nil)
			list := repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
				Return([]*Application{app}, uint64(1), nil)
			count := input.On("GetNumberOfInputs", mock.Anything, mock.Anything).Return(big.NewInt(0), nil)
			repo.On("GetNumberOfInputs", mock.Anything, mock.Anything).Return(uint64(0), nil)
			repo.On("GetEpoch", mock.Anything, mock.Anything, mock.Anything).Return((*Epoch)(nil), nil)
			cursor := repo.On("UpdateEventLastCheckBlock", mock.Anything, mock.Anything, MonitoredEvent_InputAdded, uint64(110)).Return(nil)
			failureErr := errors.New("persistent scan failure")
			var failedCall *mock.Call
			var recovered []any
			switch failure {
			case "application list":
				failedCall, recovered = list, []any{[]*Application{app}, uint64(1), nil}
				list.Return([]*Application(nil), uint64(0), failureErr)
			case "input counter":
				failedCall, recovered = count, []any{big.NewInt(0), nil}
				count.Return((*big.Int)(nil), failureErr)
			case "input logs", "input store":
				count.Return(big.NewInt(1), nil)
				events := []iinputbox.IInputBoxInputAdded{makeInputEvent(app1Addr, 0, 101)}
				logs := input.On("RetrieveInputs", mock.Anything, mock.Anything, mock.Anything).Return(events, nil)
				store := repo.On("CreateEpochsAndInputs", mock.Anything, mock.Anything, mock.Anything, uint64(110)).Return(nil)
				if failure == "input logs" {
					failedCall, recovered = logs, []any{events, nil}
					logs.Return([]iinputbox.IInputBoxInputAdded(nil), failureErr)
				} else {
					failedCall, recovered = store, []any{nil}
					store.Return(failureErr)
				}
			case "input cursor":
				failedCall, recovered = cursor, []any{nil}
				cursor.Return(failureErr)
			case "output query":
				app.LastOutputCheckBlock = 100
				repo.Unset("GetNumberOfPendingExecutableOutputs")
				failedCall = repo.On("GetNumberOfPendingExecutableOutputs", mock.Anything, mock.Anything).Return(uint64(0), failureErr)
				recovered = []any{uint64(0), nil}
			case "drive cursor":
				app.ForecloseBlock = 100
				contract.On("GetAccountsDriveMerkleRoot", mock.Anything).Return(false, common.Hash{}, nil)
				failedCall = repo.On("UpdateApplicationLastAccountsDriveProvedCheckBlock", mock.Anything, mock.Anything, mock.Anything).Return(failureErr)
				recovered = []any{nil}
			case "withdrawal store":
				app.ForecloseBlock = 100
				app.AccountsDriveProvedBlock = 100
				contract.On("GetNumberOfWithdrawals", mock.Anything).Return(big.NewInt(0), nil)
				failedCall = repo.On("StoreWithdrawalEvents", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(failureErr)
				recovered = []any{nil}
			case "foreclosure query":
				app.LastForecloseCheckBlock = 100
				contract.Unset("IsForeclosed")
				failedCall = contract.On("IsForeclosed", mock.Anything).Return(false, failureErr)
				recovered = []any{false, nil}
				repo.On("UpdateApplicationLastForecloseCheckBlock", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			}
			r := &Service{client: client, repository: repo, inputReaderEnabled: true,
				defaultBlock: DefaultBlock_Latest, readyMaxStaleness: time.Hour}
			r.Logger = slog.Default()
			r.resolver = newApplicationAdapterResolver(r.Logger, newMockAdapterFactory().SetupDefaultBehaviorSingleApp(contract, input))
			require.False(t, r.Ready())
			for i := 1; i <= maxConsecutiveScanFailures+1; i++ {
				_, err := r.Tick(t.Context())
				require.NoError(t, err)
				require.Equal(t, i < maxConsecutiveScanFailures, r.Ready())
			}
			// Even a successful header cannot clear the failure streak.
			require.EqualValues(t, maxConsecutiveScanFailures, r.consecutiveScanFailures.Load())
			failedCall.Return(recovered...)
			_, err := r.Tick(t.Context())
			require.NoError(t, err)
			require.True(t, r.Ready())
			require.Zero(t, r.consecutiveScanFailures.Load())

			// A canceled scan must neither count as recovery nor refresh staleness.
			before := *r.lastSuccessfulPoll.Load()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err = r.Tick(ctx)
			require.NoError(t, err)
			require.Equal(t, before, *r.lastSuccessfulPoll.Load())
		})
	}
}

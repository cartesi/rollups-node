// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDaveOpenInputViewRetriesFromSameCursorWithoutPublishingIncompleteInputs(t *testing.T) {
	const head uint64 = 100
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing all logs", true: "missing last log"}[partial], func(t *testing.T) {
			repo, inputBox := newMockRepository(), newMockInputBox()
			app := newDaveAppContracts(inputBox, nil)
			app.application.LastEpochCheckBlock = head
			app.application.LastInputCheckBlock = head - 1
			app.application.Status = model.ApplicationStatus_OK
			sealed := &model.Epoch{Index: 0, LastBlock: head - 2, Status: model.EpochStatus_Closed}
			var open *model.Epoch
			stored, total := uint64(0), uint64(1)
			var firstLogs []iinputbox.IInputBoxInputAdded
			if partial {
				stored, total = 1, 3
				open = &model.Epoch{Index: 1, FirstBlock: sealed.LastBlock, LastBlock: head - 1,
					InputIndexUpperBound: stored, Status: model.EpochStatus_Open}
				firstLogs = []iinputbox.IInputBoxInputAdded{makeInputEvent(app1Addr, stored, head)}
			}
			var output bytes.Buffer
			reader := &Service{repository: repo}
			reader.Logger = slog.New(slog.NewJSONHandler(&output, nil))
			repo.On("GetLastNonOpenEpoch", mock.Anything, app1Addr.Hex()).Return(sealed, nil).Twice()
			repo.On("GetEpoch", mock.Anything, app1Addr.Hex(), uint64(1)).Return(open, nil).Twice()
			repo.On("GetEventLastCheckBlock", mock.Anything, app.application.ID, model.MonitoredEvent_InputAdded).
				Return(head-1, nil).Twice()
			repo.On("GetNumberOfInputs", mock.Anything, app1Addr.Hex()).Return(stored, nil).Twice()
			inputBox.On("GetNumberOfInputs", inputCountBlock(head), app1Addr).
				Return(new(big.Int).SetUint64(total), nil).Twice()
			inputBox.On("RetrieveInputs", inputLogBlock(head), []common.Address{app1Addr}, mock.Anything).
				Return(firstLogs, nil).Once()
			require.False(t, reader.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, head))
			require.Contains(t, output.String(), `"level":"WARN"`)
			require.NotContains(t, output.String(), `"level":"ERROR"`)
			require.Contains(t, output.String(), "Open epoch input reads disagree; retrying from the saved cursor")
			for _, context := range []string{
				app.application.Name, strings.ToLower(app1Addr.Hex()), `"from_block":99`, `"to_block":100`,
				"stored", "on-chain", "got",
			} {
				require.Contains(t, output.String(), context)
			}
			require.Equal(t, head-1, app.application.LastInputCheckBlock)
			if open != nil {
				require.Equal(t, stored, open.InputIndexUpperBound)
				require.Equal(t, head-1, open.LastBlock)
			}
			repo.AssertNotCalled(t, "CreateEpochsAndInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			require.Equal(t, model.ApplicationStatus_OK, app.application.Status)

			// The next response is complete at the same head. The persisted cursor
			// still selects the original block, including the missing input.
			inputBox.Unset("RetrieveInputs")
			var complete []iinputbox.IInputBoxInputAdded
			for index := stored; index < total; index++ {
				complete = append(complete, makeInputEvent(app1Addr, index, head))
			}
			inputBox.On("RetrieveInputs", inputLogBlock(head), []common.Address{app1Addr}, mock.Anything).
				Return(complete, nil).Once()
			repo.On("CreateEpochsAndInputs", mock.Anything, app1Addr.Hex(), mock.Anything, head).
				Run(func(args mock.Arguments) {
					for epoch, inputs := range args.Get(2).(map[*model.Epoch][]*model.Input) {
						require.Equal(t, uint64(1), epoch.Index)
						require.Equal(t, total, epoch.InputIndexUpperBound)
						require.Len(t, inputs, len(complete))
					}
					app.application.LastInputCheckBlock = head
				}).Return(nil).Once()
			output.Reset()
			require.True(t, reader.scanDaveConsensusEpochsAndInputs(t.Context(), []appContracts{app}, head))
			require.Equal(t, head, app.application.LastInputCheckBlock)
			require.NotContains(t, output.String(), `"level":"ERROR"`)
			repo.AssertNumberOfCalls(t, "CreateEpochsAndInputs", 1)
			repo.AssertNumberOfCalls(t, "UpdateEventLastCheckBlock", 0)
			inputBox.AssertExpectations(t)
			repo.AssertExpectations(t)
		})
	}
}

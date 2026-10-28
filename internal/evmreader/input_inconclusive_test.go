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
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIncompleteInputViewRetriesWithoutWritingOrAdvancingCursor(t *testing.T) {
	for _, test := range []struct {
		name       string
		stored     uint64
		startCount int64
		endCount   uint64
		firstLogs  []iinputbox.IInputBoxInputAdded
	}{
		{name: "counter includes next block input", startCount: 0, endCount: 1},
		{name: "counter includes third input outside filter", stored: 1, startCount: 2, endCount: 3,
			firstLogs: []iinputbox.IInputBoxInputAdded{makeInputEvent(app1Addr, 1, 101)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := appContracts{application: &model.Application{
				ID: 1, Name: "retry-input-view", IApplicationAddress: app1Addr, IInputBoxAddress: inputBoxAddr,
				IInputBoxBlock: 1, EpochLength: 10, LastInputCheckBlock: 100,
				Status: model.ApplicationStatus_OK, Enabled: true,
			}}
			repo, input := newMockRepository(), newMockInputBox()
			app.inputSource = input
			var output bytes.Buffer
			reader := &Service{repository: repo, defaultBlock: model.DefaultBlock_Latest}
			reader.Logger = slog.New(slog.NewJSONHandler(&output, nil))
			repo.On("GetNumberOfInputs", mock.Anything, app1Addr.Hex()).Return(test.stored, nil).Twice()
			// The counter includes an input outside this scan's end block. The
			// logs correctly cover only the requested range.
			badCount := input.On("GetNumberOfInputs", mock.Anything, app1Addr).Return(new(big.Int).SetUint64(test.endCount), nil)
			partial := input.On("RetrieveInputs", mock.Anything, mock.Anything, mock.Anything).Return(test.firstLogs, nil).Once()
			err := reader.readAndStoreApplicationInputs(t.Context(), 100, 101, app)
			require.ErrorIs(t, err, errInconclusiveInputView)
			reader.reportInputScanError(t.Context(), app, 100, 101, err)
			require.Contains(t, output.String(), `"level":"WARN"`)
			require.NotContains(t, output.String(), `"level":"ERROR"`)
			for _, context := range []string{
				`"application":"retry-input-view"`, strings.ToLower(app1Addr.Hex()),
				`"last_processed_block":100`, `"most_recent_block":101`, "stored", "on-chain", "got",
			} {
				require.Contains(t, output.String(), context)
			}
			repo.AssertNotCalled(t, "GetEpoch", mock.Anything, mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "CreateEpochsAndInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			require.EqualValues(t, 100, app.application.LastInputCheckBlock)
			require.Equal(t, model.ApplicationStatus_OK, app.application.Status)
			badCount.Unset()
			partial.Unset()
			input.On("GetNumberOfInputs", inputCountBlock(101), app1Addr).Return(big.NewInt(test.startCount), nil)
			input.On("GetNumberOfInputs", inputCountBlock(102), app1Addr).Return(new(big.Int).SetUint64(test.endCount), nil)
			if len(test.firstLogs) != 0 {
				input.On("RetrieveInputs", inputLogBlock(101), mock.Anything, mock.Anything).Return(test.firstLogs, nil).Once()
			}
			lastLog := makeInputEvent(app1Addr, test.endCount-1, 102)
			input.On("RetrieveInputs", inputLogBlock(102), mock.Anything, mock.Anything).
				Return([]iinputbox.IInputBoxInputAdded{lastLog}, nil).Once()
			repo.On("GetEpoch", mock.Anything, app1Addr.Hex(), uint64(10)).Return(&model.Epoch{
				Index: 10, FirstBlock: 100, LastBlock: 109, Status: model.EpochStatus_Open,
			}, nil).Once()
			repo.On("CreateEpochsAndInputs", mock.Anything, app1Addr.Hex(), mock.Anything, uint64(102)).
				Run(func(args mock.Arguments) {
					var count uint64
					for _, inputs := range args.Get(2).(map[*model.Epoch][]*model.Input) {
						count += uint64(len(inputs))
					}
					require.Equal(t, test.endCount-test.stored, count)
				}).Return(nil).Once()
			require.True(t, reader.scanIConsensusInputs(t.Context(), []appContracts{app}, 102))
			repo.AssertNumberOfCalls(t, "CreateEpochsAndInputs", 1)
			repo.AssertNumberOfCalls(t, "UpdateEventLastCheckBlock", 0)
			input.AssertExpectations(t)
			repo.AssertExpectations(t)
		})
	}
}

func inputCountBlock(block uint64) any {
	return mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber != nil && opts.BlockNumber.Uint64() == block
	})
}

func inputLogBlock(block uint64) any {
	return mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == block && opts.End != nil && *opts.End == block
	})
}

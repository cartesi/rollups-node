// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	iorFirstScanBlock    = uint64(20)
	iorMismatchCondition = "output_execution_mismatch"
)

func iorExecutionEvent(index, block uint64, data []byte) *iapplication.IApplicationOutputExecuted {
	return &iapplication.IApplicationOutputExecuted{
		OutputIndex: index, Output: data,
		Raw: types.Log{BlockNumber: block, TxHash: common.BigToHash(new(big.Int).SetUint64(index + 1))},
	}
}

func expectIORScanBlock(contract *MockApplicationContract, block, count uint64,
	events []*iapplication.IApplicationOutputExecuted) {
	contract.On("GetNumberOfExecutedOutputs", blockFrom(block)).Return(new(big.Int).SetUint64(count), nil).Once()
	contract.On("RetrieveOutputExecutionEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == block && opts.End != nil && *opts.End == block
	})).Return(events, nil).Once()
}

func TestIOROutputMismatchDoesNotBlockLaterScans(t *testing.T) {
	repo := newMockRepository()
	contract := newMockApplicationContract()
	var logs bytes.Buffer
	reader := &Service{repository: repo}
	reader.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	app := &model.Application{ID: 1, Name: "scan-recovery", Enabled: true, Status: model.ApplicationStatus_OK}
	observedApp := appContracts{application: app, applicationContract: contract}
	mismatch := &model.Output{Index: 0, RawData: []byte("local-output")}
	matching := &model.Output{Index: 1, RawData: []byte("matching-output")}
	later := &model.Output{Index: 2, RawData: []byte("later-output")}
	different := iorExecutionEvent(mismatch.Index, iorFirstScanBlock, []byte("chain-output"))
	matchingEvent := iorExecutionEvent(matching.Index, iorFirstScanBlock, matching.RawData)
	laterEvent := iorExecutionEvent(later.Index, iorFirstScanBlock+2, later.RawData)
	cursor := iorFirstScanBlock - 1
	executedCount := uint64(0)
	repo.On("GetNumberOfExecutedOutputs", mock.Anything, app.IApplicationAddress.Hex()).
		Return(uint64(0), nil).Once()
	repo.On("GetNumberOfExecutedOutputs", mock.Anything, app.IApplicationAddress.Hex()).
		Return(uint64(1), nil).Twice()
	repo.On("GetOutput", mock.Anything, app.IApplicationAddress.Hex(), mismatch.Index).Return(mismatch, nil).Once()
	repo.On("GetOutput", mock.Anything, app.IApplicationAddress.Hex(), matching.Index).Return(matching, nil).Once()
	repo.On("GetOutput", mock.Anything, app.IApplicationAddress.Hex(), later.Index).Return(later, nil).Once()
	expectIORScanBlock(contract, iorFirstScanBlock, 2, []*iapplication.IApplicationOutputExecuted{different, matchingEvent})
	// The local executed count is one behind the chain because index 0 stays
	// unexecuted. FindTransitions synthesizes a hit at this next start block,
	// whose canonical filter correctly returns no new events.
	expectIORScanBlock(contract, iorFirstScanBlock+1, 2, nil)
	expectIORScanBlock(contract, iorFirstScanBlock+2, 3, []*iapplication.IApplicationOutputExecuted{laterEvent})
	repo.On("UpdateOutputsExecution", mock.Anything, app.IApplicationAddress.Hex(), []*model.Output{matching}, iorFirstScanBlock).
		Run(func(args mock.Arguments) { cursor = args.Get(3).(uint64); executedCount++ }).Return(nil).Once()
	repo.On("UpdateEventLastCheckBlock", mock.Anything, []int64{app.ID},
		model.MonitoredEvent_OutputExecuted, iorFirstScanBlock+1).
		Run(func(args mock.Arguments) { cursor = args.Get(3).(uint64) }).Return(nil).Once()
	repo.On("UpdateOutputsExecution", mock.Anything, app.IApplicationAddress.Hex(), []*model.Output{later}, iorFirstScanBlock+2).
		Run(func(args mock.Arguments) { cursor = args.Get(3).(uint64); executedCount++ }).Return(nil).Once()

	for block := iorFirstScanBlock; block <= iorFirstScanBlock+2; block++ {
		require.True(t, reader.readAndUpdateOutputs(t.Context(), observedApp, cursor, block))
		require.Equal(t, block, cursor)
		require.Nil(t, mismatch.ExecutionTransactionHash, "different bytes must never be marked executed")
		require.Equal(t, model.ApplicationStatus_OK, app.Status)
	}
	require.Equal(t, uint64(2), executedCount)
	require.Equal(t, matchingEvent.Raw.TxHash, *matching.ExecutionTransactionHash)
	require.Equal(t, laterEvent.Raw.TxHash, *later.ExecutionTransactionHash)
	require.Contains(t, logs.String(), iorMismatchCondition)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	contract.AssertExpectations(t)
}

func TestIOROutputMismatchRetryKeepsCursorAndBoundsDiagnostics(t *testing.T) {
	const (
		payloadSize           = 64 * 1024
		maximumDiagnosticSize = 2048
		attempts              = 2
	)
	repo := newMockRepository()
	contract := newMockApplicationContract()
	var logs bytes.Buffer
	reader := &Service{repository: repo}
	reader.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	app := &model.Application{ID: 1, Name: "retry-diagnostic", Enabled: true, Status: model.ApplicationStatus_OK}
	observedApp := appContracts{application: app, applicationContract: contract}
	localData := bytes.Repeat([]byte("a"), payloadSize)
	chainData := bytes.Repeat([]byte("b"), payloadSize)
	mismatch := &model.Output{Index: 0, RawData: localData}
	event := iorExecutionEvent(mismatch.Index, iorFirstScanBlock, chainData)
	missing := iorExecutionEvent(1, iorFirstScanBlock, []byte("not-stored-yet"))
	repo.On("GetNumberOfExecutedOutputs", mock.Anything, app.IApplicationAddress.Hex()).Return(uint64(0), nil).Times(attempts)
	repo.On("GetOutput", mock.Anything, app.IApplicationAddress.Hex(), mismatch.Index).Return(mismatch, nil).Times(attempts)
	repo.On("GetOutput", mock.Anything, app.IApplicationAddress.Hex(), missing.OutputIndex).
		Return((*model.Output)(nil), nil).Times(attempts)
	for range attempts {
		expectIORScanBlock(contract, iorFirstScanBlock, 2, []*iapplication.IApplicationOutputExecuted{event, missing})
	}
	cursor := iorFirstScanBlock - 1
	for range attempts {
		require.False(t, reader.readAndUpdateOutputs(t.Context(), observedApp, cursor, iorFirstScanBlock))
		require.Equal(t, iorFirstScanBlock-1, cursor)
		require.Nil(t, mismatch.ExecutionTransactionHash)
	}
	decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
	mismatchRecords := 0
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		if record["condition"] != iorMismatchCondition {
			continue
		}
		mismatchRecords++
		require.Equal(t, "WARN", record["level"])
		require.Equal(t, float64(payloadSize), record["local_output_size"])
		require.Equal(t, float64(payloadSize), record["chain_output_size"])
		require.Equal(t, crypto.Keccak256Hash(localData).Hex(), record["local_output_hash"])
		require.Equal(t, crypto.Keccak256Hash(chainData).Hex(), record["chain_output_hash"])
		require.Equal(t, float64(iorFirstScanBlock), record["event_block"])
		require.Equal(t, event.Raw.TxHash.Hex(), record["event_transaction"])
		encoded, err := json.Marshal(record)
		require.NoError(t, err)
		require.Less(t, len(encoded), maximumDiagnosticSize)
	}
	require.Equal(t, attempts, mismatchRecords, "a retry can repeat a bounded diagnostic")
	require.NotContains(t, logs.String(), string(localData))
	require.NotContains(t, logs.String(), string(chainData))
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	repo.AssertNotCalled(t, "UpdateOutputsExecution", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "UpdateEventLastCheckBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	contract.AssertExpectations(t)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package replay

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	summary       model.ReplaySummary
	summaryErr    error
	records       []*model.ReplayRecord
	pageErr       error
	pageRequests  []repository.ReplayPageRequest
	summaryLevels []repository.ReplayVerificationLevel
	summaryApps   []common.Address
	pageOverride  func(repository.ReplayPageRequest) ([]*model.ReplayRecord, error)
}

func (source *fakeSource) ReplaySummary(
	_ context.Context,
	applicationAddress common.Address,
	verification repository.ReplayVerificationLevel,
) (model.ReplaySummary, error) {
	source.summaryApps = append(source.summaryApps, applicationAddress)
	source.summaryLevels = append(source.summaryLevels, verification)
	return source.summary, source.summaryErr
}

func (source *fakeSource) ReplayPage(
	_ context.Context,
	request repository.ReplayPageRequest,
) ([]*model.ReplayRecord, error) {
	source.pageRequests = append(source.pageRequests, request)
	if source.pageOverride != nil {
		return source.pageOverride(request)
	}
	if source.pageErr != nil {
		return nil, source.pageErr
	}
	page := make([]*model.ReplayRecord, 0, request.Limit)
	for _, record := range source.records {
		if record == nil || record.Input.InputIndex >= request.FromInput &&
			record.Input.InputIndex < request.ToInputExclusive {
			page = append(page, record)
			if uint64(len(page)) == request.Limit {
				break
			}
		}
	}
	return page, nil
}

type fakeExecutor struct {
	processed      uint64
	advanceErr     error
	advanceCalls   []model.ReplayInput
	computeHashes  []bool
	wrongResultPos bool
	fullPRTResult  bool
}

func (executor *fakeExecutor) ProcessedInputs() uint64 { return executor.processed }

func (executor *fakeExecutor) Advance(
	_ context.Context,
	data []byte,
	epochIndex uint64,
	inputIndex uint64,
	computeHashes bool,
) (*model.AdvanceResult, error) {
	if executor.advanceErr != nil {
		return nil, executor.advanceErr
	}
	executor.advanceCalls = append(executor.advanceCalls, model.ReplayInput{
		EpochIndex: epochIndex,
		InputIndex: inputIndex,
		RawData:    data,
	})
	executor.computeHashes = append(executor.computeHashes, computeHashes)
	executor.processed++
	resultIndex := inputIndex
	if executor.wrongResultPos {
		resultIndex++
	}
	result := &model.AdvanceResult{
		EpochIndex: epochIndex,
		InputIndex: resultIndex,
		Status:     model.InputCompletionStatus_Accepted,
		OutputsProof: model.OutputsProof{
			MachineHash: common.BigToHash(newBig(inputIndex + 1)),
			OutputsHash: common.BigToHash(newBig(inputIndex + 100)),
		},
	}
	if executor.fullPRTResult {
		result.PaddingRepetitions = 1 << 24
	}
	return result, nil
}

func newBig(value uint64) *big.Int { return new(big.Int).SetUint64(value) }

func replayRecords(count uint64) []*model.ReplayRecord {
	records := make([]*model.ReplayRecord, count)
	for index := range count {
		machineHash := common.BigToHash(newBig(index + 1))
		outputsHash := common.BigToHash(newBig(index + 100))
		records[index] = &model.ReplayRecord{Input: model.ReplayInput{
			ApplicationID: 7,
			EpochIndex:    index / 2,
			InputIndex:    index,
			RawData:       []byte{byte(index)},
			Status:        model.InputCompletionStatus_Accepted,
			MachineHash:   &machineHash,
			OutputsHash:   &outputsHash,
		}}
	}
	return records
}

func replayOptions(consensus model.Consensus, from, to uint64) Options {
	return Options{
		Application: &model.Application{
			ID:                  7,
			Name:                "app",
			IApplicationAddress: common.HexToAddress("0x1234"),
			ConsensusType:       consensus,
			ProcessedInputs:     to,
		},
		FromInput:        from,
		ToInputExclusive: to,
		BatchSize:        2,
		Verification:     repository.ReplayVerificationCanonical,
	}
}

func TestRunCanonicalRangeAndPagination(t *testing.T) {
	records := replayRecords(5)
	source := &fakeSource{
		summary: model.ReplaySummary{ApplicationID: 7, ProcessedInputs: 5, Consensus: model.Consensus_PRT},
		records: records,
	}
	executor := &fakeExecutor{processed: 2}
	opts := replayOptions(model.Consensus_PRT, 2, 5)

	result, err := Run(context.Background(), source, executor, opts)
	require.NoError(t, err)
	require.Equal(t, uint64(3), result.ReplayedInputs)
	require.Equal(t, uint64(5), executor.ProcessedInputs())
	require.Equal(t, []bool{false, false, false}, executor.computeHashes)
	require.Equal(t, []common.Address{opts.Application.IApplicationAddress}, source.summaryApps)
	require.Equal(t, []repository.ReplayPageRequest{
		{ApplicationID: 7, FromInput: 2, ToInputExclusive: 5, Limit: 2, Verification: repository.ReplayVerificationCanonical},
		{ApplicationID: 7, FromInput: 4, ToInputExclusive: 5, Limit: 1, Verification: repository.ReplayVerificationCanonical},
	}, source.pageRequests)
}

func TestRunSelectsReplaySummaryByTypedAddress(t *testing.T) {
	source := &fakeSource{
		summary: model.ReplaySummary{
			ApplicationID: 7, ProcessedInputs: 0, Consensus: model.Consensus_Authority,
		},
	}
	opts := replayOptions(model.Consensus_Authority, 0, 0)
	// A valid application name may itself look like another application's
	// address. Replay identity must never be inferred from that string.
	opts.Application.Name = common.HexToAddress("0x9999").String()

	_, err := Run(context.Background(), source, &fakeExecutor{}, opts)
	require.NoError(t, err)
	require.Equal(t, []common.Address{opts.Application.IApplicationAddress}, source.summaryApps)
}

func TestRunFullPRTComputesHashes(t *testing.T) {
	record := replayRecords(1)[0]
	record.StateHashes = []model.ReplayStateHash{{MachineHash: *record.Input.MachineHash, Repetitions: 1 << 24}}
	source := &fakeSource{
		summary: model.ReplaySummary{ApplicationID: 7, ProcessedInputs: 1, Consensus: model.Consensus_PRT},
		records: []*model.ReplayRecord{record},
	}
	executor := &fakeExecutor{fullPRTResult: true}
	opts := replayOptions(model.Consensus_PRT, 0, 1)
	opts.Verification = repository.ReplayVerificationFull

	_, err := Run(context.Background(), source, executor, opts)
	require.NoError(t, err)
	require.Equal(t, []bool{true}, executor.computeHashes)
}

func TestRunCaughtUpStillValidatesSummary(t *testing.T) {
	source := &fakeSource{
		summary: model.ReplaySummary{ApplicationID: 7, ProcessedInputs: 2, Consensus: model.Consensus_Authority},
	}
	executor := &fakeExecutor{processed: 2}
	opts := replayOptions(model.Consensus_Authority, 2, 2)

	result, err := Run(context.Background(), source, executor, opts)
	require.NoError(t, err)
	require.Zero(t, result.ReplayedInputs)
	require.Equal(t, []repository.ReplayVerificationLevel{repository.ReplayVerificationCanonical}, source.summaryLevels)
	require.Empty(t, source.pageRequests)
}

func TestRunRejectsMalformedPagesBeforeExecution(t *testing.T) {
	tests := []struct {
		name    string
		records []*model.ReplayRecord
	}{
		{name: "empty", records: nil},
		{name: "nil record", records: []*model.ReplayRecord{nil}},
		{name: "gap", records: replayRecords(2)[1:]},
		{name: "oversized", records: replayRecords(3)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := &fakeSource{
				summary: model.ReplaySummary{ApplicationID: 7, ProcessedInputs: 2, Consensus: model.Consensus_Authority},
				records: test.records,
			}
			if test.name == "oversized" {
				source.pageOverride = func(repository.ReplayPageRequest) ([]*model.ReplayRecord, error) {
					return test.records, nil
				}
			}
			executor := &fakeExecutor{}
			opts := replayOptions(model.Consensus_Authority, 0, 2)

			_, err := Run(context.Background(), source, executor, opts)
			require.ErrorIs(t, err, ErrContradiction)
			require.Empty(t, executor.advanceCalls)
		})
	}
}

func TestRunErrorClassification(t *testing.T) {
	opts := replayOptions(model.Consensus_Authority, 0, 1)
	t.Run("source transport", func(t *testing.T) {
		sourceErr := errors.New("database unavailable")
		source := &fakeSource{summaryErr: sourceErr}
		_, err := Run(context.Background(), source, &fakeExecutor{}, opts)
		require.ErrorIs(t, err, ErrExecution)
		require.ErrorIs(t, err, sourceErr)
		require.NotErrorIs(t, err, ErrContradiction)
	})
	t.Run("source structure", func(t *testing.T) {
		source := &fakeSource{summaryErr: &repository.ReplayStructureViolationError{
			Kind:       repository.ReplayStructureCompletedInputSequence,
			InputIndex: 0,
		}}
		_, err := Run(context.Background(), source, &fakeExecutor{}, opts)
		require.ErrorIs(t, err, ErrContradiction)
	})
	t.Run("executor", func(t *testing.T) {
		executionErr := errors.New("emulator unavailable")
		source := &fakeSource{
			summary: model.ReplaySummary{ApplicationID: 7, ProcessedInputs: 1, Consensus: model.Consensus_Authority},
			records: replayRecords(1),
		}
		_, err := Run(context.Background(), source, &fakeExecutor{advanceErr: executionErr}, opts)
		require.ErrorIs(t, err, ErrExecution)
		require.ErrorIs(t, err, executionErr)
	})
}

func TestRunRejectsInvalidOptions(t *testing.T) {
	source := &fakeSource{}
	executor := &fakeExecutor{}
	opts := replayOptions(model.Consensus_Authority, 0, 1)
	tests := []struct {
		name   string
		source repository.ReplayRepository
		exec   Executor
		mutate func(*Options)
	}{
		{"nil source", nil, executor, func(*Options) {}},
		{"nil executor", source, nil, func(*Options) {}},
		{"nil app", source, executor, func(o *Options) { o.Application = nil }},
		{"invalid level", source, executor, func(o *Options) { o.Verification = 255 }},
		{"zero batch", source, executor, func(o *Options) { o.BatchSize = 0 }},
		{"inverted range", source, executor, func(o *Options) { o.FromInput = 2 }},
		{"upper beyond app", source, executor, func(o *Options) { o.ToInputExclusive = 2 }},
		{"executor position", source, &fakeExecutor{processed: 1}, func(*Options) {}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := opts
			test.mutate(&current)
			_, err := Run(context.Background(), test.source, test.exec, current)
			require.ErrorIs(t, err, ErrInvalidOptions)
		})
	}
}

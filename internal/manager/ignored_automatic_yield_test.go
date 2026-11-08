// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/stretchr/testify/require"
)

type ignoredYieldLogEntry struct {
	Message         string `json:"msg"`
	Level           string
	Application     string
	Request         string
	EpochIndex      *uint64 `json:"epoch_index"`
	InputIndex      *uint64 `json:"input_index"`
	ProcessedInputs *uint64 `json:"processed_inputs"`
	Yields          struct {
		Reasons []uint16
		Counts  []uint64
	} `json:"ignored_automatic_yields"`
}

func readIgnoredYieldLogs(t *testing.T, output *bytes.Buffer) []ignoredYieldLogEntry {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	var entries []ignoredYieldLogEntry
	for {
		var entry ignoredYieldLogEntry
		err := decoder.Decode(&entry)
		if errors.Is(err, io.EOF) {
			return entries
		}
		require.NoError(t, err)
		if entry.Message == "Ignored unknown automatic yields" {
			entries = append(entries, entry)
		}
	}
}

func (s *MachineInstanceSuite) TestAdvanceLogsIgnoredAutomaticYields() {
	for _, test := range []struct {
		name   string
		status machine.CompletionStatus
	}{
		{"accepted", machine.CompletionStatusAccepted},
		{"rejected", machine.CompletionStatusRejected},
		{"invalid root", machine.CompletionStatusInvalidOutputsRoot},
	} {
		for _, hashes := range []bool{false, true} {
			s.Run(fmt.Sprintf("%s/hashes=%t", test.name, hashes), func() {
				check := s.Require()
				_, baselineFork, baseline := s.setupAdvance()
				_, fork, instance := s.setupAdvance()
				for _, runtime := range []*MockRollupsMachine{baselineFork, fork} {
					runtime.CompletionStatusReturn = test.status
					runtime.CloseError = nil
				}
				fork.AdvanceIgnoredYieldsReturn = []machine.IgnoredAutomaticYield{
					{Reason: 8, Count: 3}, {Reason: 127, Count: 1},
				}
				var baselineOutput, output bytes.Buffer
				baseline.logger = slog.New(slog.NewJSONHandler(&baselineOutput, nil))
				instance.logger = slog.New(slog.NewJSONHandler(&output, nil))
				baseline.logger.Info("Unrelated advance log")
				instance.logger.Info("Unrelated advance log")

				want, err := baseline.Advance(s.T().Context(), []byte("input"), 12, 5, hashes)
				check.NoError(err)
				got, err := instance.Advance(s.T().Context(), []byte("input"), 12, 5, hashes)
				check.NoError(err)
				check.Equal(want, got)
				check.Equal(baseline.ProcessedInputs(), instance.ProcessedInputs())
				check.Equal(baseline.Application(), instance.Application())
				check.Empty(readIgnoredYieldLogs(s.T(), &baselineOutput))

				entries := readIgnoredYieldLogs(s.T(), &output)
				check.Len(entries, 1)
				entry := entries[0]
				check.Equal("WARN", entry.Level)
				check.Equal("advance", entry.Request)
				check.NotNil(entry.EpochIndex)
				check.Equal(uint64(12), *entry.EpochIndex)
				check.NotNil(entry.InputIndex)
				check.Equal(uint64(5), *entry.InputIndex)
				check.Nil(entry.ProcessedInputs)
				check.Equal([]uint16{8, 127}, entry.Yields.Reasons)
				check.Equal([]uint64{3, 1}, entry.Yields.Counts)
			})
		}
	}
}

func (s *MachineInstanceSuite) TestAdvanceLogsIgnoredYieldsBeforeResultValidation() {
	for _, test := range []struct {
		name      string
		proofRead bool
	}{
		{name: "proof read", proofRead: true},
		{name: "final entry"},
	} {
		s.Run(test.name, func() {
			check := s.Require()
			inner, fork, instance := s.setupAdvance()
			fork.CloseError = nil
			fork.AdvanceIgnoredYieldsReturn = []machine.IgnoredAutomaticYield{{Reason: 127, Count: 2}, {Reason: 8, Count: 3}}
			proofErr := errors.New("post-run proof read failed")
			proofCalls := 0
			fork.StateProofFunc = func(context.Context) (*machine.StateProof, error) {
				proofCalls++
				if test.proofRead && proofCalls == 2 {
					return nil, proofErr
				}
				return fork.StateProofReturn, nil
			}
			wrongEntry := machine.Hash{}
			fork.AdvanceFinalStateEntryReturn = &wrongEntry
			var output bytes.Buffer
			instance.logger = slog.New(slog.NewJSONHandler(&output, nil))
			result, err := instance.Advance(s.T().Context(), []byte("input"), 12, 5, true)
			if test.proofRead {
				check.ErrorIs(err, proofErr)
			} else {
				check.ErrorContains(err, "collector final-state entry")
			}
			check.Nil(result)
			check.Equal(2, proofCalls)
			check.Same(inner, instance.runtime)
			check.Equal(uint64(5), instance.ProcessedInputs())
			check.Zero(inner.CloseCalls.Load())
			check.Equal(int64(1), fork.CloseCalls.Load())
			entries := readIgnoredYieldLogs(s.T(), &output)
			check.Len(entries, 1)
			entry := entries[0]
			check.Equal("WARN", entry.Level)
			check.Equal("advance", entry.Request)
			check.NotNil(entry.EpochIndex)
			check.Equal(uint64(12), *entry.EpochIndex)
			check.NotNil(entry.InputIndex)
			check.Equal(uint64(5), *entry.InputIndex)
			check.Nil(entry.ProcessedInputs)
			check.Equal([]uint16{127, 8}, entry.Yields.Reasons)
			check.Equal([]uint64{2, 3}, entry.Yields.Counts)
		})
	}
}

func (s *MachineInstanceSuite) TestInspectLogsIgnoredAutomaticYields() {
	for _, test := range []struct {
		name         string
		executionErr error
		closeErr     error
	}{
		{name: "completed"},
		{name: "execution error", executionErr: errors.New("inspect stopped")},
		{name: "close error", closeErr: errors.New("inspect close failed")},
	} {
		for _, processed := range []uint64{0, 55} {
			s.Run(fmt.Sprintf("%s/processed=%d", test.name, processed), func() {
				check := s.Require()
				_, baselineFork, baseline := s.setupInspect()
				_, fork, instance := s.setupInspect()
				for _, runtime := range []*MockRollupsMachine{baselineFork, fork} {
					runtime.InspectError = test.executionErr
					if test.executionErr != nil {
						runtime.InspectResponseReturn.Status = machine.CompletionStatusUnknown
					}
					runtime.CloseError = test.closeErr
				}
				baseline.processedInputs.Store(processed)
				instance.processedInputs.Store(processed)
				fork.InspectResponseReturn.IgnoredAutomaticYields = []machine.IgnoredAutomaticYield{
					{Reason: 0, Count: 2}, {Reason: 65535, Count: 1},
				}
				var baselineOutput, output bytes.Buffer
				baseline.logger = slog.New(slog.NewJSONHandler(&baselineOutput, nil))
				instance.logger = slog.New(slog.NewJSONHandler(&output, nil))
				baseline.logger.Info("Unrelated inspect log")
				instance.logger.Info("Unrelated inspect log")

				want, wantErr := baseline.Inspect(s.T().Context(), []byte("query"))
				got, err := instance.Inspect(s.T().Context(), []byte("query"))
				check.Equal(wantErr, err)
				check.Equal(want, got)
				if test.executionErr != nil {
					check.NotNil(got)
					check.Equal(machine.CompletionStatusUnknown, got.Status)
					check.ErrorIs(got.Error, test.executionErr)
				}
				check.Equal(processed, instance.ProcessedInputs())
				check.NotSame(fork, instance.runtime)
				check.Empty(readIgnoredYieldLogs(s.T(), &baselineOutput))

				entries := readIgnoredYieldLogs(s.T(), &output)
				check.Len(entries, 1)
				entry := entries[0]
				check.Equal("WARN", entry.Level)
				check.Equal("inspect", entry.Request)
				check.NotNil(entry.ProcessedInputs)
				check.Equal(processed, *entry.ProcessedInputs)
				check.Nil(entry.EpochIndex)
				check.Nil(entry.InputIndex)
				check.Equal([]uint16{0, 65535}, entry.Yields.Reasons)
				check.Equal([]uint64{2, 1}, entry.Yields.Counts)
			})
		}
	}
}

func (s *MachineInstanceSuite) TestIgnoredYieldLogsUseCapturedInspectPrefix() {
	check := s.Require()
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	innerA, advanceA, appA := s.setupAdvance()
	_, advanceB, appB := s.setupAdvance()
	appA.processedInputs.Store(0)
	appB.processedInputs.Store(0)
	appA.inspectTimeout = 2 * time.Second
	appA.logger = logger.With("application", "A")
	appB.logger = logger.With("application", "B")
	advanceA.AdvanceIgnoredYieldsReturn = []machine.IgnoredAutomaticYield{{Reason: 8, Count: 2}}
	advanceB.AdvanceIgnoredYieldsReturn = []machine.IgnoredAutomaticYield{{Reason: 65535, Count: 3}}

	started, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	inspectFork := &MockRollupsMachine{
		InspectFunc: func(ctx context.Context, _ []byte) (*machine.InspectResponse, error) {
			close(started)
			select {
			case <-release:
				return &machine.InspectResponse{
					Status:                 machine.CompletionStatusAccepted,
					Reports:                []machine.Report{[]byte("captured-prefix")},
					IgnoredAutomaticYields: []machine.IgnoredAutomaticYield{{Reason: 127, Count: 1}},
				}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	var forks atomic.Int64
	innerA.ForkFunc = func(context.Context) (machine.Machine, error) {
		if forks.Add(1) == 1 {
			return inspectFork, nil
		}
		return advanceA, nil
	}
	type inspectOutcome struct {
		result *InspectResult
		err    error
	}
	done := make(chan inspectOutcome, 1)
	ctx := s.T().Context()
	go func() {
		result, err := appA.Inspect(ctx, []byte("query at zero"))
		done <- inspectOutcome{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		s.T().Fatal("inspect did not start")
	}

	resultA, err := appA.Advance(s.T().Context(), []byte("A input"), 4, 0, false)
	check.NoError(err)
	check.NotNil(resultA)
	resultB, err := appB.Advance(s.T().Context(), []byte("B input"), 9, 0, false)
	check.NoError(err)
	check.NotNil(resultB)
	check.Equal(uint64(1), appA.ProcessedInputs())
	check.Equal(uint64(1), appB.ProcessedInputs())
	check.Equal(int64(1), innerA.CloseCalls.Load())
	check.Zero(inspectFork.CloseCalls.Load())
	unblock()

	select {
	case outcome := <-done:
		check.NoError(outcome.err)
		check.NotNil(outcome.result)
		check.NoError(outcome.result.Error)
		check.Equal(uint64(0), outcome.result.ProcessedInputs)
		check.Equal([]machine.Report{[]byte("captured-prefix")}, outcome.result.Reports)
	case <-time.After(2 * time.Second):
		s.T().Fatal("inspect did not finish")
	}
	check.Equal(int64(2), forks.Load())
	check.Equal(int64(1), inspectFork.CloseCalls.Load())
	check.Same(advanceA, appA.runtime)
	check.Same(advanceB, appB.runtime)

	entries := readIgnoredYieldLogs(s.T(), &output)
	check.Len(entries, 3)
	check.Equal("A", entries[0].Application)
	check.Equal("advance", entries[0].Request)
	check.Equal([]uint16{8}, entries[0].Yields.Reasons)
	check.Equal([]uint64{2}, entries[0].Yields.Counts)
	check.Equal("B", entries[1].Application)
	check.Equal("advance", entries[1].Request)
	check.Equal([]uint16{65535}, entries[1].Yields.Reasons)
	check.Equal([]uint64{3}, entries[1].Yields.Counts)
	check.Equal("A", entries[2].Application)
	check.Equal("inspect", entries[2].Request)
	check.NotNil(entries[2].ProcessedInputs)
	check.Equal(uint64(0), *entries[2].ProcessedInputs)
	check.Nil(entries[2].EpochIndex)
	check.Nil(entries[2].InputIndex)
	check.Equal([]uint16{127}, entries[2].Yields.Reasons)
	check.Equal([]uint64{1}, entries[2].Yields.Counts)
}

func (s *MachineInstanceSuite) TestIgnoredYieldTextLogNamesReasonCounts() {
	check := s.Require()
	_, fork, instance := s.setupAdvance()
	fork.AdvanceIgnoredYieldsReturn = []machine.IgnoredAutomaticYield{
		{Reason: 8, Count: 3}, {Reason: 127, Count: 1},
	}
	var output bytes.Buffer
	instance.logger = slog.New(slog.NewTextHandler(&output, nil))

	_, err := instance.Advance(s.T().Context(), []byte("input"), 12, 5, false)
	check.NoError(err)
	check.Contains(output.String(), "ignored_automatic_yields.reasons=\"[8 127]\"")
	check.Contains(output.String(), "ignored_automatic_yields.counts=\"[3 1]\"")
}

func (s *MachineInstanceSuite) TestInspectCaptureFailureReleasesPermit() {
	for _, closed := range []bool{false, true} {
		s.Run(fmt.Sprintf("closed=%t", closed), func() {
			check := s.Require()
			inner, _, instance := s.setupInspect()
			forkErr := errors.New("fork failed")
			inner.ForkError = forkErr
			if closed {
				instance.runtime = nil
				forkErr = ErrMachineClosed
			}
			result, err := instance.Inspect(s.T().Context(), nil)
			check.ErrorIs(err, forkErr)
			check.Nil(result)
			permits := int64(instance.maxConcurrentInspects)
			check.True(instance.inspectSemaphore.TryAcquire(permits))
			instance.inspectSemaphore.Release(permits)
		})
	}
}

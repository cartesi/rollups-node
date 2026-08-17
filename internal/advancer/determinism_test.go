// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/machine"
	pkgservice "github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const determinismWaitTimeout = 5 * time.Second

var errDeterminismRuntimeClosed = errors.New("determinism runtime is closed")

func TestProcessInputs_RetryFromNonzeroPredecessorMatchesUninterruptedResult(t *testing.T) {
	tests := []struct {
		name          string
		targetPayload []byte
		wantStatus    model.InputCompletionStatus
	}{
		{
			name:          "accepted",
			targetPayload: []byte("deposit:alice:17"),
			wantStatus:    model.InputCompletionStatus_Accepted,
		},
		{
			name:          "rejected",
			targetPayload: []byte("reject:malformed-deposit"),
			wantStatus:    model.InputCompletionStatus_Rejected,
		},
		{
			name:          "exception",
			targetPayload: []byte("exception:application-error"),
			wantStatus:    model.InputCompletionStatus_Exception,
		},
		{
			name:          "machine halted",
			targetPayload: []byte("halt:application-finished"),
			wantStatus:    model.InputCompletionStatus_MachineHalted,
		},
		{
			name:          "mcycle overflow",
			targetPayload: []byte("overflow:cycle-ceiling"),
			wantStatus:    model.InputCompletionStatus_Overflow,
		},
		{
			name:          "unexpected yield",
			targetPayload: []byte("unexpected-yield:unknown-reason"),
			wantStatus:    model.InputCompletionStatus_UnexpectedYield,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, predecessor, target := determinismBaseline(t, tt.targetPayload)
			requireDeterminismTarget(t, prefix, target, tt.targetPayload, tt.wantStatus)

			t.Run("pre-canceled calls do not fork", func(t *testing.T) {
				testDeterminismEarlyInterruptions(t, prefix, target, tt.targetPayload)
			})
			t.Run("caller cancellation after mutation", func(t *testing.T) {
				testDeterminismCallerInterruptions(
					t, prefix, target, tt.targetPayload, context.Canceled, 1,
				)
			})
			t.Run("caller deadline reconstructs predecessor across retries", func(t *testing.T) {
				testDeterminismCallerDeadlines(
					t, prefix, predecessor, target, tt.targetPayload, 3,
				)
			})
			t.Run("infrastructure retry reconstructs predecessor", func(t *testing.T) {
				testDeterminismInfrastructureRetries(
					t, prefix, predecessor, target, tt.targetPayload, 2,
				)
			})
			for _, deadline := range []time.Duration{100 * time.Millisecond, 250 * time.Millisecond} {
				t.Run("manager watchdog "+deadline.String(), func(t *testing.T) {
					testDeterminismManagerWatchdog(
						t, prefix, predecessor, target, tt.targetPayload, deadline,
					)
				})
			}
		})
	}
}

func determinismBaseline(
	t *testing.T,
	targetPayload []byte,
) (*model.AdvanceResult, determinismMachineState, *model.AdvanceResult) {
	t.Helper()
	repo := &MockRepository{}
	harness := newTemplateDeterminismHarness(
		t, repo, determinismWaitTimeout,
		determinismAdvanceSuccess,
		determinismAdvanceSuccess,
	)

	require.NoError(t, harness.process(context.Background(), 0, []byte("deposit:prefix:11")))
	require.Len(t, repo.StoredResults, 1)
	prefix := cloneDeterminismResult(repo.StoredResults[0])
	predecessor := harness.liveState(t)
	require.Equal(t, uint64(1), predecessor.step)

	require.NoError(t, harness.process(context.Background(), 1, targetPayload))
	require.Len(t, repo.StoredResults, 2)
	target := cloneDeterminismResult(repo.StoredResults[1])
	require.Equal(t, uint64(2), harness.instance.ProcessedInputs())
	if target.Status.IsTerminal() {
		_, err := harness.instance.Hash(context.Background())
		require.ErrorIs(t, err, manager.ErrMachineClosed)
	} else {
		require.Equal(t, machine.Hash(target.MachineHash), harness.runtimeHash(t))
	}
	return prefix, predecessor, target
}

func testDeterminismEarlyInterruptions(
	t *testing.T,
	wantPrefix *model.AdvanceResult,
	wantTarget *model.AdvanceResult,
	targetPayload []byte,
) {
	t.Helper()
	repo := &MockRepository{}
	harness := newTemplateDeterminismHarness(
		t, repo, determinismWaitTimeout,
		determinismAdvanceSuccess,
		determinismAdvanceSuccess,
	)
	require.NoError(t, harness.process(context.Background(), 0, []byte("deposit:prefix:11")))
	require.Equal(t, wantPrefix, repo.StoredResults[0])
	predecessor := harness.liveRuntime(t)
	predecessorState := predecessor.snapshot()
	forksBefore := harness.factory.forkCount()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, harness.process(canceled, 1, targetPayload), context.Canceled)

	expired, cancelDeadline := context.WithDeadline(
		context.Background(), time.Now().Add(-time.Second),
	)
	defer cancelDeadline()
	require.ErrorIs(t, harness.process(expired, 1, targetPayload), context.DeadlineExceeded)

	require.Equal(t, forksBefore, harness.factory.forkCount(),
		"an already-finished caller context must be rejected before Fork")
	requireCanonicalPrefix(t, repo, wantPrefix)
	require.Equal(t, uint64(1), harness.instance.ProcessedInputs())
	require.False(t, predecessor.isClosed())
	require.Equal(t, predecessorState, harness.liveState(t))

	require.NoError(t, harness.process(context.Background(), 1, targetPayload))
	requireCanonicalHistory(t, repo, wantPrefix, wantTarget)
	requireSuccessfulRetryState(t, harness, predecessor, wantTarget)
}

func testDeterminismCallerInterruptions(
	t *testing.T,
	wantPrefix *model.AdvanceResult,
	wantTarget *model.AdvanceResult,
	targetPayload []byte,
	wantErr error,
	attempts int,
) {
	t.Helper()
	behaviors := []determinismAdvanceBehavior{determinismAdvanceSuccess}
	for range attempts {
		behaviors = append(behaviors, determinismAdvanceWaitForContext)
	}
	behaviors = append(behaviors, determinismAdvanceSuccess)
	repo := &MockRepository{}
	harness := newTemplateDeterminismHarness(
		t, repo, determinismWaitTimeout, behaviors...,
	)
	require.NoError(t, harness.process(context.Background(), 0, []byte("deposit:prefix:11")))
	require.Equal(t, wantPrefix, repo.StoredResults[0])
	predecessor := harness.liveRuntime(t)
	predecessorState := predecessor.snapshot()

	for range attempts {
		controlled := newDeterminismContext(context.Background())
		defer controlled.finish(wantErr)
		errCh := make(chan error, 1)
		go func() { errCh <- harness.process(controlled, 1, targetPayload) }()

		fork := harness.waitForMutation(t)
		controlled.finish(wantErr)
		require.ErrorIs(t, waitDeterminismError(t, errCh), wantErr)
		requireCanonicalPrefix(t, repo, wantPrefix)
		require.Equal(t, uint64(1), harness.instance.ProcessedInputs())
		require.Zero(t, repo.ApplicationStatusUpdates,
			"caller-owned interruptions must not mark the application failed")
		require.False(t, predecessor.isClosed(),
			"caller interruption must leave the reusable predecessor open")
		require.Equal(t, predecessorState, harness.liveState(t))
		requireDiscardedMutatedFork(t, fork, predecessorState)
	}

	require.NoError(t, harness.process(context.Background(), 1, targetPayload))
	requireCanonicalHistory(t, repo, wantPrefix, wantTarget)
	requireSuccessfulRetryState(t, harness, predecessor, wantTarget)
}

func testDeterminismCallerDeadlines(
	t *testing.T,
	wantPrefix *model.AdvanceResult,
	predecessor determinismMachineState,
	wantTarget *model.AdvanceResult,
	targetPayload []byte,
	attempts int,
) {
	t.Helper()
	repo := repositoryWithDeterminismPrefix(wantPrefix)

	for range attempts {
		harness := newSnapshotDeterminismHarness(
			t, repo, predecessor, determinismWaitTimeout,
			determinismAdvanceWaitForContext,
		)
		controlled := newDeterminismContext(context.Background())
		errCh := make(chan error, 1)
		go func() { errCh <- harness.process(controlled, 1, targetPayload) }()

		fork := harness.waitForMutation(t)
		controlled.finish(context.DeadlineExceeded)
		require.ErrorIs(t, waitDeterminismError(t, errCh), context.DeadlineExceeded)
		requireCanonicalPrefix(t, repo, wantPrefix)
		require.Equal(t, uint64(1), harness.instance.ProcessedInputs())
		require.Zero(t, repo.ApplicationStatusUpdates,
			"the expired context prevents the immediate FAILED status write")
		require.True(t, harness.provider.HasPendingApplicationFailures())
		require.Equal(
			t,
			context.DeadlineExceeded.Error(),
			harness.provider.failureReason(harness.app.ID),
		)
		require.Equal(t, predecessor, harness.factory.base.snapshot())
		require.True(t, harness.factory.base.isClosed(),
			"a timed-out advance must close the changed live machine")
		_, err := harness.factory.base.Fork(context.Background())
		require.ErrorIs(t, err, errDeterminismRuntimeClosed)
		requireDiscardedMutatedFork(t, fork, predecessor)
	}

	recovered := newSnapshotDeterminismHarness(
		t, repo, predecessor, determinismWaitTimeout, determinismAdvanceSuccess,
	)
	require.NoError(t, recovered.process(context.Background(), 1, targetPayload))
	requireCanonicalHistory(t, repo, wantPrefix, wantTarget)
	requireSuccessfulRetryState(t, recovered, recovered.factory.base, wantTarget)
}

func testDeterminismInfrastructureRetries(
	t *testing.T,
	wantPrefix *model.AdvanceResult,
	predecessor determinismMachineState,
	wantTarget *model.AdvanceResult,
	targetPayload []byte,
	attempts int,
) {
	t.Helper()
	repo := repositoryWithDeterminismPrefix(wantPrefix)

	for range attempts {
		harness := newSnapshotDeterminismHarness(
			t, repo, predecessor, determinismWaitTimeout,
			determinismAdvanceInfrastructureFailure,
		)
		err := harness.process(context.Background(), 1, targetPayload)
		require.ErrorIs(t, err, machine.ErrMachineInternal)
		requireCanonicalPrefix(t, repo, wantPrefix)
		require.Equal(t, uint64(1), harness.instance.ProcessedInputs())
		require.Equal(t, predecessor, harness.factory.base.snapshot())
		require.True(t, harness.factory.base.isClosed(),
			"advancer must close the failed live runtime")
		_, err = harness.factory.base.Fork(context.Background())
		require.ErrorIs(t, err, errDeterminismRuntimeClosed)
		requireDiscardedMutatedFork(t, harness.lastFork(t), predecessor)
	}
	require.Equal(t, attempts, repo.ApplicationStatusUpdates)
	require.Equal(t, model.ApplicationStatus_Failed, repo.LastApplicationStatus)

	recovered := newSnapshotDeterminismHarness(
		t, repo, predecessor, determinismWaitTimeout, determinismAdvanceSuccess,
	)
	require.NoError(t, recovered.process(context.Background(), 1, targetPayload))
	requireCanonicalHistory(t, repo, wantPrefix, wantTarget)
	requireSuccessfulRetryState(t, recovered, recovered.factory.base, wantTarget)
}

func testDeterminismManagerWatchdog(
	t *testing.T,
	wantPrefix *model.AdvanceResult,
	predecessor determinismMachineState,
	wantTarget *model.AdvanceResult,
	targetPayload []byte,
	deadline time.Duration,
) {
	t.Helper()
	repo := repositoryWithDeterminismPrefix(wantPrefix)
	harness := newSnapshotDeterminismHarness(
		t, repo, predecessor, deadline, determinismAdvanceWaitForContext,
	)
	parent := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- harness.process(parent, 1, targetPayload) }()

	err := waitDeterminismError(t, errCh)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, parent.Err(), "the manager watchdog, not the caller, must expire")
	requireCanonicalPrefix(t, repo, wantPrefix)
	require.Equal(t, uint64(1), harness.instance.ProcessedInputs())
	require.Equal(t, 1, repo.ApplicationStatusUpdates)
	require.Equal(t, model.ApplicationStatus_Failed, repo.LastApplicationStatus)
	require.Equal(t, predecessor, harness.factory.base.snapshot())
	require.True(t, harness.factory.base.isClosed())
	requireDiscardedMutatedFork(t, harness.lastFork(t), predecessor)

	recovered := newSnapshotDeterminismHarness(
		t, repo, predecessor, determinismWaitTimeout, determinismAdvanceSuccess,
	)
	require.NoError(t, recovered.process(context.Background(), 1, targetPayload))
	requireCanonicalHistory(t, repo, wantPrefix, wantTarget)
	requireSuccessfulRetryState(t, recovered, recovered.factory.base, wantTarget)
}

func requireDeterminismTarget(
	t *testing.T,
	prefix *model.AdvanceResult,
	target *model.AdvanceResult,
	targetPayload []byte,
	wantStatus model.InputCompletionStatus,
) {
	t.Helper()
	require.Equal(t, uint64(7), target.EpochIndex)
	require.Equal(t, uint64(1), target.InputIndex)
	require.Equal(t, wantStatus, target.Status)
	require.True(t, target.IsDaveConsensus)
	require.Len(t, target.PeriodicStateHashes, 2, "a PRT result must retain its periodic state hashes")
	require.Equal(t, machine.InputEntryCapacity-uint64(len(target.PeriodicStateHashes)), target.PaddingRepetitions)

	if wantStatus == model.InputCompletionStatus_Accepted {
		require.True(t, target.IsComplete())
		require.Equal(t, [][]byte{append([]byte("output:"), targetPayload...)}, target.Outputs)
		require.Equal(t, [][]byte{append([]byte("report:"), targetPayload...)}, target.Reports)
		require.NotEqual(t, prefix.MachineHash, target.MachineHash)
		require.NotEqual(t, prefix.TxBufferDataBlock, target.TxBufferDataBlock)
		return
	}

	require.Empty(t, target.Outputs, "effects are canonical only for accepted inputs")
	require.Empty(t, target.Reports, "effects are canonical only for accepted inputs")
	if wantStatus.IsTerminal() {
		require.True(t, target.IsComplete(),
			"a terminal result must preserve its actual post-run proof")
		require.NotEqual(t, prefix.MachineHash, target.MachineHash,
			"a terminal completion must preserve its post-run machine root")
		require.NotEqual(t, prefix.TxBufferDataBlock, target.TxBufferDataBlock,
			"a terminal completion must preserve its post-run TX buffer")
	} else {
		require.True(t, target.IsComplete())
		require.Equal(t, prefix.MachineHash, target.MachineHash,
			"a rejected candidate must not replace the predecessor")
	}
	if !wantStatus.IsTerminal() {
		require.Equal(t, prefix.TxBufferDataBlock, target.TxBufferDataBlock)
	}
	if !wantStatus.IsTerminal() {
		require.Equal(t, prefix.StateProof, target.StateProof)
	}
}

func requireDiscardedMutatedFork(
	t *testing.T,
	fork *determinismRuntime,
	predecessor determinismMachineState,
) {
	t.Helper()
	require.True(t, fork.isClosed(), "the mutated candidate must be discarded")
	require.NotEqual(t, predecessor.machineHash, fork.snapshot().machineHash,
		"the injected failure must happen after candidate state changes")
	_, err := fork.Hash(context.Background())
	require.ErrorIs(t, err, errDeterminismRuntimeClosed)
}

func requireSuccessfulRetryState(
	t *testing.T,
	harness *determinismHarness,
	predecessor *determinismRuntime,
	wantTarget *model.AdvanceResult,
) {
	t.Helper()
	require.Equal(t, uint64(2), harness.instance.ProcessedInputs())
	lastCandidate := harness.lastFork(t)
	if wantTarget.Status.IsTerminal() {
		_, err := harness.instance.Hash(context.Background())
		require.ErrorIs(t, err, manager.ErrMachineClosed)
		require.True(t, predecessor.isClosed())
		require.True(t, lastCandidate.isClosed(), "the terminal candidate must be disposed")
		return
	}
	require.Equal(t, machine.Hash(wantTarget.MachineHash), harness.runtimeHash(t))
	if wantTarget.Status == model.InputCompletionStatus_Accepted {
		require.True(t, predecessor.isClosed())
		require.False(t, lastCandidate.isClosed(), "the state-producing candidate must be adopted")
		return
	}
	require.False(t, predecessor.isClosed(), "rejection must keep the predecessor live")
	require.True(t, lastCandidate.isClosed(), "the rejected candidate must be discarded")
}

func requireCanonicalPrefix(
	t *testing.T,
	repo *MockRepository,
	wantPrefix *model.AdvanceResult,
) {
	t.Helper()
	require.Len(t, repo.StoredResults, 1,
		"an interrupted input must not add a canonical result")
	require.Equal(t, wantPrefix, repo.StoredResults[0])
}

func requireCanonicalHistory(
	t *testing.T,
	repo *MockRepository,
	wantPrefix *model.AdvanceResult,
	wantTarget *model.AdvanceResult,
) {
	t.Helper()
	require.Equal(t, []*model.AdvanceResult{wantPrefix, wantTarget}, repo.StoredResults)
}

func repositoryWithDeterminismPrefix(prefix *model.AdvanceResult) *MockRepository {
	return &MockRepository{StoredResults: []*model.AdvanceResult{cloneDeterminismResult(prefix)}}
}

func waitDeterminismError(t *testing.T, errCh <-chan error) error {
	t.Helper()
	select {
	case err := <-errCh:
		return err
	case <-time.After(determinismWaitTimeout):
		t.Fatal("advance did not finish before the test timeout")
		return nil
	}
}

func cloneDeterminismResult(result *model.AdvanceResult) *model.AdvanceResult {
	clone := *result
	clone.TxBufferProof = append([][32]byte(nil), result.TxBufferProof...)
	clone.IflagsYProof = append([][32]byte(nil), result.IflagsYProof...)
	clone.HtifTohostProof = append([][32]byte(nil), result.HtifTohostProof...)
	clone.Outputs = cloneDeterminismBytes(result.Outputs)
	clone.Reports = cloneDeterminismBytes(result.Reports)
	clone.ExceptionData = append([]byte(nil), result.ExceptionData...)
	clone.PeriodicStateHashes = append([][32]byte(nil), result.PeriodicStateHashes...)
	return &clone
}

func cloneDeterminismBytes(values [][]byte) [][]byte {
	if values == nil {
		return nil
	}
	clone := make([][]byte, len(values))
	for i := range values {
		clone[i] = append([]byte(nil), values[i]...)
	}
	return clone
}

type determinismHarness struct {
	app      *model.Application
	instance manager.MachineInstance
	service  *Service
	factory  *determinismRuntimeFactory
	provider *determinismMachineProvider
}

func newTemplateDeterminismHarness(
	t *testing.T,
	repo *MockRepository,
	advanceDeadline time.Duration,
	behaviors ...determinismAdvanceBehavior,
) *determinismHarness {
	t.Helper()
	return newDeterminismHarness(
		t, repo, newDeterminismMachineState(), 0, advanceDeadline, behaviors...,
	)
}

func newSnapshotDeterminismHarness(
	t *testing.T,
	repo *MockRepository,
	predecessor determinismMachineState,
	advanceDeadline time.Duration,
	behaviors ...determinismAdvanceBehavior,
) *determinismHarness {
	t.Helper()
	return newDeterminismHarness(
		t, repo, predecessor, 1, advanceDeadline, behaviors...,
	)
}

func newDeterminismHarness(
	t *testing.T,
	repo *MockRepository,
	start determinismMachineState,
	processedInputs uint64,
	advanceDeadline time.Duration,
	behaviors ...determinismAdvanceBehavior,
) *determinismHarness {
	t.Helper()
	app := &model.Application{
		ID:                  1,
		Name:                "deterministic-input-statuses",
		IApplicationAddress: common.HexToAddress("0x1"),
		ConsensusType:       model.Consensus_PRT,
		Enabled:             true,
		Status:              model.ApplicationStatus_OK,
		ProcessedInputs:     processedInputs,
		ExecutionParameters: model.ExecutionParameters{
			SnapshotPolicy:        model.SnapshotPolicy_None,
			AdvanceMaxDeadline:    advanceDeadline,
			InspectMaxDeadline:    determinismWaitTimeout,
			LoadDeadline:          determinismWaitTimeout,
			StoreDeadline:         determinismWaitTimeout,
			MaxConcurrentInspects: 1,
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	factory := newDeterminismRuntimeFactory(start, behaviors...)
	instance, err := manager.NewMachineInstanceWithFactory(
		context.Background(), app, processedInputs, logger, factory,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, instance.Close()) })
	provider := &determinismMachineProvider{app: app, instance: instance}
	service := &Service{
		Service: pkgservice.Service{
			Logger: logger,
			Cancel: func() {},
		},
		inputBatchSize: 500,
		machineManager: provider,
		repository:     repo,
	}
	return &determinismHarness{
		app: app, instance: instance, service: service, factory: factory, provider: provider,
	}
}

func (h *determinismHarness) process(ctx context.Context, index uint64, payload []byte) error {
	_, _, err := h.service.processInputs(ctx, h.app, []*model.Input{{
		EpochApplicationID: h.app.ID,
		EpochIndex:         7,
		Index:              index,
		RawData:            append([]byte(nil), payload...),
	}})
	return err
}

func (h *determinismHarness) waitForMutation(t *testing.T) *determinismRuntime {
	t.Helper()
	select {
	case fork := <-h.factory.mutated:
		return fork
	case <-time.After(determinismWaitTimeout):
		t.Fatal("candidate did not reach the injected interruption point")
		return nil
	}
}

func (h *determinismHarness) liveRuntime(t *testing.T) *determinismRuntime {
	t.Helper()
	if h.instance.ProcessedInputs() == 0 {
		return h.factory.base
	}
	return h.factory.acceptedRuntime(t)
}

func (h *determinismHarness) liveState(t *testing.T) determinismMachineState {
	t.Helper()
	return h.liveRuntime(t).snapshot()
}

func (h *determinismHarness) lastFork(t *testing.T) *determinismRuntime {
	t.Helper()
	return h.factory.lastFork(t)
}

func (h *determinismHarness) runtimeHash(t *testing.T) machine.Hash {
	t.Helper()
	hash, err := h.instance.Hash(context.Background())
	require.NoError(t, err)
	return hash
}

type determinismAdvanceBehavior uint8

const (
	determinismAdvanceSuccess determinismAdvanceBehavior = iota
	determinismAdvanceWaitForContext
	determinismAdvanceInfrastructureFailure
)

type determinismMachineState struct {
	step           uint64
	machineHash    machine.Hash
	outputsHash    machine.Hash
	checkpointHash machine.Hash
}

func newDeterminismMachineState() determinismMachineState {
	machineHash := determinismHash("base-machine")
	outputsHash := determinismHash("base-outputs")
	return determinismMachineState{
		machineHash: machineHash,
		outputsHash: outputsHash,
	}
}

func (s determinismMachineState) clone() determinismMachineState {
	return s
}

type determinismRuntimeFactory struct {
	mu        sync.Mutex
	behaviors []determinismAdvanceBehavior
	next      int
	start     determinismMachineState
	base      *determinismRuntime
	forks     []*determinismRuntime
	accepted  *determinismRuntime
	mutated   chan *determinismRuntime
}

func newDeterminismRuntimeFactory(
	start determinismMachineState,
	behaviors ...determinismAdvanceBehavior,
) *determinismRuntimeFactory {
	return &determinismRuntimeFactory{
		behaviors: append([]determinismAdvanceBehavior(nil), behaviors...),
		start:     start.clone(),
		mutated:   make(chan *determinismRuntime, len(behaviors)+1),
	}
}

func (f *determinismRuntimeFactory) CreateMachineRuntime(
	ctx context.Context,
	_ *model.Application,
	_ *slog.Logger,
) (machine.Machine, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.base = &determinismRuntime{factory: f, state: f.start.clone()}
	return f.base, nil
}

func (f *determinismRuntimeFactory) fork(state determinismMachineState) *determinismRuntime {
	f.mu.Lock()
	defer f.mu.Unlock()
	behavior := determinismAdvanceSuccess
	if f.next < len(f.behaviors) {
		behavior = f.behaviors[f.next]
	}
	f.next++
	child := &determinismRuntime{
		factory:  f,
		behavior: behavior,
		state:    state.clone(),
	}
	f.forks = append(f.forks, child)
	return child
}

func (f *determinismRuntimeFactory) recordAccepted(runtime *determinismRuntime) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accepted = runtime
}

func (f *determinismRuntimeFactory) acceptedRuntime(t *testing.T) *determinismRuntime {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotNil(t, f.accepted)
	return f.accepted
}

func (f *determinismRuntimeFactory) lastFork(t *testing.T) *determinismRuntime {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(t, f.forks)
	return f.forks[len(f.forks)-1]
}

func (f *determinismRuntimeFactory) forkCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.forks)
}

type determinismRuntime struct {
	mu       sync.Mutex
	factory  *determinismRuntimeFactory
	behavior determinismAdvanceBehavior
	state    determinismMachineState
	closed   bool
}

func (m *determinismRuntime) Fork(ctx context.Context) (machine.Machine, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkOpenLocked(ctx); err != nil {
		return nil, err
	}
	return m.factory.fork(m.state), nil
}

func (m *determinismRuntime) Hash(ctx context.Context) (machine.Hash, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkOpenLocked(ctx); err != nil {
		return machine.Hash{}, err
	}
	return m.state.machineHash, nil
}

func (m *determinismRuntime) StateProof(ctx context.Context) (*machine.StateProof, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkOpenLocked(ctx); err != nil {
		return nil, err
	}
	var iflagsYData machine.Hash
	iflagsYData[8] = 1
	var htifTohostData machine.Hash
	htifTohostData[20] = 1
	htifTohostData[22] = 1
	htifTohostData[23] = 2
	return &machine.StateProof{
		MachineHash:     m.state.machineHash,
		IflagsYProof:    determinismValidityLeaf("iflags-y", iflagsYData),
		HtifTohostProof: determinismValidityLeaf("htif-tohost", htifTohostData),
		TxBufferProof:   determinismValidityLeaf("tx-buffer", m.state.outputsHash),
	}, nil
}

func (m *determinismRuntime) Advance(
	ctx context.Context,
	input []byte,
	checkpointHash machine.Hash,
	computeHashes bool,
) (*machine.AdvanceResponse, error) {
	m.mu.Lock()
	if err := m.checkOpenLocked(ctx); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if !computeHashes {
		m.mu.Unlock()
		return nil, errors.New("determinism test requires PRT input hash collection")
	}
	if len(input) == 0 {
		m.mu.Unlock()
		return nil, errors.New("determinism test input must not be empty")
	}

	// The revert root rides with the advance request and must be the machine's
	// pre-input root — the instance always passes fork.Hash(). A mismatch is a
	// harness (or caller) bug, not a determinism scenario.
	if checkpointHash != m.state.machineHash {
		m.mu.Unlock()
		return nil, errors.New("determinism test requires the checkpoint hash to equal the pre-input machine root")
	}
	previous := m.state.clone()
	// Recording the request's own revert root is the first mutation of the
	// candidate, exactly like the CMIO response.
	m.state.checkpointHash = checkpointHash
	status := machine.CompletionStatusAccepted
	switch {
	case bytes.HasPrefix(input, []byte("reject:")):
		status = machine.CompletionStatusRejected
	case bytes.HasPrefix(input, []byte("exception:")):
		status = machine.CompletionStatusException
	case bytes.HasPrefix(input, []byte("halt:")):
		status = machine.CompletionStatusHalted
	case bytes.HasPrefix(input, []byte("overflow:")):
		status = machine.CompletionStatusOverflow
	case bytes.HasPrefix(input, []byte("unexpected-yield:")):
		status = machine.CompletionStatusUnexpectedYield
	}
	output := append([]byte("output:"), input...)
	report := append([]byte("report:"), input...)
	firstHash := determinismHash("trace-before", previous.machineHash[:], input)
	finalHash := determinismHash("trace-after", firstHash[:], input)
	m.state.step++
	m.state.machineHash = determinismHash(
		"machine", previous.machineHash[:], checkpointHash[:], input,
	)
	m.state.outputsHash = determinismHash("outputs", previous.outputsHash[:], output)
	hashes := []machine.Hash{firstHash, finalHash}
	response := &machine.AdvanceResponse{
		Status:              status,
		PeriodicStateHashes: hashes,
		PaddingRepetitions:  machine.InputEntryCapacity - uint64(len(hashes)),
	}
	if status == machine.CompletionStatusAccepted {
		response.Outputs = []machine.Output{output}
		response.Reports = []machine.Report{report}
	} else if status == machine.CompletionStatusException {
		response.ExceptionData = append([]byte{}, input...)
	}
	behavior := m.behavior
	m.mu.Unlock()

	if behavior == determinismAdvanceWaitForContext {
		m.factory.mutated <- m
	}

	switch behavior {
	case determinismAdvanceSuccess:
		if status == machine.CompletionStatusAccepted {
			m.factory.recordAccepted(m)
		}
		return response, nil
	case determinismAdvanceWaitForContext:
		<-ctx.Done()
		return nil, ctx.Err()
	case determinismAdvanceInfrastructureFailure:
		return nil, machine.ErrMachineInternal
	default:
		return nil, errors.New("unknown determinism advance behavior")
	}
}

func (m *determinismRuntime) Inspect(
	ctx context.Context,
	_ []byte,
) (*machine.InspectResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkOpenLocked(ctx); err != nil {
		return &machine.InspectResponse{}, err
	}
	return &machine.InspectResponse{Status: machine.CompletionStatusRejected}, nil
}

func (m *determinismRuntime) Store(ctx context.Context, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checkOpenLocked(ctx)
}

func (m *determinismRuntime) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *determinismRuntime) Address() string { return "determinism-runtime" }

func (m *determinismRuntime) checkOpenLocked(ctx context.Context) error {
	if m.closed {
		return errDeterminismRuntimeClosed
	}
	return ctx.Err()
}

// snapshot is test-only diagnostics; unlike the machine interface it remains
// readable after Close so tests can prove which state was closed or discarded.
func (m *determinismRuntime) snapshot() determinismMachineState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.clone()
}

func (m *determinismRuntime) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func determinismHash(label string, values ...[]byte) machine.Hash {
	h := sha256.New()
	_, _ = h.Write([]byte(label))
	for _, value := range values {
		_, _ = h.Write(value)
	}
	var result machine.Hash
	copy(result[:], h.Sum(nil))
	return result
}

func determinismValidityLeaf(label string, dataBlock machine.Hash) machine.LeafProof {
	const canonicalMachineProofDepth = 59
	siblings := make([]machine.Hash, canonicalMachineProofDepth)
	for i := range siblings {
		siblings[i] = determinismHash(label, dataBlock[:], []byte{byte(i)})
	}
	return machine.LeafProof{DataBlock: dataBlock, Siblings: siblings}
}

type determinismMachineProvider struct {
	app      *model.Application
	instance manager.MachineInstance
	mu       sync.Mutex
	failures map[int64]string
}

func (p *determinismMachineProvider) GetMachine(appID int64) (manager.MachineInstance, bool) {
	if appID != p.app.ID {
		return nil, false
	}
	return p.instance, true
}

func (p *determinismMachineProvider) Applications() []*model.Application {
	return []*model.Application{p.app}
}

func (p *determinismMachineProvider) UpdateMachines(context.Context) error { return nil }

func (p *determinismMachineProvider) FenceApplicationFailure(app *model.Application, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failures == nil {
		p.failures = map[int64]string{}
	}
	p.failures[app.ID] = reason
}

func (p *determinismMachineProvider) HasPendingApplicationFailures() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.failures) != 0
}

func (p *determinismMachineProvider) failureReason(appID int64) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failures[appID]
}

func (p *determinismMachineProvider) HasMachine(appID int64) bool { return appID == p.app.ID }

func (p *determinismMachineProvider) Close() error { return p.instance.Close() }

// determinismContext makes cancellation and deadline propagation controllable
// without relying on scheduler timing or short wall-clock deadlines.
type determinismContext struct {
	parent context.Context
	done   chan struct{}
	once   sync.Once
	mu     sync.RWMutex
	err    error
}

func newDeterminismContext(parent context.Context) *determinismContext {
	return &determinismContext{parent: parent, done: make(chan struct{})}
}

func (c *determinismContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *determinismContext) Done() <-chan struct{}       { return c.done }
func (c *determinismContext) Err() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.err
}
func (c *determinismContext) Value(key any) any { return c.parent.Value(key) }
func (c *determinismContext) finish(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.done)
	})
}

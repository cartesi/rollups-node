// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type automaticYieldTestEvent struct {
	cycle  uint64
	reason uint16
	data   []byte
}

func automaticYieldTestMachine(
	t *testing.T, request requestType, events []automaticYieldTestEvent, finish uint64, hashes bool, completion ...CompletionStatus,
) *machineImpl {
	t.Helper()
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("ReadMCycle", mock.Anything).Return(uint64(0), nil).Once()
	backend.SetupRequestYield()
	backend.On("SendCmioResponse", uint16(request), []byte("request"), mock.Anything, mock.Anything).Return(nil).Once()
	status := CompletionStatusAccepted
	if len(completion) != 0 {
		status = completion[0]
	}
	breakReason := YieldedManually
	manualReason := ManualYieldReasonAccepted
	var completionData []byte
	completionLength := uint32(HashSize)
	switch status {
	case CompletionStatusAccepted:
	case CompletionStatusRejected:
		completionLength = 0
		manualReason = ManualYieldReasonRejected
	case CompletionStatusException:
		const exception = "exception"
		manualReason = ManualYieldReasonException
		completionData = []byte(exception)
		completionLength = uint32(len(exception))
	case CompletionStatusHalted:
		breakReason = Halted
	case CompletionStatusUnknown, CompletionStatusOverflow, CompletionStatusUnexpectedYield, CompletionStatusInvalidOutputsRoot:
		t.Fatalf("unsupported automatic-yield test completion: %d", status)
	}
	var previous *HashCollectorState
	checkHandoff := func(state *HashCollectorState) {
		if previous != nil {
			require.Equal(t, previous, state, "collector state must match the last backend return before reconstruction")
		}
	}
	saveReturn := func(state *HashCollectorState) {
		copyState := *state
		copyState.Hashes = append([]Hash(nil), state.Hashes...)
		copyState.PartialBundle = append([]byte(nil), state.PartialBundle...)
		previous = &copyState
	}
	for _, event := range events {
		if hashes {
			backend.On("RunAndCollectRootHashes", mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					require.LessOrEqual(t, event.cycle, args.Get(0).(uint64))
					state := args.Get(1).(*HashCollectorState)
					checkHandoff(state)
					collectTestEntries(state, event.cycle, false, Hash{1}, Hash{2})
					if event.cycle == 65*MCycleComputationHashPeriod+1 {
						require.Len(t, state.Hashes, 1)
						require.NotEmpty(t, state.PartialBundle)
						require.Equal(t, uint64(1), state.MCyclePhase)
					}
					saveReturn(state)
				}).Return(YieldedAutomatically, nil).Once()
		} else {
			backend.On("Run", mock.Anything, mock.Anything).Return(YieldedAutomatically, nil).Once()
		}
		backend.On("ReadMCycle", mock.Anything).Return(event.cycle, nil).Once()
		backend.SetupAutomaticYield(event.reason, event.data)
	}
	if hashes {
		backend.On("RunAndCollectRootHashes", mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				require.LessOrEqual(t, finish, args.Get(0).(uint64))
				state := args.Get(1).(*HashCollectorState)
				checkHandoff(state)
				collectTestEntries(state, finish, true, Hash{1}, Hash{2})
				saveReturn(state)
			}).Return(breakReason, nil).Once()
	} else {
		backend.On("Run", mock.Anything, mock.Anything).Return(breakReason, nil).Once()
	}
	backend.On("ReadMCycle", mock.Anything).Return(finish, nil).Once()
	if breakReason == YieldedManually {
		if request == AdvanceStateRequest {
			backend.SetupManualYield(manualReason, completionLength)
		}
		if request == InspectStateRequest || manualReason != ManualYieldReasonAccepted {
			backend.On("ReceiveCmioRequest", mock.Anything).
				Return(uint8(0), uint16(manualReason), completionData, nil).Once()
		}
	}
	t.Cleanup(func() { backend.AssertExpectations(t) })
	config := DefaultConfig("")
	config.ExecutionParameters.AdvanceIncCycles = max(4*MCycleComputationHashPeriod, finish+1)
	config.ExecutionParameters.InspectIncCycles = max(4*MCycleComputationHashPeriod, finish+1)
	config.ExecutionParameters.AdvanceMaxDeadline = time.Second
	config.ExecutionParameters.InspectMaxDeadline = time.Second
	return &machineImpl{backend: backend, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		params: config.ExecutionParameters}
}

func TestAdvanceIgnoresUnknownAutomaticYields(t *testing.T) {
	const finish = 3*MCycleComputationHashPeriod + 9
	events := []automaticYieldTestEvent{
		{MCycleComputationHashPeriod - 1, 8, []byte("discard first")},
		{MCycleComputationHashPeriod, 8, []byte("discard boundary")},
		{MCycleComputationHashPeriod + 1, 0x7f, []byte("discard second")},
		{2*MCycleComputationHashPeriod + 1, 8, []byte("discard repeated")},
		{2*MCycleComputationHashPeriod + 2, 0, []byte("discard zero")},
		{2*MCycleComputationHashPeriod + 3, 0xffff, []byte("discard largest reason")},
		{2*MCycleComputationHashPeriod + 4, uint16(AutomaticYieldReasonProgress), []byte{0xff, 0xff, 0xff, 0xff}},
	}
	for _, hashes := range []bool{false, true} {
		name := "without hashes"
		if hashes {
			name = "with hashes"
		}
		t.Run(name, func(t *testing.T) {
			baseline := automaticYieldTestMachine(t, AdvanceStateRequest, nil, finish, hashes)
			want, err := baseline.Advance(t.Context(), []byte("request"), Hash{}, hashes)
			require.NoError(t, err)
			machine := automaticYieldTestMachine(t, AdvanceStateRequest, events, finish, hashes)
			got, err := machine.Advance(t.Context(), []byte("request"), Hash{}, hashes)
			require.NoError(t, err)
			require.Equal(t, CompletionStatusAccepted, got.Status)
			require.Empty(t, got.Outputs)
			require.Empty(t, got.Reports)
			require.Empty(t, want.IgnoredAutomaticYields)
			want.IgnoredAutomaticYields = []IgnoredAutomaticYield{
				{Reason: 8, Count: 3}, {Reason: 0x7f, Count: 1},
				{Reason: 0, Count: 1}, {Reason: 0xffff, Count: 1},
			}
			require.Equal(t, want, got)
			if hashes {
				require.NotEmpty(t, got.PeriodicStateHashes)
			}
		})
	}
}

func TestInspectContinuesReportsAcrossUnknownAutomaticYield(t *testing.T) {
	events := []automaticYieldTestEvent{
		{1, uint16(AutomaticYieldReasonReport), []byte("before")},
		{2, 0x7f, []byte("discard")},
		{3, uint16(AutomaticYieldReasonReport), []byte("after")},
	}
	machine := automaticYieldTestMachine(t, InspectStateRequest, events, 4, false)
	got, err := machine.Inspect(t.Context(), []byte("request"))
	require.NoError(t, err)
	require.Equal(t, CompletionStatusAccepted, got.Status)
	require.Equal(t, []Report{[]byte("before"), []byte("after")}, got.Reports)
	require.Equal(t, []IgnoredAutomaticYield{{Reason: 0x7f, Count: 1}}, got.IgnoredAutomaticYields)
}

func TestIgnoredAutomaticYieldDiagnosticsOnIncompleteRequests(t *testing.T) {
	for _, request := range []requestType{AdvanceStateRequest, InspectStateRequest} {
		t.Run(request.String(), func(t *testing.T) {
			backend := NewMockBackend()
			backendError := errors.New("backend stopped")
			backend.On("CmioRxBufferSize").Return(uint64(1024))
			backend.On("ReadMCycle", mock.Anything).Return(uint64(0), nil).Once()
			backend.SetupRequestYield()
			backend.On("SendCmioResponse", uint16(request), []byte("request"), mock.Anything, mock.Anything).Return(nil).Once()
			for _, event := range []automaticYieldTestEvent{
				{1, uint16(AutomaticYieldReasonReport), []byte("partial")},
				{2, 0x7f, []byte("discard")},
			} {
				backend.On("Run", mock.Anything, mock.Anything).Return(YieldedAutomatically, nil).Once()
				backend.On("ReadMCycle", mock.Anything).Return(event.cycle, nil).Once()
				backend.SetupAutomaticYield(event.reason, event.data)
			}
			backend.On("Run", mock.Anything, mock.Anything).Return(Failed, backendError).Once()
			config := DefaultConfig("")
			machine := &machineImpl{backend: backend, params: config.ExecutionParameters,
				logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			if request == AdvanceStateRequest {
				got, err := machine.Advance(t.Context(), []byte("request"), Hash{}, false)
				require.ErrorIs(t, err, backendError)
				require.Nil(t, got)
			} else {
				got, err := machine.Inspect(t.Context(), []byte("request"))
				require.ErrorIs(t, err, backendError)
				require.Equal(t, CompletionStatusUnknown, got.Status)
				require.Equal(t, []Report{[]byte("partial")}, got.Reports)
				require.Equal(t, []IgnoredAutomaticYield{{Reason: 0x7f, Count: 1}}, got.IgnoredAutomaticYields)
			}
			backend.AssertExpectations(t)
		})
	}
}

// unknownYieldAdvanceBackend inserts an unknown request between the output
// and report of statefulAdvanceBackend without changing the guest's endpoint.
type unknownYieldAdvanceBackend struct {
	*statefulAdvanceBackend
}

func (b *unknownYieldAdvanceBackend) Run(end uint64, timeout time.Duration) (BreakReason, error) {
	if b.cycle == 3 && end >= 4 {
		b.runTargets = append(b.runTargets, end)
		b.cycle = 4
		return YieldedAutomatically, nil
	}
	return b.statefulAdvanceBackend.Run(end, timeout)
}

func (b *unknownYieldAdvanceBackend) RunAndCollectRootHashes(
	end uint64, state *HashCollectorState, timeout time.Duration,
) (BreakReason, error) {
	reason, err := b.Run(end, timeout)
	if err == nil {
		collectTestEntries(state, b.cycle, isFixedPointBreakReason(reason), b.hashes[0], Hash{0xF1, 0xED})
	}
	return reason, err
}

func (b *unknownYieldAdvanceBackend) ReadMemory(address, length uint64, timeout time.Duration) ([]byte, error) {
	if b.cycle == 4 {
		if address != htifTohostAddress || length != 8 {
			return nil, fmt.Errorf("unknown payload must not be read at %d with length %d", address, length)
		}
		return binary.LittleEndian.AppendUint64(nil, htifDeviceYield<<htifDeviceShift|uint64(0x7f)<<htifReasonShift|7), nil
	}
	return b.statefulAdvanceBackend.ReadMemory(address, length, timeout)
}

func TestAdvancePreservesOutputAcrossUnknownAutomaticYield(t *testing.T) {
	for _, increment := range []uint64{1, 2, 3, 32} {
		for _, hashes := range []bool{false, true} {
			t.Run(fmt.Sprintf("increment=%d/hashes=%t", increment, hashes), func(t *testing.T) {
				config := DefaultConfig("")
				config.ExecutionParameters.AdvanceIncCycles = increment
				config.ExecutionParameters.AdvanceMaxDeadline = time.Second
				logger := slog.New(slog.NewTextHandler(io.Discard, nil))
				newBackend := func() *statefulAdvanceBackend {
					return &statefulAdvanceBackend{outputsHash: Hash{1}, hashes: []Hash{{2}}}
				}
				baselineBackend := newBackend()
				baseline := &machineImpl{backend: baselineBackend, logger: logger, params: config.ExecutionParameters}
				want, err := baseline.Advance(t.Context(), []byte("stable-input"), Hash{}, hashes)
				require.NoError(t, err)

				backend := &unknownYieldAdvanceBackend{statefulAdvanceBackend: newBackend()}
				machine := &machineImpl{backend: backend, logger: logger, params: config.ExecutionParameters}
				got, err := machine.Advance(t.Context(), []byte("stable-input"), Hash{}, hashes)
				require.NoError(t, err)
				require.Equal(t, CompletionStatusAccepted, got.Status)
				require.Equal(t, []Output{[]byte("stable-output")}, got.Outputs)
				require.Equal(t, []Report{[]byte("stable-report")}, got.Reports)
				require.Equal(t, uint64(9), backend.cycle)
				require.Equal(t, baselineBackend.cycle, backend.cycle)
				require.Empty(t, want.IgnoredAutomaticYields)
				want.IgnoredAutomaticYields = []IgnoredAutomaticYield{{Reason: 0x7f, Count: 1}}
				require.Equal(t, want, got)
			})
		}
	}
}

func TestAdvancePreservesCollectorHandoffAcrossUnknownYield(t *testing.T) {
	const finish = 66*MCycleComputationHashPeriod + 3
	baseline := automaticYieldTestMachine(t, AdvanceStateRequest, nil, finish, true)
	want, err := baseline.Advance(t.Context(), []byte("request"), Hash{}, true)
	require.NoError(t, err)
	machine := automaticYieldTestMachine(t, AdvanceStateRequest,
		[]automaticYieldTestEvent{{65*MCycleComputationHashPeriod + 1, 0x7f, []byte("discard")}}, finish, true)
	machine.params.AdvanceIncCycles = 67 * MCycleComputationHashPeriod
	got, err := machine.Advance(t.Context(), []byte("request"), Hash{}, true)
	require.NoError(t, err)
	require.Len(t, got.PeriodicStateHashes, 2)
	want.IgnoredAutomaticYields = []IgnoredAutomaticYield{{Reason: 0x7f, Count: 1}}
	require.Equal(t, want, got)
}

func TestAdvanceRetainsIgnoredYieldsOnCompletedOutcomes(t *testing.T) {
	for _, status := range []CompletionStatus{CompletionStatusRejected, CompletionStatusException, CompletionStatusHalted} {
		t.Run(fmt.Sprintf("status=%d", status), func(t *testing.T) {
			machine := automaticYieldTestMachine(t, AdvanceStateRequest,
				[]automaticYieldTestEvent{{1, 0x7f, []byte("discard")}}, 2, false, status)
			result, err := machine.Advance(t.Context(), []byte("request"), Hash{}, false)
			require.NoError(t, err)
			require.Equal(t, status, result.Status)
			require.Equal(t, []IgnoredAutomaticYield{{Reason: 0x7f, Count: 1}}, result.IgnoredAutomaticYields)
		})
	}
}

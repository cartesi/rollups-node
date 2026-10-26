// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCollectorProgressChecksBothGeometries(t *testing.T) {
	for _, geometry := range []struct {
		period uint64
		height uint8
	}{{24, 0}, {18, 6}} {
		period := uint64(1) << geometry.period
		for _, delta := range []uint64{0, 1, period, period + 1, 63 * period, 64 * period, 65*period + 1} {
			state := &HashCollectorState{MCycleSamplingPeriod: period, Log2BundleMCycleCount: int32(geometry.height)}
			collectTestEntries(state, delta, false, Hash{1}, Hash{2})
			require.NoError(t, validateHashCollectorProgress(state, delta, false))
			state.Hashes = append(state.Hashes, Hash{3})
			require.ErrorContains(t, validateHashCollectorProgress(state, delta, false), "entries, expected")
		}
	}
	state := &HashCollectorState{MCycleSamplingPeriod: 1 << 18, Log2BundleMCycleCount: 6}
	for _, test := range []struct {
		data, want string
	}{
		{`null`, "present=false"}, {`{}`, "no leaf_count"},
		{`{"leaf_count":0}`, "has 0 samples, expected 1"}, {`{"leaf_count":2}`, "has 2 samples, expected 1"},
		{`{"leaf_count":null}`, "no leaf_count"}, {`{"leaf_count":-1}`, "invalid partial bundle"},
	} {
		state.PartialBundle = json.RawMessage(test.data)
		require.ErrorContains(t, validateHashCollectorProgress(state, 1<<18, false), test.want)
	}
	state.PartialBundle = json.RawMessage(` null `)
	require.NoError(t, validateHashCollectorProgress(state, 0, false))
	require.NoError(t, validateHashCollectorProgress(state, 0, true))
	state.PartialBundle = json.RawMessage(`{"leaf_count":1}`)
	require.ErrorContains(t, validateHashCollectorProgress(state, 1<<18, true), "partial bundle remains")
}

type progressRunBackend struct {
	Backend
	cycle, finish, automatic uint64
	calls                    int
	state                    *HashCollectorState
	dropBeforeCall           bool
	tamper                   func(*HashCollectorState, bool)
}

func (b *progressRunBackend) ReadMCycle(time.Duration) (uint64, error) { return b.cycle, nil }

func (b *progressRunBackend) RunAndCollectRootHashes(end uint64, state *HashCollectorState, _ time.Duration) (BreakReason, error) {
	b.calls++
	b.state = state
	reason := ReachedTargetMcycle
	b.cycle = min(end, b.finish)
	if b.automatic != 0 && b.automatic <= b.cycle {
		b.cycle = b.automatic
		b.automatic = 0
		reason = YieldedAutomatically
	} else if b.cycle == b.finish {
		reason = McycleOverflow
	}
	collectTestEntries(state, b.cycle, isFixedPointBreakReason(reason), Hash{1}, Hash{2})
	if b.tamper != nil {
		b.tamper(state, isFixedPointBreakReason(reason))
	}
	return reason, nil
}

func (b *progressRunBackend) ReceiveCmioRequest(time.Duration) (uint8, uint16, []byte, error) {
	if b.dropBeforeCall {
		b.state.PartialBundle = nil
	}
	return 0, uint16(AutomaticYieldReasonProgress), nil, nil
}

func newProgressRunMachine(backend Backend) *machineImpl {
	return &machineImpl{backend: backend, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		params: model.ExecutionParameters{AdvanceIncCycles: MCycleComputationHashPeriod/3 + 12345,
			AdvanceIncDeadline: time.Second, AdvanceMaxDeadline: time.Second}}
}

func TestCollectorCarriesAcrossIncrementsAndAutomaticYield(t *testing.T) {
	finish := 3*MCycleComputationHashPeriod + 1
	backend := &progressRunBackend{finish: finish, automatic: MCycleComputationHashPeriod + 1}
	machine := newProgressRunMachine(backend)
	result, err := machine.run(t.Context(), AdvanceStateRequest, true, testExecutionBounds(0, finish))
	require.ErrorIs(t, err, ErrMcycleOverflow)
	require.Greater(t, backend.calls, 3)
	kept, padding, final, err := model.CanonicalInputHashCollection(backend.state.Hashes, finish)
	require.NoError(t, err)
	require.Equal(t, kept, result.periodicStateHashes)
	require.Equal(t, padding, result.paddingRepetitions)
	require.Equal(t, final, result.finalStateEntry)
}

func TestCollectorIntegrityErrorsStayInternal(t *testing.T) {
	for _, test := range []struct {
		name   string
		finish uint64
		tamper func(*HashCollectorState, bool)
	}{
		{name: "extra entry", finish: 3 * MCycleComputationHashPeriod,
			tamper: func(state *HashCollectorState, _ bool) { state.Hashes = append(state.Hashes, Hash{3}) }},
		{name: "wrong partial count", finish: 3 * MCycleComputationHashPeriod,
			tamper: func(state *HashCollectorState, fixed bool) {
				if !fixed {
					state.PartialBundle = json.RawMessage(`{"leaf_count":63}`)
				}
			}},
		{name: "terminal partial", finish: 1,
			tamper: func(state *HashCollectorState, _ bool) { state.PartialBundle = json.RawMessage(`{"leaf_count":1}`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := &progressRunBackend{finish: test.finish, tamper: test.tamper}
			machine := newProgressRunMachine(backend)
			if test.name == "wrong partial count" {
				machine.params.AdvanceIncCycles = MCycleComputationHashPeriod + 1
			}
			_, err := machine.run(t.Context(), AdvanceStateRequest, true, testExecutionBounds(0, test.finish))
			require.ErrorIs(t, err, ErrMachineInternal)
			require.NotErrorIs(t, err, ErrMcycleOverflow)
			require.Contains(t, err.Error(), "deterministic")
			require.Contains(t, err.Error(), "emulator")
		})
	}
}

func TestCollectorDetectsPartialLossBeforeNextCall(t *testing.T) {
	if Log2StateHashesPerEntry == 0 {
		t.Skip("partial carry requires bundled entries")
	}
	finish := 3 * MCycleComputationHashPeriod
	backend := &progressRunBackend{finish: finish, automatic: MCycleComputationHashPeriod + 1, dropBeforeCall: true}
	machine := newProgressRunMachine(backend)
	_, err := machine.run(context.Background(), AdvanceStateRequest, true, testExecutionBounds(0, finish))
	require.ErrorIs(t, err, ErrMachineInternal)
	require.Contains(t, err.Error(), "present=false")
}

func TestFailedCollectorKeepsItsOwnError(t *testing.T) {
	backend := NewMockBackend()
	backend.On("RunAndCollectRootHashes", uint64(1), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			state := args.Get(1).(*HashCollectorState)
			state.Hashes = []Hash{{1}}
			state.PartialBundle = json.RawMessage(`{"leaf_count":1}`)
		}).Return(Failed, nil).Once()
	backend.On("ReadMCycle", mock.Anything).Return(uint64(1), nil).Once()
	machine := newProgressRunMachine(backend)
	_, err := machine.run(t.Context(), AdvanceStateRequest, true, testExecutionBounds(0, 1))
	require.ErrorIs(t, err, ErrMachineInternal)
	require.NotContains(t, err.Error(), "invalid collected state hash span")
	require.NotErrorIs(t, err, ErrMcycleOverflow)
	backend.AssertExpectations(t)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func TestImplementation(t *testing.T) {
	suite.Run(t, new(ImplementationSuite))
}

type ImplementationSuite struct {
	suite.Suite
	logger *slog.Logger
}

func testExecutionBounds(start, limit uint64) executionBounds {
	return executionBounds{
		start: start, limit: limit, span: limit - start, configured: true,
	}
}

func (s *ImplementationSuite) SetupSuite() {
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Test Fork method
func (s *ImplementationSuite) TestFork() {
	require := s.Require()
	ctx := context.Background()

	// Test successful fork
	mockBackend := NewMockBackend()
	forkedBackend := NewMockBackend()
	mockBackend.On("ForkServer", mock.AnythingOfType("time.Duration")).Return(
		forkedBackend, "127.0.0.1:54321", uint32(54321), nil)

	machine := &machineImpl{
		backend: mockBackend,
		address: "127.0.0.1:12345",
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}

	forkedMachine, err := machine.Fork(ctx)
	require.NoError(err)
	require.NotNil(forkedMachine)
	require.Equal("127.0.0.1:54321", forkedMachine.Address())
	mockBackend.AssertExpectations(s.T())

	// Test fork with backend error
	mockBackend2 := NewMockBackend()
	mockBackend2.On("ForkServer", mock.AnythingOfType("time.Duration")).Return(
		(*MockBackend)(nil), "", uint32(0), errors.New("fork failed"))

	machine2 := &machineImpl{
		backend: mockBackend2,
		address: "127.0.0.1:12345",
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}

	_, err = machine2.Fork(ctx)
	require.Error(err)
	require.ErrorIs(err, ErrMachineInternal)
	require.Contains(err.Error(), "could not fork the machine")

	// Test fork with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = machine.Fork(canceledCtx)
	require.ErrorIs(err, ErrCanceled)
	mockBackend2.AssertExpectations(s.T())
}

// Test Hash method
func (s *ImplementationSuite) TestHash() {
	require := s.Require()
	ctx := context.Background()

	// Test successful hash retrieval
	mockBackend := NewMockBackend()
	expectedHash := randomFakeHash()
	mockBackend.On("GetRootHash", mock.AnythingOfType("time.Duration")).Return(expectedHash, nil)

	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			LoadDeadline: time.Second * 5,
		},
	}

	hash, err := machine.Hash(ctx)
	require.NoError(err)
	require.Equal(expectedHash, hash)
	mockBackend.AssertExpectations(s.T())

	// Test hash with backend error
	mockBackend2 := NewMockBackend()
	mockBackend2.On("GetRootHash", mock.AnythingOfType("time.Duration")).Return((Hash)(Hash{}), errors.New("hash failed"))
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			LoadDeadline: time.Second * 5,
		},
	}
	_, err = machine2.Hash(ctx)
	require.Error(err)
	require.ErrorIs(err, ErrMachineInternal)
	require.Contains(err.Error(), "could not get the machine's root hash")
	mockBackend2.AssertExpectations(s.T())

	// Test hash with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = machine.Hash(canceledCtx)
	require.ErrorIs(err, ErrCanceled)
}

// Test OutputsHash method
func (s *ImplementationSuite) TestOutputsHash() {
	require := s.Require()
	ctx := context.Background()

	// Test successful outputs hash retrieval
	mockBackend := NewMockBackend()
	expectedHash := randomFakeHash()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), expectedHash[:], nil)

	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}

	hash, err := machine.OutputsHash(ctx)
	require.NoError(err)
	require.Equal(expectedHash, hash)
	mockBackend.AssertExpectations(s.T())

	// Test outputs hash with rejected request
	mockBackend2 := NewMockBackend()
	mockBackend2.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonRejected), make([]byte, 32), nil)
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	_, err = machine2.OutputsHash(ctx)
	require.ErrorIs(err, ErrRejected)
	mockBackend2.AssertExpectations(s.T())

	// Exception remains distinguishable through the public error taxonomy.
	mockBackendException := NewMockBackend()
	mockBackendException.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonException), []byte("exception"), nil)
	machineException := &machineImpl{
		backend: mockBackendException,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	_, err = machineException.OutputsHash(ctx)
	require.ErrorIs(err, ErrException)
	mockBackendException.AssertExpectations(s.T())

	// Test outputs hash with invalid length
	mockBackend3 := NewMockBackend()
	mockBackend3.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), make([]byte, 16), nil) // Invalid length
	machine3 := &machineImpl{
		backend: mockBackend3,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	_, err = machine3.OutputsHash(ctx)
	require.Error(err)
	require.ErrorIs(err, ErrHashLength)
	mockBackend3.AssertExpectations(s.T())

	// Test outputs hash with backend error
	mockBackend4 := NewMockBackend()
	mockBackend4.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(0), []byte{}, errors.New("receive failed"))
	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	_, err = machine4.OutputsHash(ctx)
	require.Error(err)
	require.Contains(err.Error(), "could not read the outputs hash")
	mockBackend4.AssertExpectations(s.T())
}

// Test OutputsHashProof method
func (s *ImplementationSuite) TestOutputsHashProof() {
	require := s.Require()
	ctx := context.Background()

	// Test successful outputs hash proof retrieval
	mockBackend := NewMockBackend()
	expectedProof := []Hash{randomFakeHash(), randomFakeHash(), randomFakeHash()}
	mockBackend.On("GetProof", TxBufferAddress, int32(HashLog2Size), mock.AnythingOfType("time.Duration")).
		Return(expectedProof, nil)

	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			LoadDeadline: time.Second * 5,
		},
	}

	proof, err := machine.OutputsHashProof(ctx)
	require.NoError(err)
	require.Equal(expectedProof, proof)
	mockBackend.AssertExpectations(s.T())

	// Test outputs hash proof with backend error
	mockBackend2 := NewMockBackend()
	mockBackend2.On("GetProof", TxBufferAddress, int32(HashLog2Size), mock.AnythingOfType("time.Duration")).
		Return([]Hash(nil), errors.New("proof failed"))
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			LoadDeadline: time.Second * 5,
		},
	}
	_, err = machine2.OutputsHashProof(ctx)
	require.Error(err)
	require.ErrorIs(err, ErrMachineInternal)
	require.Contains(err.Error(), "could not get outputs hash machine proof")
	mockBackend2.AssertExpectations(s.T())

	// Test outputs hash proof with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = machine.OutputsHashProof(canceledCtx)
	require.ErrorIs(err, ErrCanceled)
}

// Test Advance method
func (s *ImplementationSuite) TestAdvance() {
	require := s.Require()
	ctx := context.Background()
	expectedHash := randomFakeHash()

	// Test successful advance (accepted)
	mockBackend := NewMockBackend()
	mockBackend.SetupAccepted(AdvanceStateRequest)

	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	input := []byte("test input")
	resp, err := machine.Advance(ctx, input, expectedHash, false)
	require.NoError(err)
	require.NotNil(resp)
	require.Equal(CompletionStatusAccepted, resp.Status)
	require.Empty(resp.Outputs)
	require.Empty(resp.Reports)
	require.NotEqual(Hash{}, resp.OutputsHash)
	mockBackend.AssertExpectations(s.T())

	// Test advance with rejection
	mockBackend2 := NewMockBackend()
	mockBackend2.SetupRejected(AdvanceStateRequest)
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machine2.Advance(ctx, input, expectedHash, false)
	require.NoError(err)
	require.NotNil(resp)
	require.Equal(CompletionStatusRejected, resp.Status)
	require.Empty(resp.Outputs)
	require.Empty(resp.Reports)
	require.Equal(Hash{}, resp.OutputsHash)
	mockBackend2.AssertExpectations(s.T())

	// Test advance with exception
	mockBackend3 := NewMockBackend()
	mockBackend3.SetupException(AdvanceStateRequest)
	machine3 := &machineImpl{
		backend: mockBackend3,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machine3.Advance(ctx, input, expectedHash, false)
	require.NoError(err)
	require.NotNil(resp)
	require.Equal(CompletionStatusException, resp.Status)
	require.Equal([]byte("exception data"), resp.ExceptionData)
	require.Equal(Hash{}, resp.OutputsHash)
	mockBackend3.AssertExpectations(s.T())

	// Halting is also a completed deterministic status and retains
	// the PRT input hash collection produced before the halt.
	expectedHaltHash := randomFakeHash()
	mockBackendHalted := NewMockBackend()
	mockBackendHalted.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackendHalted.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil).Once()
	mockBackendHalted.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(100), nil).Once()
	mockBackendHalted.On(
		"SendCmioResponse",
		uint16(AdvanceStateRequest), mock.Anything, mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	mockBackendHalted.On(
		"RunAndCollectRootHashes",
		mock.AnythingOfType("uint64"),
		mock.AnythingOfType("*machine.HashCollectorState"),
		mock.AnythingOfType("time.Duration"),
	).Run(func(args mock.Arguments) {
		state := args.Get(1).(*HashCollectorState)
		state.Hashes = append(state.Hashes, expectedHaltHash)
		state.MCyclePhase = 0 // halt occurred exactly at the collected boundary
	}).Return(Halted, nil)
	machineHalted := &machineImpl{
		backend: mockBackendHalted,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machineHalted.Advance(ctx, input, expectedHash, true)
	require.NoError(err)
	require.NotNil(resp)
	require.Equal(CompletionStatusHalted, resp.Status)
	require.Empty(resp.PeriodicStateHashes)
	require.Equal(InputEntryCapacity, resp.PaddingRepetitions)
	mockBackendHalted.AssertExpectations(s.T())

	// Test advance halted off a sampling boundary: 0.21 still appends the
	// fixed-point sample (nonzero phase) and the node must still drop it.
	offBoundaryHaltHash := randomFakeHash()
	mockBackendHaltedOff := NewMockBackend()
	mockBackendHaltedOff.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackendHaltedOff.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil).Once()
	mockBackendHaltedOff.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(101), nil).Once()
	mockBackendHaltedOff.On(
		"SendCmioResponse",
		uint16(AdvanceStateRequest), mock.Anything, mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	mockBackendHaltedOff.On(
		"RunAndCollectRootHashes",
		mock.AnythingOfType("uint64"),
		mock.AnythingOfType("*machine.HashCollectorState"),
		mock.AnythingOfType("time.Duration"),
	).Run(func(args mock.Arguments) {
		state := args.Get(1).(*HashCollectorState)
		state.Hashes = append(state.Hashes, offBoundaryHaltHash)
		state.MCyclePhase = 5 // halt occurred between sampling boundaries
	}).Return(Halted, nil)
	machineHaltedOff := &machineImpl{
		backend: mockBackendHaltedOff,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machineHaltedOff.Advance(ctx, input, expectedHash, true)
	require.NoError(err)
	require.NotNil(resp)
	require.Equal(CompletionStatusHalted, resp.Status)
	require.Empty(resp.PeriodicStateHashes)
	require.Equal(InputEntryCapacity, resp.PaddingRepetitions)
	mockBackendHaltedOff.AssertExpectations(s.T())

	// Test advance with payload too large
	mockBackend4 := NewMockBackend()
	mockBackend4.On("CmioRxBufferSize").Return(uint64(5))
	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	largeInput := make([]byte, 10)
	resp, err = machine4.Advance(ctx, largeInput, expectedHash, false)
	require.ErrorIs(err, ErrPayloadLengthLimitExceeded)
	require.Nil(resp)
	mockBackend4.AssertExpectations(s.T())

	// A configured target beyond uint64 saturates at MaxUint64, just like the
	// emulator's imcyclemax. If the machine reaches that endpoint, the machine
	// overflow—not the configured-cap sentinel—explains why execution stopped.
	mockBackendOverflow := NewMockBackend()
	mockBackendOverflow.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackendOverflow.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(^uint64(0)-uint64(999), nil).Once()
	mockBackendOverflow.On("SendCmioResponse", uint16(AdvanceStateRequest), input, expectedHash,
		mock.AnythingOfType("time.Duration")).Return(nil).Once()
	mockBackendOverflow.On("Run", ^uint64(0), mock.AnythingOfType("time.Duration")).
		Return(McycleOverflow, nil).Once()
	mockBackendOverflow.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(^uint64(0), nil).Once()
	machineOverflow := &machineImpl{
		backend: mockBackendOverflow,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   ^uint64(0),
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: time.Second,
			AdvanceMaxCycles:   1000,
		},
	}
	resp, err = machineOverflow.Advance(ctx, input, expectedHash, false)
	require.ErrorIs(err, ErrReachedLimitMcycle)
	require.ErrorIs(err, ErrMcycleOverflow)
	require.Contains(err.Error(), "requested_span=1000")
	require.Contains(err.Error(), "target_span=999")
	require.Contains(err.Error(), "executed_cycles=999")
	require.Nil(resp)
	mockBackendOverflow.AssertExpectations(s.T())

	// Test advance with invalid hash length
	mockBackend5 := NewMockBackend()
	mockBackend5.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackend5.On("SendCmioResponse", uint16(AdvanceStateRequest), mock.Anything, expectedHash, mock.AnythingOfType("time.Duration")).Return(nil)
	mockBackend5.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	mockBackend5.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	mockBackend5.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), make([]byte, 16), nil) // Invalid hash length
	machine5 := &machineImpl{
		backend: mockBackend5,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machine5.Advance(ctx, input, expectedHash, false)
	require.Error(err)
	require.ErrorIs(err, ErrHashLength)
	require.Nil(resp)
	mockBackend5.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestCompletedAdvanceIsIndependentOfAdequateIncrementSettings() {
	expectedOutputsHash := randomFakeHash()
	expectedHashes := []Hash{randomFakeHash(), randomFakeHash()}
	configs := []model.ExecutionParameters{
		{
			FastDeadline:       time.Second,
			AdvanceIncCycles:   2,
			AdvanceIncDeadline: 10 * time.Millisecond,
			AdvanceMaxDeadline: time.Second,
		},
		{
			FastDeadline:       2 * time.Second,
			AdvanceIncCycles:   4,
			AdvanceIncDeadline: 100 * time.Millisecond,
			AdvanceMaxDeadline: 2 * time.Second,
		},
		{
			FastDeadline:       3 * time.Second,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: 3 * time.Second,
		},
	}
	expectedTargets := [][]uint64{
		{2, 4, 5, 7, 8, 10},
		{4, 7, 10},
		{100, 103, 106},
	}

	var baseline *AdvanceResponse
	for i, params := range configs {
		s.Run(fmt.Sprintf("configuration-%d", i), func() {
			backend := &statefulAdvanceBackend{
				outputsHash: expectedOutputsHash,
				hashes:      expectedHashes,
			}
			machine := &machineImpl{backend: backend, logger: s.logger, params: params}

			result, err := machine.Advance(context.Background(), []byte("stable-input"), Hash{}, true)
			s.Require().NoError(err)
			s.Require().NotNil(result)
			s.Require().Equal(expectedTargets[i], backend.runTargets)
			if baseline == nil {
				baseline = result
			} else {
				s.Require().Equal(baseline, result)
			}
		})
	}
}

func (s *ImplementationSuite) TestPRTHashCollectionIsIndependentOfConfiguredCycleCap() {
	const configuredCap uint64 = 10
	expectedOutputsHash := randomFakeHash()
	expectedHashes := []Hash{randomFakeHash(), randomFakeHash()}

	runAdvance := func(maxCycles uint64) (*AdvanceResponse, []uint64) {
		backend := &statefulAdvanceBackend{
			outputsHash: expectedOutputsHash,
			hashes:      expectedHashes,
		}
		machine := &machineImpl{
			backend: backend,
			logger:  s.logger,
			params: model.ExecutionParameters{
				FastDeadline:       time.Second,
				AdvanceIncCycles:   100,
				AdvanceIncDeadline: time.Second,
				AdvanceMaxDeadline: time.Second,
				AdvanceMaxCycles:   maxCycles,
			},
		}

		result, err := machine.Advance(context.Background(), []byte("stable-input"), Hash{}, true)
		s.Require().NoError(err)
		s.Require().NotNil(result)
		return result, backend.runTargets
	}

	uncapped, uncappedTargets := runAdvance(0)
	capped, cappedTargets := runAdvance(configuredCap)
	maximumCap, maximumCapTargets := runAdvance(model.MaxExecutionCycleSpan)

	s.Equal([]uint64{100, 103, 106}, uncappedTargets)
	s.Equal([]uint64{configuredCap, configuredCap, configuredCap}, cappedTargets)
	s.NotEqual(uncappedTargets, cappedTargets, "configured cap must alter the backend run targets")
	s.Equal(uncapped, capped)
	s.Equal(uncapped.PeriodicStateHashes, capped.PeriodicStateHashes)
	s.Equal(uncapped.PaddingRepetitions, capped.PaddingRepetitions)
	s.Equal(expectedHashes, capped.PeriodicStateHashes)
	s.Equal(InputEntryCapacity-uint64(len(expectedHashes)), capped.PaddingRepetitions)
	s.Equal(uncappedTargets, maximumCapTargets)
	s.Equal(uncapped, maximumCap,
		"the largest valid configured span must be canonically identical to no operator cap")
}

// statefulAdvanceBackend models a guest that produces an output at mcycle 3,
// a report at mcycle 6, and its final accepted yield at mcycle 9. Small
// increments reach intermediate targets; large increments stop early at the
// same guest events. This makes the scheduling-invariance test exercise the
// real run loop rather than preprogramming one yield per call.
type statefulAdvanceBackend struct {
	cycle       uint64
	runTargets  []uint64
	outputsHash Hash
	hashes      []Hash
}

func (b *statefulAdvanceBackend) run(mcycleEnd uint64) (BreakReason, error) {
	b.runTargets = append(b.runTargets, mcycleEnd)
	if mcycleEnd <= b.cycle {
		return Failed, fmt.Errorf("run target %d is not ahead of cycle %d", mcycleEnd, b.cycle)
	}
	var eventCycle uint64
	var reason BreakReason
	switch {
	case b.cycle < 3:
		eventCycle, reason = 3, YieldedAutomatically
	case b.cycle < 6:
		eventCycle, reason = 6, YieldedAutomatically
	case b.cycle < 9:
		eventCycle, reason = 9, YieldedManually
	default:
		return Failed, errors.New("scripted advance already completed")
	}
	if mcycleEnd < eventCycle {
		b.cycle = mcycleEnd
		return ReachedTargetMcycle, nil
	}
	b.cycle = eventCycle
	return reason, nil
}

func (b *statefulAdvanceBackend) Load(string, string, time.Duration) error { return nil }
func (b *statefulAdvanceBackend) Store(string, time.Duration) error        { return nil }
func (b *statefulAdvanceBackend) Run(end uint64, _ time.Duration) (BreakReason, error) {
	return b.run(end)
}
func (b *statefulAdvanceBackend) RunAndCollectRootHashes(
	end uint64,
	state *HashCollectorState,
	_ time.Duration,
) (BreakReason, error) {
	reason, err := b.run(end)
	if err == nil && reason == YieldedAutomatically {
		switch b.cycle {
		case 3:
			state.Hashes = append(state.Hashes, b.hashes[0])
		case 6:
			state.Hashes = append(state.Hashes, b.hashes[1])
		}
	}
	if err == nil && reason == YieldedManually {
		// Emulator 0.21 appends the fixed-point sample whenever a collecting
		// run stops at a manual yield, regardless of phase; the node drops
		// exactly this final entry.
		state.Hashes = append(state.Hashes, Hash{0xF1, 0xED})
	}
	return reason, err
}
func (b *statefulAdvanceBackend) IsAtManualYield(time.Duration) (bool, error) {
	return b.cycle == 9, nil
}
func (b *statefulAdvanceBackend) ReadMCycle(time.Duration) (uint64, error) { return b.cycle, nil }
func (b *statefulAdvanceBackend) SendCmioResponse(reason uint16, data []byte, _ *Hash, _ time.Duration) error {
	if reason != uint16(AdvanceStateRequest) || string(data) != "stable-input" {
		return fmt.Errorf("unexpected CMIO response reason=%d data_len=%d", reason, len(data))
	}
	return nil
}
func (b *statefulAdvanceBackend) ReceiveCmioRequest(time.Duration) (uint8, uint16, []byte, error) {
	switch b.cycle {
	case 3:
		return 0, uint16(AutomaticYieldReasonOutput), []byte("stable-output"), nil
	case 6:
		return 0, uint16(AutomaticYieldReasonReport), []byte("stable-report"), nil
	case 9:
		return 0, uint16(ManualYieldReasonAccepted), b.outputsHash[:], nil
	default:
		return 0, 0, nil, fmt.Errorf("no CMIO request at cycle %d", b.cycle)
	}
}
func (b *statefulAdvanceBackend) WriteMemory(uint64, []byte, time.Duration) error { return nil }
func (b *statefulAdvanceBackend) GetRootHash(time.Duration) (Hash, error)         { return Hash{}, nil }
func (b *statefulAdvanceBackend) GetProof(uint64, int32, time.Duration) ([]Hash, error) {
	return nil, nil
}
func (b *statefulAdvanceBackend) Delete() {}
func (b *statefulAdvanceBackend) ForkServer(time.Duration) (Backend, string, uint32, error) {
	return nil, "", 0, errors.New("not implemented")
}
func (b *statefulAdvanceBackend) ShutdownServer(time.Duration) error { return nil }
func (b *statefulAdvanceBackend) NewMachineRuntimeConfig() (string, error) {
	return "{}", nil
}
func (b *statefulAdvanceBackend) CmioRxBufferSize() uint64 { return 1024 }

func (s *ImplementationSuite) TestInterruptedAdvanceReturnsNilAndCanBeRetried() {
	expectedOutputsHash := randomFakeHash()
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	backend.On(
		"SendCmioResponse",
		uint16(AdvanceStateRequest),
		[]byte("retry-input"),
		mock.Anything,
		mock.AnythingOfType("time.Duration"),
	).Return(nil)
	backend.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).
		Return(YieldedManually, nil)
	backend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), expectedOutputsHash[:], nil,
	)
	machine := &machineImpl{
		backend: backend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := machine.Advance(ctx, []byte("retry-input"), Hash{}, false)
	s.Require().ErrorIs(err, ErrCanceled)
	s.Require().Nil(result)

	result, err = machine.Advance(context.Background(), []byte("retry-input"), Hash{}, false)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(CompletionStatusAccepted, result.Status)
	s.Require().Equal(expectedOutputsHash, result.OutputsHash)
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestRunUsesHardExecutionSpanWhenMaximumIsZero() {
	const startCycle uint64 = 7
	// The fixed window mirrors the emulator's imcyclemax: 2^48 - 1 ahead.
	const executionCycleSpan uint64 = (1 << Log2MaxMCyclesPerAdvanceState) - 1

	tests := []struct {
		name    string
		reqType requestType
		params  model.ExecutionParameters
	}{
		{
			name:    "advance",
			reqType: AdvanceStateRequest,
			params: model.ExecutionParameters{
				AdvanceIncCycles:   ^uint64(0),
				AdvanceIncDeadline: time.Second,
				AdvanceMaxDeadline: time.Second,
			},
		},
		{
			name:    "inspect",
			reqType: InspectStateRequest,
			params: model.ExecutionParameters{
				InspectIncCycles:   ^uint64(0),
				InspectIncDeadline: time.Second,
				InspectMaxDeadline: time.Second,
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			mockBackend := NewMockBackend()
			mockBackend.On("Run", startCycle+executionCycleSpan, mock.AnythingOfType("time.Duration")).
				Return(McycleOverflow, nil)
			mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
				Return(startCycle+executionCycleSpan, nil).Once()

			machine := &machineImpl{
				backend: mockBackend,
				logger:  s.logger,
				params:  tt.params,
			}

			_, err := machine.run(
				context.Background(), tt.reqType, false,
				executionBounds{
					start: startCycle, limit: startCycle + executionCycleSpan,
					span: executionCycleSpan,
				},
			)
			s.Require().ErrorIs(err, ErrReachedLimitMcycle)
			s.Require().ErrorIs(err, ErrMcycleOverflow)
			s.Contains(err.Error(), fmt.Sprintf("executed_cycles=%d", executionCycleSpan))
			mockBackend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestExecutionCycleBoundsAcceptsMaxSafeStart() {
	const maxSafeStart uint64 = ^uint64(0) - model.MaxExecutionCycleSpan

	mockBackend := NewMockBackend()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(maxSafeStart, nil)
	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:     time.Second,
			AdvanceIncCycles: 1,
		},
	}

	bounds, err := machine.executionCycleBounds(context.Background(), AdvanceStateRequest)
	s.Require().NoError(err)
	s.Equal(maxSafeStart, bounds.start)
	s.Equal(maxSafeStart+model.MaxExecutionCycleSpan, bounds.limit)
	s.Equal(model.MaxExecutionCycleSpan, bounds.span)
	s.False(bounds.configured)
	mockBackend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestExecutionCycleBoundsUsesRequestSpecificMaximum() {
	const start uint64 = 17
	for _, test := range []struct {
		name    string
		reqType requestType
		span    uint64
		params  model.ExecutionParameters
	}{
		{
			name: "advance configured", reqType: AdvanceStateRequest, span: 101,
			params: model.ExecutionParameters{AdvanceIncCycles: 1, AdvanceMaxCycles: 101},
		},
		{
			name: "inspect configured", reqType: InspectStateRequest, span: 202,
			params: model.ExecutionParameters{InspectIncCycles: 1, InspectMaxCycles: 202},
		},
		{
			name: "advance zero uses fixed", reqType: AdvanceStateRequest, span: model.MaxExecutionCycleSpan,
			params: model.ExecutionParameters{AdvanceIncCycles: 1},
		},
		{
			name: "inspect zero uses fixed", reqType: InspectStateRequest, span: model.MaxExecutionCycleSpan,
			params: model.ExecutionParameters{InspectIncCycles: 1},
		},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil)
			machine := &machineImpl{backend: backend, logger: s.logger, params: test.params}

			bounds, err := machine.executionCycleBounds(context.Background(), test.reqType)
			s.Require().NoError(err)
			s.Equal(start, bounds.start)
			s.Equal(start+test.span, bounds.limit)
			s.Equal(test.span, bounds.span)
			if test.params.AdvanceMaxCycles != 0 || test.params.InspectMaxCycles != 0 {
				s.True(bounds.configured)
			} else {
				s.False(bounds.configured)
			}
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestExecutionCycleBoundsRejectsMaximumAboveHardLimit() {
	for _, reqType := range []requestType{AdvanceStateRequest, InspectStateRequest} {
		s.Run(reqType.String(), func() {
			backend := NewMockBackend()
			params := model.ExecutionParameters{
				AdvanceIncCycles: 1,
				AdvanceMaxCycles: model.MaxExecutionCycles,
				InspectIncCycles: 1,
			}
			if reqType == InspectStateRequest {
				params.AdvanceMaxCycles = 0
				params.InspectMaxCycles = model.MaxExecutionCycles
			}
			machine := &machineImpl{backend: backend, logger: s.logger, params: params}

			_, err := machine.executionCycleBounds(context.Background(), reqType)
			s.Require().ErrorIs(err, ErrReachedLimitMcycle)
			s.Contains(err.Error(), reqType.String())
			s.Contains(err.Error(), fmt.Sprint(model.MaxExecutionCycles))
			backend.AssertNotCalled(s.T(), "ReadMCycle", mock.Anything)
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestProcessRejectsInvalidMaximumBeforeCMIO() {
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	machine := &machineImpl{
		backend: backend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			AdvanceIncCycles: 1,
			AdvanceMaxCycles: model.MaxExecutionCycles,
			FastDeadline:     time.Second,
		},
	}

	response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
	s.Require().Nil(response)
	s.Require().ErrorIs(err, ErrReachedLimitMcycle)
	backend.AssertNotCalled(s.T(), "ReadMCycle", mock.Anything)
	backend.AssertNotCalled(s.T(), "SendCmioResponse",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestProcessRejectsZeroIncrementBeforeCMIO() {
	for _, test := range []struct {
		name    string
		reqType requestType
		invoke  func(*machineImpl) error
	}{
		{
			name:    "advance",
			reqType: AdvanceStateRequest,
			invoke: func(machine *machineImpl) error {
				_, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
				return err
			},
		},
		{
			name:    "inspect",
			reqType: InspectStateRequest,
			invoke: func(machine *machineImpl) error {
				_, err := machine.Inspect(context.Background(), []byte("query"))
				return err
			},
		},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("CmioRxBufferSize").Return(uint64(1024)).Maybe()
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(1), nil).Maybe()
			params := model.ExecutionParameters{
				FastDeadline:       time.Second,
				AdvanceIncDeadline: time.Second,
				AdvanceMaxDeadline: time.Second,
				InspectIncDeadline: time.Second,
				InspectMaxDeadline: time.Second,
			}
			machine := &machineImpl{backend: backend, logger: s.logger, params: params}

			err := test.invoke(machine)
			s.Require().Error(err)
			s.Contains(err.Error(), test.reqType.String())
			s.Contains(err.Error(), "increment")
			backend.AssertNotCalled(s.T(), "SendCmioResponse",
				mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			backend.AssertNotCalled(s.T(), "Run", mock.Anything, mock.Anything)
			backend.AssertNotCalled(s.T(), "RunAndCollectRootHashes", mock.Anything, mock.Anything, mock.Anything)
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestConfiguredCycleEndpointCompletionSucceeds() {
	const start, span = uint64(10), uint64(5)
	expectedOutputsHash := randomFakeHash()
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil).Once()
	backend.On(
		"SendCmioResponse",
		uint16(AdvanceStateRequest), []byte("input"), mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	backend.On("Run", start+span, mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start+span, nil).Once()
	backend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), expectedOutputsHash[:], nil)
	machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
		AdvanceIncCycles: span + 100, AdvanceMaxCycles: span,
		AdvanceIncDeadline: time.Second, AdvanceMaxDeadline: time.Second,
		FastDeadline: time.Second,
	}}

	response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
	s.Require().NoError(err)
	s.Require().Equal(CompletionStatusAccepted, response.Status)
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestConfiguredCycleEndpointHaltSucceeds() {
	const start, span = uint64(20), uint64(7)
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil).Once()
	backend.On(
		"SendCmioResponse",
		uint16(AdvanceStateRequest), []byte("input"), mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	backend.On("Run", start+span, mock.AnythingOfType("time.Duration")).Return(Halted, nil)
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start+span, nil).Once()
	machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
		AdvanceIncCycles: span + 100, AdvanceMaxCycles: span,
		AdvanceIncDeadline: time.Second, AdvanceMaxDeadline: time.Second,
		FastDeadline: time.Second,
	}}

	response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
	s.Require().NoError(err)
	s.Require().Equal(CompletionStatusHalted, response.Status)
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestConfiguredCycleExhaustionFails() {
	const start, span = uint64(10), uint64(5)
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil).Once()
	backend.On(
		"SendCmioResponse",
		uint16(InspectStateRequest), []byte("query"), mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	backend.On("Run", start+span, mock.AnythingOfType("time.Duration")).Return(ReachedTargetMcycle, nil)
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start+span, nil).Once()
	machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
		InspectIncCycles: span + 100, InspectMaxCycles: span,
		InspectIncDeadline: time.Second, InspectMaxDeadline: time.Second,
		FastDeadline: time.Second,
	}}

	response, err := machine.Inspect(context.Background(), []byte("query"))
	s.Require().NotNil(response)
	s.Require().Equal(CompletionStatusUnknown, response.Status)
	s.Require().Empty(response.Reports)
	s.Require().ErrorIs(err, ErrReachedLimitMcycle)
	s.Contains(err.Error(), "inspect execution reached configured cycle limit")
	s.Contains(err.Error(), "start=10 requested_span=5")
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestAdvanceConfiguredCycleExhaustionReturnsNoResult() {
	const start, span = uint64(30), uint64(9)
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil).Once()
	backend.On(
		"SendCmioResponse",
		uint16(AdvanceStateRequest), []byte("input"), mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	backend.On("Run", start+span, mock.AnythingOfType("time.Duration")).Return(ReachedTargetMcycle, nil)
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start+span, nil).Once()
	machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
		AdvanceIncCycles: span + 100, AdvanceMaxCycles: span,
		AdvanceIncDeadline: time.Second, AdvanceMaxDeadline: time.Second,
		FastDeadline: time.Second,
	}}

	response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
	s.Require().Nil(response)
	s.Require().ErrorIs(err, ErrReachedLimitMcycle)
	s.Contains(err.Error(), "advance execution reached configured cycle limit")
	s.Contains(err.Error(), "start=30 requested_span=9")
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestAdvanceFixedSpanExhaustionPreservesMachineOverflow() {
	const start = uint64(30)
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil).Once()
	backend.On(
		"SendCmioResponse",
		uint16(AdvanceStateRequest), []byte("input"), mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	// With no configured cap, the emulator enforces the span itself and reports
	// the mcycle-overflow fixed point as a break reason.
	backend.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).
		Return(McycleOverflow, nil)
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(start+model.MaxExecutionCycleSpan, nil).Once()
	machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
		AdvanceIncCycles:   ^uint64(0),
		AdvanceIncDeadline: time.Second, AdvanceMaxDeadline: time.Second,
		FastDeadline: time.Second,
	}}

	response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
	s.Require().Nil(response)
	s.Require().ErrorIs(err, ErrReachedLimitMcycle)
	s.Require().ErrorIs(err, ErrMcycleOverflow)
	s.Contains(err.Error(), "advance execution reached fixed (machine imcyclemax) cycle limit")
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestAdvanceConfiguredLimitTiePreservesMachineOverflowPrecedence() {
	const start = uint64(30)
	configuredSpan := model.MaxExecutionCycleSpan
	limit := start + configuredSpan
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil).Once()
	backend.On(
		"SendCmioResponse",
		uint16(AdvanceStateRequest), []byte("input"), mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil).Once()
	backend.On("Run", limit, mock.AnythingOfType("time.Duration")).Return(McycleOverflow, nil).Once()
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(limit, nil).Once()
	machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
		AdvanceIncCycles:   ^uint64(0),
		AdvanceIncDeadline: time.Second,
		AdvanceMaxDeadline: time.Second,
		AdvanceMaxCycles:   configuredSpan,
		FastDeadline:       time.Second,
	}}

	response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
	s.Require().Nil(response)
	s.Require().ErrorIs(err, ErrReachedLimitMcycle)
	s.Require().ErrorIs(err, ErrMcycleOverflow)
	s.Contains(err.Error(), "advance execution stopped at machine imcyclemax coincident with configured target")
	s.Contains(err.Error(), fmt.Sprintf("requested_span=%d", configuredSpan))
	s.Contains(err.Error(), fmt.Sprintf("executed_cycles=%d", configuredSpan))
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestAdvanceStartingAtMachineMaximumPreservesOverflowOrigin() {
	for _, test := range []struct {
		name          string
		configuredMax uint64
	}{
		{name: "no operator cap", configuredMax: 0},
		{name: "configured cap also saturates", configuredMax: 1},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("CmioRxBufferSize").Return(uint64(1024))
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
				Return(^uint64(0), nil).Once()
			backend.On(
				"SendCmioResponse",
				uint16(AdvanceStateRequest), []byte("input"), mock.Anything, mock.AnythingOfType("time.Duration"),
			).Return(nil).Once()
			backend.On("Run", ^uint64(0), mock.AnythingOfType("time.Duration")).
				Return(McycleOverflow, nil).Once()
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
				Return(^uint64(0), nil).Once()
			machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
				AdvanceIncCycles:   1,
				AdvanceIncDeadline: time.Second,
				AdvanceMaxDeadline: time.Second,
				AdvanceMaxCycles:   test.configuredMax,
				FastDeadline:       time.Second,
			}}

			response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
			s.Require().Nil(response)
			s.Require().ErrorIs(err, ErrReachedLimitMcycle)
			s.Require().ErrorIs(err, ErrMcycleOverflow)
			expectedRequestedSpan := test.configuredMax
			if expectedRequestedSpan == 0 {
				expectedRequestedSpan = model.MaxExecutionCycleSpan
			}
			s.Contains(err.Error(), fmt.Sprintf("requested_span=%d", expectedRequestedSpan))
			s.Contains(err.Error(), "target_span=0")
			s.Contains(err.Error(), "executed_cycles=0")
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestRunRejectsNonOverflowReasonAtMachineMaximum() {
	for _, breakReason := range []BreakReason{ReachedTargetMcycle, YieldedSoftly} {
		s.Run(fmt.Sprintf("break reason %d", breakReason), func() {
			backend := NewMockBackend()
			backend.On("Run", ^uint64(0), mock.AnythingOfType("time.Duration")).
				Return(breakReason, nil).Once()
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
				Return(^uint64(0), nil).Once()
			machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
				FastDeadline: time.Second,
			}}

			result, err := machine.runIncrementInterval(
				context.Background(), ^uint64(0), ^uint64(0), 1, nil, time.Second,
			)

			s.Equal(breakReason, result.breakReason)
			s.Equal(^uint64(0), result.currentCycle)
			s.Require().ErrorIs(err, ErrMachineInternal)
			s.Contains(err.Error(), "instead of mcycle overflow at MaxUint64")
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestInspectInheritedMcycleOverflowDoesNotClaimLocalLimitExhaustion() {
	const start = uint64(100)

	for _, test := range []struct {
		name            string
		configuredMax   uint64
		requestedSpan   uint64
		executedCycles  uint64
		stopDescription string
	}{
		{
			name:            "before configured limit",
			configuredMax:   50,
			requestedSpan:   50,
			executedCycles:  7,
			stopDescription: "stopped before configured cycle limit",
		},
		{
			name:            "at configured limit",
			configuredMax:   7,
			requestedSpan:   7,
			executedCycles:  7,
			stopDescription: "stopped at machine imcyclemax coincident with configured target",
		},
		{
			name:            "before zero default fixed limit",
			requestedSpan:   model.MaxExecutionCycleSpan,
			executedCycles:  7,
			stopDescription: "stopped before fixed cycle limit",
		},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("CmioRxBufferSize").Return(uint64(1024))
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil).Once()
			backend.On(
				"SendCmioResponse",
				uint16(InspectStateRequest), []byte("query"), nil, mock.AnythingOfType("time.Duration"),
			).Return(nil).Once()
			backend.On("Run", start+test.requestedSpan, mock.AnythingOfType("time.Duration")).
				Return(McycleOverflow, nil).Once()
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
				Return(start+test.executedCycles, nil).Once()

			machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
				FastDeadline:       time.Second,
				InspectIncCycles:   ^uint64(0),
				InspectIncDeadline: time.Second,
				InspectMaxDeadline: time.Second,
				InspectMaxCycles:   test.configuredMax,
			}}

			response, err := machine.Inspect(context.Background(), []byte("query"))
			s.Require().NotNil(response)
			s.Equal(CompletionStatusUnknown, response.Status)
			s.Empty(response.Reports)
			s.Require().ErrorIs(err, ErrReachedLimitMcycle)
			s.Require().ErrorIs(err, ErrMcycleOverflow)
			s.Contains(err.Error(), test.stopDescription)
			s.Contains(err.Error(), fmt.Sprintf("requested_span=%d", test.requestedSpan))
			s.Contains(err.Error(), fmt.Sprintf("executed_cycles=%d", test.executedCycles))
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestExecutionCycleBoundsSaturatesAtMachineMaximum() {
	for _, test := range []struct {
		name       string
		start      uint64
		configured uint64
	}{
		{
			name:  "configured cap beyond representable range",
			start: ^uint64(0) - 10, configured: 11,
		},
		{
			name:  "zero cap",
			start: ^uint64(0) - model.MaxExecutionCycleSpan + 1, configured: 0,
		},
		{
			name:       "maximum cap",
			start:      ^uint64(0) - model.MaxExecutionCycleSpan + 1,
			configured: model.MaxExecutionCycleSpan,
		},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(test.start, nil)
			machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
				AdvanceIncCycles: 1,
				AdvanceMaxCycles: test.configured,
			}}

			bounds, err := machine.executionCycleBounds(context.Background(), AdvanceStateRequest)
			s.Require().NoError(err)
			s.Equal(test.start, bounds.start)
			s.Equal(^uint64(0), bounds.limit)
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestExecutionCycleBoundsUsesFullWindowWhenUncapped() {

	s.Run("window endpoint is start plus 2^48 minus 1", func() {
		const start = uint64(1000)
		backend := NewMockBackend()
		backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil)
		machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
			AdvanceIncCycles: 1,
		}}

		bounds, err := machine.executionCycleBounds(context.Background(), AdvanceStateRequest)
		s.Require().NoError(err)
		s.Equal(start, bounds.start)
		s.Equal(start+model.MaxExecutionCycleSpan, bounds.limit)
		backend.AssertExpectations(s.T())
	})
}

func (s *ImplementationSuite) TestExecutionResponseLimitErrorsExposeOnlyCounts() {
	for _, test := range []struct {
		kind     string
		count    int
		capacity int
		sentinel error
	}{
		{kind: "output", count: maxOutputs + 1, capacity: maxOutputs, sentinel: ErrOutputsLimitExceeded},
		{kind: "report", count: maxReports + 1, capacity: maxReports, sentinel: ErrReportsLimitExceeded},
	} {
		err := executionResponseLimitError(
			AdvanceStateRequest, test.kind, test.count, test.capacity, test.sentinel,
		)
		s.Require().ErrorIs(err, test.sentinel)
		s.Contains(err.Error(), fmt.Sprintf("advance %s count %d", test.kind, test.count))
		s.Contains(err.Error(), fmt.Sprintf("capacity %d", test.capacity))
	}
}

func (s *ImplementationSuite) TestInspectUsesConfiguredCycleIncrement() {
	require := s.Require()
	const startCycle uint64 = 7
	const inspectIncCycles uint64 = 137

	mockBackend := NewMockBackend()
	mockBackend.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(startCycle, nil).Once()
	mockBackend.On(
		"SendCmioResponse",
		uint16(InspectStateRequest), mock.Anything, mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	mockBackend.On("Run", startCycle+inspectIncCycles, mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(startCycle+inspectIncCycles, nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), []byte(nil), nil)

	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second,
			InspectIncCycles:   inspectIncCycles,
			InspectIncDeadline: time.Second,
			InspectMaxDeadline: time.Second,
		},
	}

	response, err := machine.Inspect(context.Background(), []byte("query"))
	require.NoError(err)
	require.Equal(CompletionStatusAccepted, response.Status)
	mockBackend.AssertExpectations(s.T())
}

// Test Inspect method
func (s *ImplementationSuite) TestInspect() {
	require := s.Require()
	ctx := context.Background()

	// Test successful inspect (accepted)
	mockBackend := NewMockBackend()
	mockBackend.SetupAccepted(InspectStateRequest)

	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			InspectIncCycles:   100,
			InspectIncDeadline: time.Second * 1,
			InspectMaxDeadline: time.Second * 10,
		},
	}

	query := []byte("test query")
	response, err := machine.Inspect(ctx, query)
	require.NoError(err)
	require.Equal(CompletionStatusAccepted, response.Status)
	require.Empty(response.Reports)
	mockBackend.AssertExpectations(s.T())

	// Test inspect with rejection
	mockBackend2 := NewMockBackend()
	mockBackend2.SetupRejected(InspectStateRequest)
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			InspectIncCycles:   100,
			InspectIncDeadline: time.Second * 1,
			InspectMaxDeadline: time.Second * 10,
		},
	}
	response, err = machine2.Inspect(ctx, query)
	require.NoError(err)
	require.Equal(CompletionStatusRejected, response.Status)
	require.Empty(response.Reports)
	mockBackend2.AssertExpectations(s.T())

	// Test inspect with exception
	mockBackend3 := NewMockBackend()
	mockBackend3.SetupException(InspectStateRequest)
	machine3 := &machineImpl{
		backend: mockBackend3,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			InspectIncCycles:   100,
			InspectIncDeadline: time.Second * 1,
			InspectMaxDeadline: time.Second * 10,
		},
	}
	response, err = machine3.Inspect(ctx, query)
	require.NoError(err)
	require.Equal(CompletionStatusException, response.Status)
	require.Equal([]byte("exception data"), response.ExceptionData)
	require.Empty(response.Reports)
	mockBackend3.AssertExpectations(s.T())

	// A halt is a completed guest outcome. Reports emitted before the halt are
	// part of that completed inspect response.
	mockBackendHalted := NewMockBackend()
	mockBackendHalted.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackendHalted.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil).Once()
	mockBackendHalted.On("SendCmioResponse", uint16(InspectStateRequest), query, nil,
		mock.AnythingOfType("time.Duration")).Return(nil).Once()
	mockBackendHalted.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).
		Return(YieldedAutomatically, nil).Once()
	mockBackendHalted.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(50), nil).Once()
	mockBackendHalted.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).
		Return(uint8(0), uint16(AutomaticYieldReasonReport), []byte("before halt"), nil).Once()
	mockBackendHalted.On("Run", uint64(150), mock.AnythingOfType("time.Duration")).
		Return(Halted, nil).Once()
	mockBackendHalted.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(75), nil).Once()
	machineHalted := &machineImpl{
		backend: mockBackendHalted,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			InspectIncCycles:   100,
			InspectIncDeadline: time.Second,
			InspectMaxDeadline: time.Second * 10,
		},
	}
	response, err = machineHalted.Inspect(ctx, query)
	require.NoError(err)
	require.Equal(CompletionStatusHalted, response.Status)
	require.Equal([]Report{[]byte("before halt")}, response.Reports)
	mockBackendHalted.AssertExpectations(s.T())

	// An internal stop is not a completed guest outcome. It returns Unknown
	// with the partial report prefix and a typed execution error.
	mockBackendFailed := NewMockBackend()
	mockBackendFailed.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackendFailed.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil).Once()
	mockBackendFailed.On("SendCmioResponse", uint16(InspectStateRequest), query, nil,
		mock.AnythingOfType("time.Duration")).Return(nil).Once()
	mockBackendFailed.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).
		Return(YieldedAutomatically, nil).Once()
	mockBackendFailed.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(50), nil).Once()
	mockBackendFailed.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).
		Return(uint8(0), uint16(AutomaticYieldReasonReport), []byte("partial"), nil).Once()
	mockBackendFailed.On("Run", uint64(150), mock.AnythingOfType("time.Duration")).
		Return(Failed, nil).Once()
	mockBackendFailed.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(75), nil).Once()
	machineFailed := &machineImpl{
		backend: mockBackendFailed,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			InspectIncCycles:   100,
			InspectIncDeadline: time.Second,
			InspectMaxDeadline: time.Second * 10,
		},
	}
	response, err = machineFailed.Inspect(ctx, query)
	require.ErrorIs(err, ErrMachineInternal)
	require.Equal(CompletionStatusUnknown, response.Status)
	require.Equal([]Report{[]byte("partial")}, response.Reports)
	mockBackendFailed.AssertExpectations(s.T())

	// Test inspect with payload too large
	mockBackend4 := NewMockBackend()
	mockBackend4.On("CmioRxBufferSize").Return(uint64(5))
	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			InspectIncCycles:   100,
			InspectIncDeadline: time.Second * 1,
			InspectMaxDeadline: time.Second * 10,
		},
	}
	largeQuery := make([]byte, 10)
	response, err = machine4.Inspect(ctx, largeQuery)
	require.NotNil(response)
	require.Equal(CompletionStatusUnknown, response.Status)
	require.ErrorIs(err, ErrPayloadLengthLimitExceeded)
	mockBackend4.AssertExpectations(s.T())

	// Inspect uses the same saturating target as advance. Reaching MaxUint64 is
	// reported as a machine overflow and never as a completed input result.
	mockBackendOverflow := NewMockBackend()
	mockBackendOverflow.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackendOverflow.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(^uint64(0)-uint64(999), nil).Once()
	mockBackendOverflow.On("SendCmioResponse", uint16(InspectStateRequest), query, nil,
		mock.AnythingOfType("time.Duration")).Return(nil).Once()
	mockBackendOverflow.On("Run", ^uint64(0), mock.AnythingOfType("time.Duration")).
		Return(McycleOverflow, nil).Once()
	mockBackendOverflow.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(^uint64(0), nil).Once()
	machineOverflow := &machineImpl{
		backend: mockBackendOverflow,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second,
			InspectIncCycles:   ^uint64(0),
			InspectIncDeadline: time.Second,
			InspectMaxDeadline: time.Second,
			InspectMaxCycles:   1000,
		},
	}
	response, err = machineOverflow.Inspect(ctx, query)
	require.NotNil(response)
	require.Equal(CompletionStatusUnknown, response.Status)
	require.Empty(response.Reports)
	require.ErrorIs(err, ErrReachedLimitMcycle)
	require.ErrorIs(err, ErrMcycleOverflow)
	require.Contains(err.Error(), "requested_span=1000")
	require.Contains(err.Error(), "target_span=999")
	require.Contains(err.Error(), "executed_cycles=999")
	mockBackendOverflow.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestAdvanceCanonicalizesTerminalBoundaryHash() {
	for _, test := range []struct {
		name                 string
		yieldReason          manualYieldReason
		status               CompletionStatus
		terminalAtBoundary   bool
		sameRunPriorBoundary bool
		immediateYield       bool
	}{
		{
			name:               "accepted at first boundary",
			yieldReason:        ManualYieldReasonAccepted,
			status:             CompletionStatusAccepted,
			terminalAtBoundary: true,
		},
		{
			name:               "rejected at first boundary",
			yieldReason:        ManualYieldReasonRejected,
			status:             CompletionStatusRejected,
			terminalAtBoundary: true,
		},
		{
			name:               "exception at first boundary",
			yieldReason:        ManualYieldReasonException,
			status:             CompletionStatusException,
			terminalAtBoundary: true,
		},
		{
			name:               "off-boundary terminal keeps prior boundary",
			yieldReason:        ManualYieldReasonAccepted,
			status:             CompletionStatusAccepted,
			terminalAtBoundary: false,
		},
		{
			name:                 "same run prior boundary survives off-boundary terminal",
			yieldReason:          ManualYieldReasonAccepted,
			status:               CompletionStatusAccepted,
			sameRunPriorBoundary: true,
		},
		{
			name:           "immediate off-boundary yield drops its fixed-point sample",
			yieldReason:    ManualYieldReasonAccepted,
			status:         CompletionStatusAccepted,
			immediateYield: true,
		},
	} {
		s.Run(test.name, func() {
			collectedHash := randomFakeHash()
			terminalSample := randomFakeHash()
			outputsHash := randomFakeHash()

			backend := NewMockBackend()
			backend.On("CmioRxBufferSize").Return(uint64(1024))
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
				Return(uint64(0), nil).Once()
			backend.On(
				"SendCmioResponse",
				uint16(AdvanceStateRequest), []byte("input"), mock.Anything, mock.AnythingOfType("time.Duration"),
			).
				Return(nil).Once()
			switch {
			case test.terminalAtBoundary:
				backend.On(
					"RunAndCollectRootHashes",
					MCycleComputationHashPeriod,
					mock.AnythingOfType("*machine.HashCollectorState"),
					mock.AnythingOfType("time.Duration"),
				).Run(func(args mock.Arguments) {
					state := args.Get(1).(*HashCollectorState)
					state.Hashes = append(state.Hashes, collectedHash)
					state.MCyclePhase = 0
				}).Return(YieldedManually, nil).Once()
				backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
					Return(MCycleComputationHashPeriod, nil).Once()
			case test.sameRunPriorBoundary:
				// RunAndCollectRootHashes may cross and collect a prior
				// boundary, then stop at a manual yield one cycle later in the
				// same call. Emulator 0.21 also appends the off-boundary
				// fixed-point sample; only that final entry may be dropped.
				backend.On(
					"RunAndCollectRootHashes",
					2*MCycleComputationHashPeriod,
					mock.AnythingOfType("*machine.HashCollectorState"),
					mock.AnythingOfType("time.Duration"),
				).Run(func(args mock.Arguments) {
					state := args.Get(1).(*HashCollectorState)
					state.Hashes = append(state.Hashes, collectedHash, terminalSample)
					state.MCyclePhase = 1
				}).Return(YieldedManually, nil).Once()
				backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
					Return(MCycleComputationHashPeriod+1, nil).Once()
			case test.immediateYield:
				// Even a yield right after delivery is a fixed point: 0.21
				// appends its sample at a nonzero phase, and the node must
				// still drop it.
				backend.On(
					"RunAndCollectRootHashes",
					MCycleComputationHashPeriod,
					mock.AnythingOfType("*machine.HashCollectorState"),
					mock.AnythingOfType("time.Duration"),
				).Run(func(args mock.Arguments) {
					state := args.Get(1).(*HashCollectorState)
					state.Hashes = append(state.Hashes, terminalSample)
					state.MCyclePhase = 1
				}).Return(YieldedManually, nil).Once()
				backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
					Return(uint64(1), nil).Once()
			default:
				// The first call reaches and records a real prior boundary.
				backend.On(
					"RunAndCollectRootHashes",
					MCycleComputationHashPeriod,
					mock.AnythingOfType("*machine.HashCollectorState"),
					mock.AnythingOfType("time.Duration"),
				).Run(func(args mock.Arguments) {
					state := args.Get(1).(*HashCollectorState)
					state.Hashes = append(state.Hashes, collectedHash)
					state.MCyclePhase = 0
				}).Return(ReachedTargetMcycle, nil).Once()
				backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
					Return(MCycleComputationHashPeriod, nil).Once()
				// The second call manually yields one cycle later; 0.21 appends
				// the off-boundary fixed-point sample, which is dropped while
				// the prior boundary remains in the collection.
				backend.On(
					"RunAndCollectRootHashes",
					2*MCycleComputationHashPeriod,
					mock.AnythingOfType("*machine.HashCollectorState"),
					mock.AnythingOfType("time.Duration"),
				).Run(func(args mock.Arguments) {
					state := args.Get(1).(*HashCollectorState)
					state.Hashes = append(state.Hashes, terminalSample)
					state.MCyclePhase = 1
				}).Return(YieldedManually, nil).Once()
				backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
					Return(MCycleComputationHashPeriod+1, nil).Once()
			}
			backend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).
				Return(uint8(0), uint16(test.yieldReason), outputsHash[:], nil).Once()

			advanceIncrement := MCycleComputationHashPeriod
			if test.sameRunPriorBoundary {
				advanceIncrement = 2 * MCycleComputationHashPeriod
			}
			machine := &machineImpl{
				backend: backend,
				logger:  s.logger,
				params: model.ExecutionParameters{
					FastDeadline:       time.Second,
					AdvanceIncCycles:   advanceIncrement,
					AdvanceIncDeadline: time.Second,
					AdvanceMaxDeadline: time.Second,
				},
			}

			response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, true)
			s.Require().NoError(err)
			s.Require().NotNil(response)
			s.Equal(test.status, response.Status)
			if test.terminalAtBoundary || test.immediateYield {
				s.Empty(response.PeriodicStateHashes)
				s.Equal(InputEntryCapacity, response.PaddingRepetitions)
			} else {
				s.Equal([]Hash{collectedHash}, response.PeriodicStateHashes)
				s.Equal(InputEntryCapacity-1, response.PaddingRepetitions)
			}
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestRunDiscardsOverflowCollection() {
	const fixedEndpoint uint64 = model.MaxExecutionCycleSpan
	const startCycle uint64 = fixedEndpoint - mcycleComputationHashChunkSize
	terminalSample := randomFakeHash()
	backend := NewMockBackend()
	backend.On(
		"RunAndCollectRootHashes",
		fixedEndpoint,
		mock.AnythingOfType("*machine.HashCollectorState"),
		mock.AnythingOfType("time.Duration"),
	).Run(func(args mock.Arguments) {
		state := args.Get(1).(*HashCollectorState)
		state.Hashes = append(state.Hashes, terminalSample)
		state.MCyclePhase = 7
	}).Return(McycleOverflow, nil).Once()
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(fixedEndpoint, nil).Once()
	machine := &machineImpl{
		backend: backend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			AdvanceIncCycles:   fixedEndpoint,
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: time.Second,
		},
	}

	result, err := machine.run(
		context.Background(), AdvanceStateRequest, true, testExecutionBounds(startCycle, fixedEndpoint),
	)

	s.Require().ErrorIs(err, ErrMcycleOverflow)
	s.Empty(result.outputs)
	s.Empty(result.reports)
	s.Empty(result.periodicStateHashes)
	s.Zero(result.paddingRepetitions)
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestRunCapsCollectorChunksAndPreservesCollectionState() {
	firstPeriodicHash := randomFakeHash()
	terminalHash := randomFakeHash()
	backend := NewMockBackend()
	backend.On(
		"RunAndCollectRootHashes",
		mcycleComputationHashChunkSize,
		mock.AnythingOfType("*machine.HashCollectorState"),
		mock.AnythingOfType("time.Duration"),
	).Run(func(args mock.Arguments) {
		state := args.Get(1).(*HashCollectorState)
		s.Zero(state.MCyclePhase)
		s.Empty(state.Hashes)
		state.Hashes = append(state.Hashes, firstPeriodicHash)
		state.MCyclePhase = 7
	}).Return(ReachedTargetMcycle, nil).Once()
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(mcycleComputationHashChunkSize, nil).Once()
	backend.On(
		"RunAndCollectRootHashes",
		2*mcycleComputationHashChunkSize,
		mock.AnythingOfType("*machine.HashCollectorState"),
		mock.AnythingOfType("time.Duration"),
	).Run(func(args mock.Arguments) {
		state := args.Get(1).(*HashCollectorState)
		s.Equal(uint64(7), state.MCyclePhase)
		s.Equal([]Hash{firstPeriodicHash}, state.Hashes)
		state.Hashes = append(state.Hashes, terminalHash)
		state.MCyclePhase = 11
	}).Return(YieldedManually, nil).Once()
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(2*mcycleComputationHashChunkSize, nil).Once()
	machine := &machineImpl{
		backend: backend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			AdvanceIncCycles:   3 * mcycleComputationHashChunkSize,
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: time.Second,
		},
	}

	result, err := machine.run(
		context.Background(), AdvanceStateRequest, true,
		testExecutionBounds(0, 3*mcycleComputationHashChunkSize),
	)

	s.Require().NoError(err)
	s.Equal([]Hash{firstPeriodicHash}, result.periodicStateHashes)
	s.Equal(InputEntryCapacity-1, result.paddingRepetitions)
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestRunRejectsFixedPointWithoutAppendedHash() {
	backend := NewMockBackend()
	backend.On(
		"RunAndCollectRootHashes",
		MCycleComputationHashPeriod,
		mock.AnythingOfType("*machine.HashCollectorState"),
		mock.AnythingOfType("time.Duration"),
	).Return(YieldedManually, nil).Once()
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(MCycleComputationHashPeriod, nil).Once()
	machine := &machineImpl{
		backend: backend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			AdvanceIncCycles:   MCycleComputationHashPeriod,
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: time.Second,
		},
	}

	_, err := machine.run(
		context.Background(), AdvanceStateRequest, true,
		testExecutionBounds(0, model.MaxExecutionCycleSpan),
	)

	s.Require().ErrorIs(err, ErrMachineInternal)
	s.Contains(err.Error(), "without appending its state hash")
	backend.AssertExpectations(s.T())
}

// Test Store method
func (s *ImplementationSuite) TestStore() {
	require := s.Require()
	ctx := context.Background()

	// Test successful store
	mockBackend := NewMockBackend()
	mockBackend.On("Store", "/tmp/test", mock.AnythingOfType("time.Duration")).Return(nil)
	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			StoreDeadline: time.Second * 5,
		},
	}

	err := machine.Store(ctx, "/tmp/test")
	require.NoError(err)
	mockBackend.AssertExpectations(s.T())

	// Test store with backend error
	mockBackend2 := NewMockBackend()
	mockBackend2.On("Store", "/tmp/test", mock.AnythingOfType("time.Duration")).Return(errors.New("store failed"))
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			StoreDeadline: time.Second * 5,
		},
	}
	err = machine2.Store(ctx, "/tmp/test")
	require.Error(err)
	require.ErrorIs(err, ErrMachineInternal)
	require.Contains(err.Error(), "could not store the machine state")

	// Test store with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = machine.Store(canceledCtx, "/tmp/test")
	require.ErrorIs(err, ErrCanceled)
	mockBackend2.AssertExpectations(s.T())
}

// Test Close method
func (s *ImplementationSuite) TestClose() {
	require := s.Require()

	// Test successful close
	mockBackend := NewMockBackend()
	mockBackend.On("ShutdownServer", mock.AnythingOfType("time.Duration")).Return(nil)
	mockBackend.On("Delete").Return()
	machine := &machineImpl{
		backend: mockBackend,
		address: "127.0.0.1:12345",
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}

	err := machine.Close()
	require.NoError(err)
	require.Nil(machine.backend)
	mockBackend.AssertExpectations(s.T())

	// Test close with nil backend
	err = machine.Close()
	require.NoError(err)

	// Test close with shutdown error
	mockBackend2 := NewMockBackend()
	mockBackend2.On("ShutdownServer", mock.AnythingOfType("time.Duration")).Return(errors.New("shutdown failed"))
	mockBackend2.On("Delete").Return()
	machine2 := &machineImpl{
		backend: mockBackend2,
		address: "127.0.0.1:12345",
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}

	err = machine2.Close()
	require.Error(err)
	require.ErrorIs(err, ErrMachineInternal)
	require.ErrorIs(err, ErrOrphanServer)
	require.Nil(machine2.backend)
	mockBackend2.AssertExpectations(s.T())
}

// Test Address method
func (s *ImplementationSuite) TestAddress() {
	require := s.Require()

	machine := &machineImpl{
		address: "127.0.0.1:12345",
	}

	address := machine.Address()
	require.Equal("127.0.0.1:12345", address)
}

// Test helper methods
func (s *ImplementationSuite) TestHelperMethods() {
	require := s.Require()
	ctx := context.Background()

	// Test isAtManualYield
	mockBackend := NewMockBackend()
	mockBackend.On("IsAtManualYield", mock.AnythingOfType("time.Duration")).Return(true, nil)
	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}

	isAtYield, err := machine.isAtManualYield(ctx)
	require.NoError(err)
	require.True(isAtYield)
	mockBackend.AssertExpectations(s.T())

	mockBackend2 := NewMockBackend()
	mockBackend2.On("IsAtManualYield", mock.AnythingOfType("time.Duration")).Return(false, errors.New("yield check failed"))
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	_, err = machine2.isAtManualYield(ctx)
	require.Error(err)
	require.ErrorIs(err, ErrMachineInternal)
	mockBackend2.AssertExpectations(s.T())

	// Test readManualYieldResult
	mockBackend3 := NewMockBackend()
	expectedHash3 := randomFakeHash()
	mockBackend3.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), expectedHash3[:], nil)
	machine3 := &machineImpl{
		backend: mockBackend3,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	manualResult, err := machine3.readManualYieldResult(ctx)
	require.NoError(err)
	require.Equal(CompletionStatusAccepted, manualResult.status)
	require.NotNil(manualResult.data)
	mockBackend3.AssertExpectations(s.T())

	mockBackend4 := NewMockBackend()
	expectedHash4 := randomFakeHash()
	mockBackend4.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonRejected), expectedHash4[:], nil)
	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	manualResult, err = machine4.readManualYieldResult(ctx)
	require.NoError(err)
	require.Equal(CompletionStatusRejected, manualResult.status)
	require.NotNil(manualResult.data)
	mockBackend4.AssertExpectations(s.T())

	mockBackend5 := NewMockBackend()
	mockBackend5.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonException), []byte("exception data"), nil)
	machine5 := &machineImpl{
		backend: mockBackend5,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	manualResult, err = machine5.readManualYieldResult(ctx)
	require.NoError(err)
	require.Equal(CompletionStatusException, manualResult.status)
	require.NotNil(manualResult.data)
	mockBackend5.AssertExpectations(s.T())

	// Test readMCycle
	mockBackend6 := NewMockBackend()
	mockBackend6.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(12345), nil)
	machine6 := &machineImpl{
		backend: mockBackend6,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	cycle, err := machine6.readMCycle(ctx)
	require.NoError(err)
	require.Equal(uint64(12345), cycle)
	mockBackend6.AssertExpectations(s.T())

	mockBackend7 := NewMockBackend()
	mockBackend7.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), errors.New("read cycle failed"))
	machine7 := &machineImpl{
		backend: mockBackend7,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline: time.Second * 5,
		},
	}
	_, err = machine7.readMCycle(ctx)
	require.Error(err)
	require.ErrorIs(err, ErrMachineInternal)
	mockBackend7.AssertExpectations(s.T())

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	_, err = machine.isAtManualYield(canceledCtx)
	require.ErrorIs(err, ErrCanceled)

	_, err = machine3.readManualYieldResult(canceledCtx)
	require.ErrorIs(err, ErrCanceled)

	_, err = machine6.readMCycle(canceledCtx)
	require.ErrorIs(err, ErrCanceled)
}

// Test run method with various scenarios
func (s *ImplementationSuite) TestRun() {
	require := s.Require()
	ctx := context.Background()

	// Test run with manual yield (no outputs/reports)
	mockBackend := NewMockBackend()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	mockBackend.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)

	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	result, err := machine.run(
		ctx, AdvanceStateRequest, false, testExecutionBounds(0, model.MaxExecutionCycleSpan),
	)
	require.NoError(err)
	require.Empty(result.outputs)
	require.Empty(result.reports)
	mockBackend.AssertExpectations(s.T())

	// Test run with read cycle error
	mockBackend2 := NewMockBackend()
	mockBackend2.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	mockBackend2.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), errors.New("read cycle failed"))
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, err = machine2.run(
		ctx, AdvanceStateRequest, false, testExecutionBounds(0, model.MaxExecutionCycleSpan),
	)
	require.Error(err)
	require.Contains(err.Error(), "read cycle failed")
	mockBackend2.AssertExpectations(s.T())

	// Test run with multiple steps
	mockBackend3 := NewMockBackend()
	mockBackend3.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	mockBackend3.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(ReachedTargetMcycle, nil).Once()
	mockBackend3.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil).Once()

	machine3 := &machineImpl{
		backend: mockBackend3,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	_, err = machine3.run(
		ctx, AdvanceStateRequest, false, testExecutionBounds(0, model.MaxExecutionCycleSpan),
	)
	require.NoError(err)
	mockBackend3.AssertExpectations(s.T())

	// Test run with ReceiveCmioRequest error during automatic yield
	mockBackend4 := NewMockBackend()
	mockBackend4.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	mockBackend4.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil)
	mockBackend4.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(0), []byte(nil), errors.New("cmio request failed"))

	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	_, err = machine4.run(
		ctx, AdvanceStateRequest, false, testExecutionBounds(0, model.MaxExecutionCycleSpan),
	)
	require.Error(err)
	require.Contains(err.Error(), "could not read output/report")
	require.Contains(err.Error(), "cmio request failed")
	mockBackend4.AssertExpectations(s.T())

	// Test run with automatic yield producing output then manual yield
	mockBackend5 := NewMockBackend()
	mockBackend5.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	mockBackend5.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).
		Return(YieldedAutomatically, nil).Once()
	mockBackend5.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).
		Return(YieldedManually, nil).Once()
	mockBackend5.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonOutput), []byte("output data"), nil).Once()

	machine5 := &machineImpl{
		backend: mockBackend5,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	result5, err := machine5.run(
		ctx, AdvanceStateRequest, false, testExecutionBounds(0, model.MaxExecutionCycleSpan),
	)
	require.NoError(err)
	require.Len(result5.outputs, 1)
	require.Equal([]byte("output data"), []byte(result5.outputs[0]))
	require.Empty(result5.reports)
	mockBackend5.AssertExpectations(s.T())

	// Test run with automatic yield producing report then manual yield
	mockBackend6 := NewMockBackend()
	mockBackend6.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	mockBackend6.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).
		Return(YieldedAutomatically, nil).Once()
	mockBackend6.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).
		Return(YieldedManually, nil).Once()
	mockBackend6.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonReport), []byte("report data"), nil).Once()

	machine6 := &machineImpl{
		backend: mockBackend6,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	result6, err := machine6.run(
		ctx, AdvanceStateRequest, false, testExecutionBounds(0, model.MaxExecutionCycleSpan),
	)
	require.NoError(err)
	require.Empty(result6.outputs)
	require.Len(result6.reports, 1)
	require.Equal([]byte("report data"), []byte(result6.reports[0]))
	mockBackend6.AssertExpectations(s.T())

}

func (s *ImplementationSuite) TestRunIncrementIntervalPreservesBreakReason() {
	for _, test := range []struct {
		name         string
		breakReason  BreakReason
		currentCycle uint64
	}{
		{"manual yield", YieldedManually, 150},
		{"automatic yield", YieldedAutomatically, 200},
		{"soft yield", YieldedSoftly, 150},
		{"target", ReachedTargetMcycle, 200},
		{"mcycle overflow", McycleOverflow, 199},
		{"halt", Halted, 175},
		{"failure", Failed, 160},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("Run", uint64(200), mock.AnythingOfType("time.Duration")).
				Return(test.breakReason, nil).Once()
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
				Return(test.currentCycle, nil).Once()
			machine := &machineImpl{backend: backend, logger: s.logger}

			result, err := machine.runIncrementInterval(
				context.Background(), 100, 1000, 100, nil, time.Second,
			)

			s.Require().NoError(err)
			s.Equal(test.breakReason, result.breakReason)
			s.Equal(test.currentCycle, result.currentCycle)
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestRunIncrementIntervalErrors() {
	s.Run("already at limit", func() {
		machine := &machineImpl{backend: NewMockBackend(), logger: s.logger}
		result, err := machine.runIncrementInterval(
			context.Background(), 1000, 1000, 100, nil, time.Second,
		)
		s.Require().ErrorIs(err, ErrReachedLimitMcycle)
		s.Equal(uint64(1000), result.currentCycle)
	})

	s.Run("backend error", func() {
		backend := NewMockBackend()
		backend.On("Run", uint64(200), mock.AnythingOfType("time.Duration")).
			Return(Failed, errors.New("run failed")).Once()
		machine := &machineImpl{backend: backend, logger: s.logger}
		_, err := machine.runIncrementInterval(
			context.Background(), 100, 1000, 100, nil, time.Second,
		)
		s.Require().ErrorContains(err, "run failed")
		backend.AssertExpectations(s.T())
	})

	s.Run("read cycle error", func() {
		backend := NewMockBackend()
		backend.On("Run", uint64(200), mock.AnythingOfType("time.Duration")).
			Return(YieldedManually, nil).Once()
		backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
			Return(uint64(0), errors.New("read cycle failed")).Once()
		machine := &machineImpl{backend: backend, logger: s.logger}
		_, err := machine.runIncrementInterval(
			context.Background(), 100, 1000, 100, nil, time.Second,
		)
		s.Require().ErrorContains(err, "read cycle failed")
		backend.AssertExpectations(s.T())
	})

	s.Run("unknown break reason", func() {
		backend := NewMockBackend()
		backend.On("Run", uint64(200), mock.AnythingOfType("time.Duration")).
			Return(BreakReason(99), nil).Once()
		backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
			Return(uint64(150), nil).Once()
		machine := &machineImpl{backend: backend, logger: s.logger}
		result, err := machine.runIncrementInterval(
			context.Background(), 100, 1000, 100, nil, time.Second,
		)
		s.Require().ErrorIs(err, ErrMachineInternal)
		s.Equal(BreakReason(99), result.breakReason)
		backend.AssertExpectations(s.T())
	})
}

// Test process method
func (s *ImplementationSuite) TestProcess() {
	require := s.Require()
	ctx := context.Background()
	expectedHash := randomFakeHash()

	// Test successful process
	mockBackend := NewMockBackend()
	mockBackend.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackend.On("SendCmioResponse", mock.AnythingOfType("uint16"), mock.Anything, expectedHash, mock.AnythingOfType("time.Duration")).Return(nil)
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	mockBackend.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), make([]byte, 32), nil)

	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	input := []byte("test input")
	result, err := machine.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
	require.NoError(err)
	require.Equal(CompletionStatusAccepted, result.completion.status)
	require.Empty(result.outputs)
	require.Empty(result.reports)
	require.NotNil(result.completion.data)
	mockBackend.AssertExpectations(s.T())

	// A halt completes the request without producing a CMIO manual yield.
	haltedBackend := NewMockBackend()
	haltedBackend.On("CmioRxBufferSize").Return(uint64(1024))
	haltedBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(uint64(0), nil).Once()
	haltedBackend.On(
		"SendCmioResponse",
		mock.AnythingOfType("uint16"), mock.Anything, expectedHash,
		mock.AnythingOfType("time.Duration"),
	).Return(nil).Once()
	haltedBackend.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).
		Return(Halted, nil).Once()
	haltedBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(uint64(100), nil).Once()
	haltedMachine := &machineImpl{
		backend: haltedBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       5 * time.Second,
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second,
			AdvanceMaxDeadline: 10 * time.Second,
		},
	}
	result, err = haltedMachine.process(
		ctx, input, AdvanceStateRequest, &expectedHash, false,
	)
	require.NoError(err)
	require.Equal(CompletionStatusHalted, result.completion.status)
	require.Nil(result.completion.data)
	haltedBackend.AssertExpectations(s.T())

	// Test process with payload too large
	mockBackend2 := NewMockBackend()
	mockBackend2.On("CmioRxBufferSize").Return(uint64(5))
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, err = machine2.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
	require.ErrorIs(err, ErrPayloadLengthLimitExceeded)
	require.Contains(err.Error(), "advance request payload length 10")
	require.Contains(err.Error(), "capacity 5")
	require.NotContains(err.Error(), string(input))
	mockBackend2.AssertExpectations(s.T())

	// Test process with send error
	mockBackend3 := NewMockBackend()
	mockBackend3.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackend3.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	mockBackend3.On("SendCmioResponse",
		mock.AnythingOfType("uint16"),
		mock.Anything,
		expectedHash,
		mock.AnythingOfType("time.Duration"),
	).Return(errors.New("send failed"))
	machine3 := &machineImpl{
		backend: mockBackend3,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, err = machine3.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
	require.Error(err)
	require.Contains(err.Error(), "send failed")
	mockBackend3.AssertExpectations(s.T())

	// Test process with run error
	mockBackend4 := NewMockBackend()
	mockBackend4.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackend4.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), errors.New("read cycle failed"))
	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, err = machine4.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
	require.Error(err)
	require.Contains(err.Error(), "read cycle failed")
	mockBackend4.AssertNotCalled(s.T(), "SendCmioResponse", mock.Anything, mock.Anything, mock.Anything)
	mockBackend4.AssertExpectations(s.T())
}

// Test multiple automatic yields in sequence
func (s *ImplementationSuite) TestMultipleAutomaticYields() {
	require := s.Require()
	ctx := context.Background()

	mockBackend := NewMockBackend()
	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	// Setup for multiple automatic yields followed by manual yield
	// First automatic yield with output
	mockBackend.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(10), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonOutput), []byte("output1"), nil).Once()

	// Second automatic yield with output
	mockBackend.On("Run", uint64(110), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(20), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonOutput), []byte(""), nil).Once()

	// Third automatic yield with report
	mockBackend.On("Run", uint64(120), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(30), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonReport), []byte("output2"), nil).Once()

	// Fourth automatic yield with report
	mockBackend.On("Run", uint64(130), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(40), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonReport), []byte(""), nil).Once()

	// Fifth automatic yield with progress (ignored)
	mockBackend.On("Run", uint64(140), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(50), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonProgress), []byte("progress"), nil).Once()

	// Final manual yield
	mockBackend.On("Run", uint64(150), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(60), nil).Once()

	result, err := machine.run(
		ctx, AdvanceStateRequest, false, testExecutionBounds(0, model.MaxExecutionCycleSpan),
	)
	require.NoError(err)

	require.Len(result.outputs, 2)
	require.Equal([]byte("output1"), result.outputs[0])
	require.Equal([]byte(""), result.outputs[1])

	require.Len(result.reports, 2)
	require.Equal([]byte("output2"), result.reports[0])
	require.Equal([]byte(""), result.reports[1])

	mockBackend.AssertExpectations(s.T())
}

// Test checkContext function
func (s *ImplementationSuite) TestCheckContext() {
	require := s.Require()

	// Test normal context
	ctx := context.Background()
	err := checkContext(ctx)
	require.NoError(err)

	// Test canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = checkContext(canceledCtx)
	require.ErrorIs(err, ErrCanceled)

	// Test deadline exceeded context — use a deadline in the past so it's
	// already expired without relying on sleep timing.
	expiredCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	err = checkContext(expiredCtx)
	require.ErrorIs(err, ErrDeadlineExceeded)

	// Test nil context (should not panic)
	err = checkContext(nil) // nolint
	require.NoError(err)    // nil context is valid in Go
}

func (s *ImplementationSuite) TestInputHashCollectionSpan() {
	require := s.Require()

	for _, test := range []struct {
		name    string
		hashes  uint64
		padding uint64
		wantErr bool
	}{
		{name: "empty collection", hashes: 0, padding: InputEntryCapacity},
		{name: "partial collection", hashes: 2, padding: InputEntryCapacity - 2},
		{name: "exact boundary", hashes: InputEntryCapacity, padding: 0},
		{name: "short span", hashes: 2, padding: InputEntryCapacity - 3, wantErr: true},
		{name: "long span", hashes: 2, padding: InputEntryCapacity - 1, wantErr: true},
		{name: "collector overflow", hashes: InputEntryCapacity + 1, wantErr: true},
	} {
		s.Run(test.name, func() {
			err := ValidateInputHashCollectionSpan(test.hashes, test.padding)
			require.Equal(test.wantErr, err != nil)
		})
	}

	padding, err := inputHashCollectionPaddingRepetitions(InputEntryCapacity)
	require.NoError(err)
	require.Zero(padding)
	_, err = inputHashCollectionPaddingRepetitions(InputEntryCapacity + 1)
	require.ErrorContains(err, "exceeds input entry capacity")

	canonicalCount, padding, err := canonicalInputHashCollectionShape(
		InputEntryCapacity,
		true,
	)
	require.NoError(err)
	require.Equal(InputEntryCapacity-1, canonicalCount)
	require.Equal(uint64(1), padding)

	// Validate the raw collector count before removing the terminal sample.
	// Otherwise S+1 would be incorrectly normalized into a valid S shape.
	_, _, err = canonicalInputHashCollectionShape(InputEntryCapacity+1, true)
	require.ErrorContains(err, "exceeds input entry capacity")

	// A malformed collection is an internal failure even when execution also
	// reached a completed terminal state. The terminal status is context only.
	err = invalidInputHashCollectionSpanError(ErrHalted, errors.New("overcollection"))
	require.ErrorIs(err, ErrMachineInternal)
	require.NotErrorIs(err, ErrHalted)
	require.ErrorContains(err, ErrHalted.Error())
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"errors"
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
	require.Error(err)
	require.Contains(err.Error(), "machine manual yield reason is not accepted")
	mockBackend2.AssertExpectations(s.T())

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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	input := []byte("test input")
	resp, err := machine.Advance(ctx, input, expectedHash, false)
	require.NoError(err)
	require.True(resp.Accepted)
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machine2.Advance(ctx, input, expectedHash, false)
	require.NoError(err)
	require.False(resp.Accepted)
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machine3.Advance(ctx, input, expectedHash, false)
	require.ErrorIs(err, ErrException)
	require.False(resp.Accepted)
	require.Equal(Hash{}, resp.OutputsHash)
	mockBackend3.AssertExpectations(s.T())

	// Test advance with payload too large
	mockBackend4 := NewMockBackend()
	mockBackend4.On("CmioRxBufferSize").Return(uint64(5))
	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	largeInput := make([]byte, 10)
	_, err = machine4.Advance(ctx, largeInput, expectedHash, false)
	require.ErrorIs(err, ErrPayloadLengthLimitExceeded)
	mockBackend4.AssertExpectations(s.T())

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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, err = machine5.Advance(ctx, input, expectedHash, false)
	require.Error(err)
	require.ErrorIs(err, ErrHashLength)
	mockBackend5.AssertExpectations(s.T())
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
			InspectMaxCycles:   1000,
			InspectIncDeadline: time.Second * 1,
			InspectMaxDeadline: time.Second * 10,
		},
	}

	query := []byte("test query")
	accepted, reports, err := machine.Inspect(ctx, query)
	require.NoError(err)
	require.True(accepted)
	require.Empty(reports)
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
			InspectMaxCycles:   1000,
			InspectIncDeadline: time.Second * 1,
			InspectMaxDeadline: time.Second * 10,
		},
	}
	accepted, reports, err = machine2.Inspect(ctx, query)
	require.NoError(err)
	require.False(accepted)
	require.Empty(reports)
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
			InspectMaxCycles:   1000,
			InspectIncDeadline: time.Second * 1,
			InspectMaxDeadline: time.Second * 10,
		},
	}
	accepted, reports, err = machine3.Inspect(ctx, query)
	require.ErrorIs(err, ErrException)
	require.False(accepted)
	require.Empty(reports)
	mockBackend3.AssertExpectations(s.T())

	// Test inspect with payload too large
	mockBackend4 := NewMockBackend()
	mockBackend4.On("CmioRxBufferSize").Return(uint64(5))
	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			InspectIncCycles:   100,
			InspectMaxCycles:   1000,
			InspectIncDeadline: time.Second * 1,
			InspectMaxDeadline: time.Second * 10,
		},
	}
	largeQuery := make([]byte, 10)
	_, _, err = machine4.Inspect(ctx, largeQuery)
	require.ErrorIs(err, ErrPayloadLengthLimitExceeded)
	mockBackend4.AssertExpectations(s.T())
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

	// Test wasLastRequestAccepted
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
	accepted, data, err := machine3.wasLastRequestAccepted(ctx)
	require.NoError(err)
	require.True(accepted)
	require.NotNil(data)
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
	accepted, data, err = machine4.wasLastRequestAccepted(ctx)
	require.NoError(err)
	require.False(accepted)
	require.NotNil(data)
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
	accepted, data, err = machine5.wasLastRequestAccepted(ctx)
	require.ErrorIs(err, ErrException)
	require.False(accepted)
	require.NotNil(data)
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

	_, _, err = machine3.wasLastRequestAccepted(canceledCtx)
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	outputs, reports, _, _, err := machine.run(ctx, AdvanceStateRequest, false)
	require.NoError(err)
	require.Empty(outputs)
	require.Empty(reports)
	mockBackend.AssertExpectations(s.T())

	// Test run with read cycle error
	mockBackend2 := NewMockBackend()
	mockBackend2.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), errors.New("read cycle failed"))
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, _, _, _, err = machine2.run(ctx, AdvanceStateRequest, false)
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	_, _, _, _, err = machine3.run(ctx, AdvanceStateRequest, false)
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	_, _, _, _, err = machine4.run(ctx, AdvanceStateRequest, false)
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	outputs5, reports5, _, _, err := machine5.run(ctx, AdvanceStateRequest, false)
	require.NoError(err)
	require.Len(outputs5, 1)
	require.Equal([]byte("output data"), []byte(outputs5[0]))
	require.Empty(reports5)
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	outputs6, reports6, _, _, err := machine6.run(ctx, AdvanceStateRequest, false)
	require.NoError(err)
	require.Empty(outputs6)
	require.Len(reports6, 1)
	require.Equal([]byte("report data"), []byte(reports6[0]))
	mockBackend6.AssertExpectations(s.T())

}

// Test step method
func (s *ImplementationSuite) TestStep() {
	require := s.Require()
	ctx := context.Background()

	machine := &machineImpl{
		backend: nil, // Will be set per test
		logger:  s.logger,
		params: model.ExecutionParameters{
			AdvanceIncCycles: 100,
		},
	}

	// Test step with manual yield
	mockBackend := NewMockBackend()
	mockBackend.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(150), nil)
	machine.backend = mockBackend

	yieldType, cycle, err := machine.runIncrementInterval(ctx, 100, 1000, nil, time.Second)
	require.NoError(err)
	require.NotNil(yieldType)
	require.Equal(ManualYield, *yieldType)
	require.Equal(uint64(150), cycle)
	mockBackend.AssertExpectations(s.T())

	// Test runIncrementInterval with automatic yield
	mockBackend2 := NewMockBackend()
	mockBackend2.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil)
	mockBackend2.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(200), nil)
	machine.backend = mockBackend2

	yieldType, cycle, err = machine.runIncrementInterval(ctx, 100, 1000, nil, time.Second)
	require.NoError(err)
	require.NotNil(yieldType)
	require.Equal(AutomaticYield, *yieldType)
	require.Equal(uint64(200), cycle)
	mockBackend2.AssertExpectations(s.T())

	// Test runIncrementInterval with soft yield (no yield)
	mockBackend3 := NewMockBackend()
	mockBackend3.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedSoftly, nil)
	mockBackend3.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(150), nil)
	machine.backend = mockBackend3

	yieldType, cycle, err = machine.runIncrementInterval(ctx, 100, 1000, nil, time.Second)
	require.NoError(err)
	require.Nil(yieldType)
	require.Equal(uint64(150), cycle)
	mockBackend3.AssertExpectations(s.T())

	// Test runIncrementInterval with reached target mcycle
	mockBackend4 := NewMockBackend()
	mockBackend4.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(ReachedTargetMcycle, nil)
	mockBackend4.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(1000), nil)
	machine.backend = mockBackend4

	yieldType, cycle, err = machine.runIncrementInterval(ctx, 100, 1000, nil, time.Second)
	require.ErrorIs(err, ErrReachedTargetMcycle)
	require.Nil(yieldType)
	require.Equal(uint64(1000), cycle)
	mockBackend4.AssertExpectations(s.T())

	// Test runIncrementInterval with halted
	mockBackend5 := NewMockBackend()
	mockBackend5.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(Halted, nil)
	mockBackend5.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(500), nil)
	machine.backend = mockBackend5

	yieldType, cycle, err = machine.runIncrementInterval(ctx, 100, 1000, nil, time.Second)
	require.ErrorIs(err, ErrHalted)
	require.Nil(yieldType)
	require.Equal(uint64(500), cycle)

	// Test runIncrementInterval already at limit cycle
	yieldType, cycle, err = machine.runIncrementInterval(ctx, 1000, 1000, nil, time.Second)
	require.ErrorIs(err, ErrReachedLimitMcycle)
	require.Nil(yieldType)
	require.Equal(uint64(0), cycle)
	mockBackend5.AssertExpectations(s.T())

	// Test runIncrementInterval with backend run error
	mockBackend6 := NewMockBackend()
	mockBackend6.On("Run",
		mock.AnythingOfType("uint64"),
		mock.AnythingOfType("time.Duration"),
	).Return(BreakReason(0), errors.New("run failed"))
	machine.backend = mockBackend6
	yieldType, _, err = machine.runIncrementInterval(ctx, 100, 1000, nil, time.Second)
	require.Error(err)
	require.Contains(err.Error(), "run failed")
	require.Nil(yieldType)
	mockBackend6.AssertExpectations(s.T())

	// Test runIncrementInterval with read cycle error
	mockBackend7 := NewMockBackend()
	mockBackend7.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	mockBackend7.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), errors.New("read cycle failed"))
	machine.backend = mockBackend7
	yieldType, _, err = machine.runIncrementInterval(ctx, 100, 1000, nil, time.Second)
	require.Error(err)
	require.Contains(err.Error(), "read cycle failed")
	require.Nil(yieldType)
	mockBackend7.AssertExpectations(s.T())
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	input := []byte("test input")
	accepted, outputs, reports, _, _, data, err := machine.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
	require.NoError(err)
	require.True(accepted)
	require.Empty(outputs)
	require.Empty(reports)
	require.NotNil(data)
	mockBackend.AssertExpectations(s.T())

	// Test process with payload too large
	mockBackend2 := NewMockBackend()
	mockBackend2.On("CmioRxBufferSize").Return(uint64(5))
	machine2 := &machineImpl{
		backend: mockBackend2,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, _, _, _, _, _, err = machine2.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
	require.ErrorIs(err, ErrPayloadLengthLimitExceeded)
	mockBackend2.AssertExpectations(s.T())

	// Test process with send error
	mockBackend3 := NewMockBackend()
	mockBackend3.On("CmioRxBufferSize").Return(uint64(1024))
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, _, _, _, _, _, err = machine3.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
	require.Error(err)
	require.Contains(err.Error(), "send failed")
	mockBackend3.AssertExpectations(s.T())

	// Test process with run error
	mockBackend4 := NewMockBackend()
	mockBackend4.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackend4.On("SendCmioResponse", mock.AnythingOfType("uint16"), mock.Anything, expectedHash, mock.AnythingOfType("time.Duration")).Return(nil)
	mockBackend4.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), errors.New("read cycle failed"))
	machine4 := &machineImpl{
		backend: mockBackend4,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, _, _, _, _, _, err = machine4.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
	require.Error(err)
	require.Contains(err.Error(), "read cycle failed")
	mockBackend4.AssertExpectations(s.T())
}

// Test run method with automatic yields and outputs/reports
func (s *ImplementationSuite) TestRunWithAutomaticYields() {
	require := s.Require()
	ctx := context.Background()

	mockBackend := NewMockBackend()
	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	// Setup for automatic yield with output
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil).Once()
	mockBackend.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(50), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonOutput), []byte("test output"), nil).Once()

	// Setup for manual yield after automatic yield
	mockBackend.On("Run", uint64(150), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(100), nil).Once()

	outputs, reports, _, _, err := machine.run(ctx, AdvanceStateRequest, false)
	require.NoError(err)
	require.Len(outputs, 1)
	require.Equal([]byte("test output"), outputs[0])
	require.Empty(reports)

	mockBackend.AssertExpectations(s.T())
}

// Test run method with automatic yields and reports
func (s *ImplementationSuite) TestRunWithAutomaticYieldsReports() {
	require := s.Require()
	ctx := context.Background()

	mockBackend := NewMockBackend()
	machine := &machineImpl{
		backend: mockBackend,
		logger:  s.logger,
		params: model.ExecutionParameters{
			FastDeadline:       time.Second * 5,
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	// Setup for automatic yield with report
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil).Once()
	mockBackend.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(50), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonReport), []byte("test report"), nil).Once()

	// Setup for manual yield after automatic yield
	mockBackend.On("Run", uint64(150), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(100), nil).Once()

	outputs, reports, _, _, err := machine.run(ctx, AdvanceStateRequest, false)
	require.NoError(err)
	require.Empty(outputs)
	require.Len(reports, 1)
	require.Equal([]byte("test report"), reports[0])

	mockBackend.AssertExpectations(s.T())
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	// Setup for multiple automatic yields followed by manual yield
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil).Once()

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

	outputs, reports, _, _, err := machine.run(ctx, AdvanceStateRequest, false)
	require.NoError(err)

	require.Len(outputs, 2)
	require.Equal([]byte("output1"), outputs[0])
	require.Equal([]byte(""), outputs[1])

	require.Len(reports, 2)
	require.Equal([]byte("output2"), reports[0])
	require.Equal([]byte(""), reports[1])

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

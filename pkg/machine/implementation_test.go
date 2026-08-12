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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machine2.Advance(ctx, input, expectedHash, false)
	require.NoError(err)
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	resp, err = machine3.Advance(ctx, input, expectedHash, false)
	require.NoError(err)
	require.Equal(CompletionStatusException, resp.Status)
	require.Equal([]byte("exception data"), resp.ExceptionData)
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
	resp, err = machine4.Advance(ctx, largeInput, expectedHash, false)
	require.ErrorIs(err, ErrPayloadLengthLimitExceeded)
	require.Nil(resp)
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
	resp, err = machine5.Advance(ctx, input, expectedHash, false)
	require.Error(err)
	require.ErrorIs(err, ErrHashLength)
	require.Nil(resp)
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
			InspectMaxCycles:   1000,
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
			InspectMaxCycles:   1000,
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
	response, err = machine4.Inspect(ctx, largeQuery)
	require.NotNil(response)
	require.Equal(CompletionStatusUnknown, response.Status)
	require.ErrorIs(err, ErrPayloadLengthLimitExceeded)
	mockBackend4.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestRunUsesHardExecutionSpanWhenMaximumIsZero() {
	const startCycle uint64 = 7
	const executionCycleSpan uint64 = BarchSpanToInput

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

			machine := &machineImpl{backend: mockBackend, logger: s.logger, params: tt.params}
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
			backend.AssertNotCalled(s.T(), "ReadMCycle", mock.Anything)
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

func (s *ImplementationSuite) TestAdvanceCycleExhaustionSources() {
	for _, test := range []struct {
		name           string
		configuredSpan uint64
		breakReason    BreakReason
		wantSource     string
	}{
		{
			name: "configured endpoint", configuredSpan: 9,
			breakReason: ReachedTargetMcycle, wantSource: "configured",
		},
		{
			name: "fixed window", breakReason: McycleOverflow, wantSource: "fixed",
		},
	} {
		s.Run(test.name, func() {
			const start = uint64(30)
			span := test.configuredSpan
			if span == 0 {
				span = model.MaxExecutionCycleSpan
			}
			backend := NewMockBackend()
			backend.On("CmioRxBufferSize").Return(uint64(1024))
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start, nil).Once()
			backend.On(
				"SendCmioResponse",
				uint16(AdvanceStateRequest), []byte("input"), mock.Anything, mock.AnythingOfType("time.Duration"),
			).Return(nil)
			backend.On("Run", start+span, mock.AnythingOfType("time.Duration")).Return(test.breakReason, nil)
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(start+span, nil).Once()
			machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
				AdvanceIncCycles:   ^uint64(0),
				AdvanceMaxCycles:   test.configuredSpan,
				AdvanceIncDeadline: time.Second,
				AdvanceMaxDeadline: time.Second,
				FastDeadline:       time.Second,
			}}

			response, err := machine.Advance(context.Background(), []byte("input"), Hash{}, false)
			s.Require().Nil(response)
			s.Require().ErrorIs(err, ErrReachedLimitMcycle)
			if test.breakReason == McycleOverflow {
				s.Require().ErrorIs(err, ErrMcycleOverflow)
			}
			s.Contains(err.Error(), test.wantSource)
			s.Contains(err.Error(), "cycle limit")
			s.Contains(err.Error(), fmt.Sprintf("requested_span=%d", span))
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestConfiguredLimitTiePreservesMachineOverflowPrecedence() {
	const start = uint64(30)
	configuredSpan := model.MaxExecutionCycleSpan
	limit := start + configuredSpan
	backend := NewMockBackend()
	backend.On("Run", limit, mock.AnythingOfType("time.Duration")).Return(McycleOverflow, nil).Once()
	backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(limit, nil).Once()
	machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
		AdvanceIncCycles:   ^uint64(0),
		AdvanceIncDeadline: time.Second,
		AdvanceMaxDeadline: time.Second,
		AdvanceMaxCycles:   configuredSpan,
		FastDeadline:       time.Second,
	}}

	_, err := machine.run(context.Background(), AdvanceStateRequest, false, executionBounds{
		start: start, limit: limit, span: configuredSpan, configured: true,
	})
	s.Require().ErrorIs(err, ErrReachedLimitMcycle)
	s.Require().ErrorIs(err, ErrMcycleOverflow)
	s.Contains(err.Error(), "advance execution stopped at machine imcyclemax coincident with configured target")
	backend.AssertExpectations(s.T())
}

func (s *ImplementationSuite) TestExecutionStartingAtMachineMaximumPreservesOverflowOrigin() {
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
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(^uint64(0), nil).Once()
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
			name: "configured cap beyond representable range", start: ^uint64(0) - 10, configured: 11,
		},
		{
			name: "zero cap", start: ^uint64(0) - model.MaxExecutionCycleSpan + 1,
		},
		{
			name: "maximum cap", start: ^uint64(0) - model.MaxExecutionCycleSpan + 1,
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
	const startCycle uint64 = 7
	const inspectIncCycles uint64 = 137

	mockBackend := NewMockBackend()
	mockBackend.On("CmioRxBufferSize").Return(uint64(1024))
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(startCycle, nil).Once()
	mockBackend.On(
		"SendCmioResponse",
		uint16(InspectStateRequest), mock.Anything, mock.Anything, mock.AnythingOfType("time.Duration"),
	).Return(nil)
	mockBackend.On("Run", startCycle+inspectIncCycles, mock.AnythingOfType("time.Duration")).
		Return(YieldedManually, nil)
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).
		Return(startCycle+inspectIncCycles, nil).Once()
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
	s.Require().NoError(err)
	s.Require().Equal(CompletionStatusAccepted, response.Status)
	mockBackend.AssertExpectations(s.T())
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	result, err := machine.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, err = machine2.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
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

	_, err = machine3.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
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

	_, err = machine4.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
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

	result5, err := machine5.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}

	result6, err := machine6.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
	require.NoError(err)
	require.Empty(result6.outputs)
	require.Len(result6.reports, 1)
	require.Equal([]byte("report data"), []byte(result6.reports[0]))
	mockBackend6.AssertExpectations(s.T())

}

func (s *ImplementationSuite) TestRunIncrementIntervalPreservesBreakReason() {
	for _, test := range []struct {
		name        string
		breakReason BreakReason
		cycle       uint64
	}{
		{"manual", YieldedManually, 150},
		{"automatic", YieldedAutomatically, 200},
		{"soft", YieldedSoftly, 150},
		{"target", ReachedTargetMcycle, 200},
		{"overflow", McycleOverflow, 199},
		{"halt", Halted, 175},
		{"failed", Failed, 160},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("Run", uint64(200), mock.AnythingOfType("time.Duration")).Return(test.breakReason, nil)
			backend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(test.cycle, nil)
			machine := &machineImpl{backend: backend, logger: s.logger}

			result, err := machine.runIncrementInterval(
				context.Background(), 100, 1000, 100, nil, time.Second,
			)
			s.Require().NoError(err)
			s.Equal(test.breakReason, result.breakReason)
			s.Equal(test.cycle, result.currentCycle)
			backend.AssertExpectations(s.T())
		})
	}

	s.Run("already at limit", func() {
		machine := &machineImpl{backend: NewMockBackend(), logger: s.logger}
		result, err := machine.runIncrementInterval(
			context.Background(), 1000, 1000, 100, nil, time.Second,
		)
		s.Require().ErrorIs(err, ErrReachedLimitMcycle)
		s.Equal(uint64(1000), result.currentCycle)
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
			AdvanceMaxCycles:   1000,
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
			AdvanceMaxCycles:   1000,
			AdvanceIncCycles:   100,
			AdvanceIncDeadline: time.Second * 1,
			AdvanceMaxDeadline: time.Second * 10,
		},
	}
	_, err = machine2.process(ctx, input, AdvanceStateRequest, &expectedHash, false)
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
	mockBackend3.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
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
			AdvanceMaxCycles:   1000,
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
	mockBackend.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(50), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonOutput), []byte("test output"), nil).Once()

	// Setup for manual yield after automatic yield
	mockBackend.On("Run", uint64(150), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(100), nil).Once()

	result, err := machine.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
	require.NoError(err)
	require.Len(result.outputs, 1)
	require.Equal([]byte("test output"), result.outputs[0])
	require.Empty(result.reports)

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
	mockBackend.On("Run", uint64(100), mock.AnythingOfType("time.Duration")).Return(YieldedAutomatically, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(50), nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(AutomaticYieldReasonReport), []byte("test report"), nil).Once()

	// Setup for manual yield after automatic yield
	mockBackend.On("Run", uint64(150), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil).Once()
	mockBackend.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(100), nil).Once()

	result, err := machine.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
	require.NoError(err)
	require.Empty(result.outputs)
	require.Len(result.reports, 1)
	require.Equal([]byte("test report"), result.reports[0])

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

	result, err := machine.run(ctx, AdvanceStateRequest, false, executionBounds{
		start: 0, limit: 1000, span: 1000, configured: true,
	})
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

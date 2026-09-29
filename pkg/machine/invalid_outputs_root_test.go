// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/mock"
)

func (s *ImplementationSuite) TestAdvanceOutputsRootLength() {
	// The standard CMIO TX buffer holds 2^21 bytes. Test the declaration,
	// including lengths that ReceiveCmioRequest would reject before retrieval.
	for _, length := range []uint32{0, 31, HashSize, 33, 1 << 21, 1<<21 + 1, math.MaxUint32} {
		for _, collect := range []bool{false, true} {
			s.Run(fmt.Sprintf("length=%d/collect=%t", length, collect), func() {
				backend := NewMockBackend()
				backend.On("CmioRxBufferSize").Return(uint64(1024))
				backend.On("SendCmioResponse", uint16(AdvanceStateRequest), []byte("input"), Hash{}, mock.Anything).
					Return(nil).Once()
				backend.On("ReadMCycle", mock.Anything).Return(uint64(0), nil).Once()
				backend.On("ReadMCycle", mock.Anything).Return(uint64(1), nil).Once()
				periodic := Hash{0x11}
				if collect {
					backend.On("RunAndCollectRootHashes", mock.Anything, mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) {
							collector := args.Get(1).(*HashCollectorState)
							collector.Hashes = []Hash{periodic, {0x22}}
						}).Return(YieldedManually, nil).Once()
				} else {
					backend.On("Run", mock.Anything, mock.Anything).Return(YieldedManually, nil).Once()
				}
				backend.SetupManualYield(ManualYieldReasonAccepted, length)
				machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
					FastDeadline: time.Second, AdvanceIncCycles: 10,
					AdvanceIncDeadline: time.Second, AdvanceMaxDeadline: time.Second,
				}}

				result, err := machine.Advance(context.Background(), []byte("input"), Hash{}, collect)
				s.Require().NoError(err)
				s.Require().NotNil(result)
				want := CompletionStatusInvalidOutputsRoot
				if length == HashSize {
					want = CompletionStatusAccepted
				}
				s.Equal(want, result.Status)
				s.Nil(result.ExceptionData)
				if collect {
					s.Equal([]Hash{periodic}, result.PeriodicStateHashes)
					s.Equal(InputEntryCapacity-1, result.PaddingRepetitions)
				} else {
					s.Empty(result.PeriodicStateHashes)
					s.Zero(result.PaddingRepetitions)
				}
				backend.AssertNotCalled(s.T(), "ReceiveCmioRequest", mock.Anything)
				backend.AssertExpectations(s.T())
			})
		}
	}
}

func (s *ImplementationSuite) TestAdvanceOutputsRootReadErrorsStayOperational() {
	readErr := errors.New("machine memory read failed")
	for _, test := range []struct {
		name string
		data []byte
		err  error
	}{
		{name: "read failed", err: readErr},
		{name: "short read", data: make([]byte, 7)},
		{name: "long read", data: make([]byte, 9)},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).
				Return(test.data, test.err).Once()
			machine := &machineImpl{backend: backend, logger: s.logger}
			result, err := machine.readAdvanceYieldResult(context.Background())
			s.Require().ErrorIs(err, ErrMachineInternal)
			if test.err != nil {
				s.ErrorIs(err, readErr)
			}
			s.Equal(CompletionStatusUnknown, result.status)
			backend.AssertNotCalled(s.T(), "ReceiveCmioRequest", mock.Anything)
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestAdvanceRootRuleRequiresAcceptedManualYield() {
	for _, test := range []struct {
		name   string
		tohost uint64
	}{
		{"wrong device", uint64(3)<<htifDeviceShift | htifCommandManual<<htifCommandShift | htifReasonInputAccepted<<htifReasonShift},
		{"wrong command", htifDeviceYield<<htifDeviceShift | uint64(2)<<htifCommandShift | htifReasonInputAccepted<<htifReasonShift},
		{"other reason", htifDeviceYield<<htifDeviceShift | htifCommandManual<<htifCommandShift | uint64(2)<<htifReasonShift},
	} {
		s.Run(test.name, func() {
			backend := NewMockBackend()
			backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).
				Return(binary.LittleEndian.AppendUint64(nil, test.tohost), nil).Once()
			receiveErr := errors.New("not an accepted payload")
			backend.On("ReceiveCmioRequest", mock.Anything).
				Return(uint8(0), uint16(0), []byte(nil), receiveErr).Once()
			machine := &machineImpl{backend: backend, logger: s.logger}
			result, err := machine.readAdvanceYieldResult(context.Background())
			s.Require().ErrorIs(err, receiveErr)
			s.Equal(CompletionStatusUnknown, result.status)
			backend.AssertExpectations(s.T())
		})
	}
}

func (s *ImplementationSuite) TestInspectAcceptedPayloadDoesNotDeclareOutputsRoot() {
	backend := NewMockBackend()
	backend.On("CmioRxBufferSize").Return(uint64(1024))
	backend.On("SendCmioResponse", uint16(InspectStateRequest), []byte("query"), nil, mock.Anything).Return(nil).Once()
	backend.On("ReadMCycle", mock.Anything).Return(uint64(0), nil)
	backend.On("Run", mock.Anything, mock.Anything).Return(YieldedManually, nil).Once()
	backend.On("ReceiveCmioRequest", mock.Anything).
		Return(uint8(0), uint16(ManualYieldReasonAccepted), []byte("not a root"), nil).Once()
	machine := &machineImpl{backend: backend, logger: s.logger, params: model.ExecutionParameters{
		FastDeadline: time.Second, InspectIncCycles: 10,
		InspectIncDeadline: time.Second, InspectMaxDeadline: time.Second,
	}}
	result, err := machine.Inspect(context.Background(), []byte("query"))
	s.Require().NoError(err)
	s.Equal(CompletionStatusAccepted, result.Status)
	backend.AssertNotCalled(s.T(), "ReadMemory", mock.Anything, mock.Anything, mock.Anything)
	backend.AssertExpectations(s.T())
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"testing"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func automaticTohostTestValue(reason uint16, data uint32) uint64 {
	return emulator.HtifDeviceYield<<emulator.HtifDeviceShift |
		uint64(emulator.YieldAutomatic)<<emulator.HtifCommandShift | uint64(reason)<<emulator.HtifReasonShift | uint64(data)
}

func TestAutomaticYieldRequestSkipsUnknownPayload(t *testing.T) {
	for _, reason := range []uint16{0, 8, 127, math.MaxUint16} {
		for _, length := range []uint32{0, 7, 1 << 21, 1<<21 + 1, math.MaxUint32} {
			t.Run(fmt.Sprintf("reason=%d/length=%d", reason, length), func(t *testing.T) {
				backend := NewMockBackend()
				backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).
					Return(binary.LittleEndian.AppendUint64(nil, automaticTohostTestValue(reason, length)), nil).Once()
				machine := &machineImpl{backend: backend}
				gotReason, data, err := machine.readAutomaticYieldRequest()
				require.NoError(t, err)
				require.Equal(t, reason, gotReason)
				require.Nil(t, data)
				backend.AssertNumberOfCalls(t, "ReadMemory", 1)
				backend.AssertNotCalled(t, "ReceiveCmioRequest", mock.Anything)
				backend.AssertExpectations(t)
			})
		}
	}
}

func TestAutomaticYieldRequestReceivesKnownData(t *testing.T) {
	for _, reason := range []automaticYieldReason{AutomaticYieldReasonProgress, AutomaticYieldReasonOutput, AutomaticYieldReasonReport} {
		for _, value := range []uint32{0, 7, 1 << 21, 1<<21 + 1, math.MaxUint32} {
			t.Run(fmt.Sprintf("reason=%d/value=%d", reason, value), func(t *testing.T) {
				backend := NewMockBackend()
				backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).
					Return(binary.LittleEndian.AppendUint64(nil, automaticTohostTestValue(uint16(reason), value)), nil).Once()
				operations := 1
				var want []byte
				progress := reason == AutomaticYieldReasonProgress
				tooLarge := !progress && uint64(value) > txBufferSize
				switch {
				case progress:
					want = binary.LittleEndian.AppendUint32(nil, value)
				case !tooLarge:
					want = bytes.Repeat([]byte{0x7b}, int(value))
					if value != 0 {
						operations++
						backend.On("ReadMemory", TxBufferAddress, uint64(value), mock.Anything).Return(want, nil).Once()
					}
				}
				machine := &machineImpl{backend: backend}
				gotReason, data, err := machine.readAutomaticYieldRequest()
				require.Equal(t, uint16(reason), gotReason)
				if tooLarge {
					require.ErrorContains(t, err, fmt.Sprint(value))
					require.ErrorContains(t, err, fmt.Sprint(txBufferSize))
					require.Nil(t, data)
				} else {
					require.NoError(t, err)
					require.Equal(t, want, data)
				}
				backend.AssertNumberOfCalls(t, "ReadMemory", operations)
				backend.AssertNotCalled(t, "ReceiveCmioRequest", mock.Anything)
				backend.AssertExpectations(t)
			})
		}
	}
}

func TestAutomaticYieldRequestReceivesProgressLiteral(t *testing.T) {
	backend := NewMockBackend()
	backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).
		Return(binary.LittleEndian.AppendUint64(nil,
			automaticTohostTestValue(uint16(AutomaticYieldReasonProgress), 0x01020304)), nil).Once()
	machine := &machineImpl{backend: backend}
	reason, data, err := machine.readAutomaticYieldRequest()
	require.NoError(t, err)
	require.Equal(t, uint16(AutomaticYieldReasonProgress), reason)
	require.Equal(t, []byte{4, 3, 2, 1}, data)
	backend.AssertNumberOfCalls(t, "ReadMemory", 1)
	backend.AssertNotCalled(t, "ReceiveCmioRequest", mock.Anything)
	backend.AssertExpectations(t)
}

func TestAutomaticYieldRequestPreservesReadErrors(t *testing.T) {
	readErr := errors.New("memory read failed")
	internalReadErr := errors.Join(readErr, ErrMachineInternal)
	outputHeader := binary.LittleEndian.AppendUint64(nil,
		automaticTohostTestValue(uint16(AutomaticYieldReasonOutput), 7))
	for _, test := range []struct {
		name         string
		header       []byte
		headerErr    error
		payloadErr   error
		wantInternal bool
	}{
		{name: "header failure", headerErr: readErr},
		{name: "header internal failure", headerErr: internalReadErr, wantInternal: true},
		{name: "short header", header: make([]byte, 7), wantInternal: true},
		{name: "long header", header: make([]byte, 9), wantInternal: true},
		{name: "payload failure", header: outputHeader, payloadErr: readErr},
		{name: "payload internal failure", header: outputHeader, payloadErr: internalReadErr, wantInternal: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := NewMockBackend()
			backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).Return(test.header, test.headerErr).Once()
			if test.payloadErr != nil {
				backend.On("ReadMemory", TxBufferAddress, uint64(7), mock.Anything).Return(nil, test.payloadErr).Once()
			}
			machine := &machineImpl{backend: backend}
			_, data, err := machine.readAutomaticYieldRequest()
			require.Error(t, err)
			require.Nil(t, data)
			if test.headerErr != nil || test.payloadErr != nil {
				require.ErrorIs(t, err, readErr)
			}
			if test.wantInternal {
				require.ErrorIs(t, err, ErrMachineInternal)
			} else {
				require.NotErrorIs(t, err, ErrMachineInternal)
			}
			backend.AssertNotCalled(t, "ReceiveCmioRequest", mock.Anything)
			backend.AssertExpectations(t)
		})
	}
}

func TestAutomaticYieldReadFailurePreservesRequestErrorClass(t *testing.T) {
	readErr := errors.New("memory read failed")
	for _, request := range []requestType{AdvanceStateRequest, InspectStateRequest} {
		for _, test := range []struct {
			name         string
			header       []byte
			headerErr    error
			payloadErr   error
			wantInternal bool
		}{
			{name: "header I/O", headerErr: readErr},
			{name: "payload I/O", header: binary.LittleEndian.AppendUint64(nil,
				automaticTohostTestValue(uint16(AutomaticYieldReasonOutput), 7)), payloadErr: readErr},
			{name: "short header", header: make([]byte, 7), wantInternal: true},
			{name: "oversized output", header: binary.LittleEndian.AppendUint64(nil,
				automaticTohostTestValue(uint16(AutomaticYieldReasonOutput), uint32(txBufferSize+1)))},
			{name: "oversized report", header: binary.LittleEndian.AppendUint64(nil,
				automaticTohostTestValue(uint16(AutomaticYieldReasonReport), uint32(txBufferSize+1)))},
		} {
			t.Run(request.String()+"/"+test.name, func(t *testing.T) {
				backend := NewMockBackend()
				backend.On("CmioRxBufferSize").Return(uint64(1024))
				backend.On("ReadMCycle", mock.Anything).Return(uint64(0), nil).Once()
				backend.SetupRequestYield()
				backend.On("SendCmioResponse", uint16(request), []byte("request"), mock.Anything, mock.Anything).
					Return(nil).Once()
				backend.On("Run", mock.Anything, mock.Anything).Return(YieldedAutomatically, nil).Once()
				backend.On("ReadMCycle", mock.Anything).Return(uint64(1), nil).Once()
				backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).
					Return(test.header, test.headerErr).Once()
				if test.payloadErr != nil {
					backend.On("ReadMemory", TxBufferAddress, uint64(7), mock.Anything).Return(nil, test.payloadErr).Once()
				}
				machine := &machineImpl{backend: backend, params: DefaultConfig("").ExecutionParameters,
					logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
				var err error
				if request == AdvanceStateRequest {
					var got *AdvanceResponse
					got, err = machine.Advance(t.Context(), []byte("request"), Hash{}, false)
					require.Nil(t, got)
				} else {
					var got *InspectResponse
					got, err = machine.Inspect(t.Context(), []byte("request"))
					require.NotNil(t, got)
					require.Equal(t, CompletionStatusUnknown, got.Status)
				}
				require.Error(t, err)
				if test.headerErr != nil || test.payloadErr != nil {
					require.ErrorIs(t, err, readErr)
				}
				if test.wantInternal {
					require.ErrorIs(t, err, ErrMachineInternal)
				} else {
					require.NotErrorIs(t, err, ErrMachineInternal)
				}
				require.NotErrorIs(t, err, ErrDeadlineExceeded)
				require.False(t, IsExecutionLimitError(err))
				backend.AssertExpectations(t)
			})
		}
	}
}

func TestReadTohostClassifiesReadFailureAsInternal(t *testing.T) {
	readErr := errors.New("memory read failed")
	backend := NewMockBackend()
	backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).Return(nil, readErr).Once()
	machine := &machineImpl{backend: backend}
	_, err := machine.readTohost()
	require.ErrorIs(t, err, readErr)
	require.ErrorIs(t, err, ErrMachineInternal)
	backend.AssertExpectations(t)
}

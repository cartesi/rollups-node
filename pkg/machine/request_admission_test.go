// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRequestAdmissionRefusesUnsupportedBoundary(t *testing.T) {
	// These are HTIF fields from emulator 0.22's send_cmio_response input boundary.
	accepted := emulator.HtifDeviceYield<<emulator.HtifDeviceShift |
		uint64(emulator.YieldManual)<<emulator.HtifCommandShift |
		uint64(emulator.ManualYieldReasonAccepted)<<emulator.HtifReasonShift
	rejected := emulator.HtifDeviceYield<<emulator.HtifDeviceShift |
		uint64(emulator.YieldManual)<<emulator.HtifCommandShift |
		uint64(emulator.ManualYieldReasonRejected)<<emulator.HtifReasonShift
	exception := emulator.HtifDeviceYield<<emulator.HtifDeviceShift |
		uint64(emulator.YieldManual)<<emulator.HtifCommandShift |
		uint64(emulator.ManualYieldReasonException)<<emulator.HtifReasonShift
	automatic := emulator.HtifDeviceYield<<emulator.HtifDeviceShift |
		uint64(emulator.YieldAutomatic)<<emulator.HtifCommandShift |
		uint64(emulator.ManualYieldReasonAccepted)<<emulator.HtifReasonShift
	readErr := errors.New("register read failed")
	for _, reqType := range []requestType{AdvanceStateRequest, InspectStateRequest} {
		for _, test := range []struct {
			name      string
			atYield   bool
			tohost    uint64
			yieldErr  error
			memoryErr error
			readSize  int
			refused   bool
		}{
			{name: "rejected yield", atYield: true, tohost: rejected, readSize: 8, refused: true},
			{name: "exception yield", atYield: true, tohost: exception, readSize: 8, refused: true},
			{name: "automatic yield", atYield: true, tohost: automatic, readSize: 8, refused: true},
			{name: "wrong device", atYield: true, tohost: accepted ^ (emulator.HtifDeviceYield^3)<<emulator.HtifDeviceShift,
				readSize: 8, refused: true},
			{name: "manual flag clear", tohost: accepted, refused: true},
			{name: "manual flag read error", yieldErr: readErr},
			{name: "HTIF read error", atYield: true, memoryErr: readErr},
			{name: "short HTIF read", atYield: true, readSize: 7},
			{name: "long HTIF read", atYield: true, readSize: 9},
		} {
			t.Run(reqType.String()+"/"+test.name, func(t *testing.T) {
				backend := NewMockBackend()
				backend.On("CmioRxBufferSize").Return(uint64(1024)).Once()
				backend.On("ReadMCycle", mock.Anything).Return(uint64(0), nil).Once()
				backend.On("IsAtManualYield", mock.Anything).Return(test.atYield, test.yieldErr).Once()
				if test.atYield && test.yieldErr == nil {
					data := make([]byte, test.readSize)
					if len(data) >= 8 {
						binary.LittleEndian.PutUint64(data, test.tohost)
					}
					backend.On("ReadMemory", htifTohostAddress, uint64(8), mock.Anything).
						Return(data, test.memoryErr).Once()
				}
				machine := &machineImpl{backend: backend, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
					params: model.ExecutionParameters{FastDeadline: time.Second, AdvanceIncCycles: 10, InspectIncCycles: 10}}
				var err error
				if reqType == AdvanceStateRequest {
					var response *AdvanceResponse
					response, err = machine.Advance(t.Context(), []byte("input"), Hash{}, false)
					require.Nil(t, response)
				} else {
					_, err = machine.Inspect(t.Context(), []byte("query"))
				}
				require.ErrorIs(t, err, ErrMachineInternal)
				if test.refused {
					require.ErrorIs(t, err, ErrNotAtManualYield)
				}
				if test.yieldErr != nil || test.memoryErr != nil {
					require.ErrorIs(t, err, readErr)
				}
				backend.AssertNotCalled(t, "SendCmioResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				backend.AssertNotCalled(t, "Run", mock.Anything, mock.Anything)
				backend.AssertExpectations(t)
			})
		}
	}
}

type admissionCountingBackend struct {
	Backend
	sends int
}

func (b *admissionCountingBackend) SendCmioResponse(reason uint16, data []byte, checkpoint *Hash, timeout time.Duration) error {
	b.sends++
	return b.Backend.SendCmioResponse(reason, data, checkpoint, timeout)
}

func TestRejectedYieldRefusesAdvanceAndInspectWithoutMutation(t *testing.T) {
	machine := loadEndpointGuest(t, 0, emulator.ManualYieldReasonRejected, false)
	impl := machine.(*machineImpl)
	backend := &admissionCountingBackend{Backend: impl.backend}
	impl.backend = backend
	checkpoint, err := machine.Hash(t.Context())
	require.NoError(t, err)
	response, err := machine.Advance(t.Context(), []byte("first input"), checkpoint, false)
	require.NoError(t, err)
	require.Equal(t, CompletionStatusRejected, response.Status)
	require.Equal(t, 1, backend.sends)
	boundary, err := machine.Hash(t.Context())
	require.NoError(t, err)
	backend.sends = 0

	t.Run("advance", func(t *testing.T) {
		_, err := machine.Advance(t.Context(), []byte("second input"), boundary, false)
		require.ErrorIs(t, err, ErrNotAtManualYield)
		require.ErrorIs(t, err, ErrMachineInternal)
		require.Zero(t, backend.sends)
		after, err := machine.Hash(t.Context())
		require.NoError(t, err)
		require.Equal(t, boundary, after)
	})
	t.Run("inspect", func(t *testing.T) {
		_, err := machine.Inspect(t.Context(), []byte("query"))
		require.ErrorIs(t, err, ErrNotAtManualYield)
		require.ErrorIs(t, err, ErrMachineInternal)
		require.Zero(t, backend.sends)
		after, err := machine.Hash(t.Context())
		require.NoError(t, err)
		require.Equal(t, boundary, after)
	})
}

func TestHaltedAcceptedYieldStillDeliversRequest(t *testing.T) {
	for _, reqType := range []requestType{AdvanceStateRequest, InspectStateRequest} {
		t.Run(reqType.String(), func(t *testing.T) {
			machine := loadEndpointGuest(t, 0, emulator.ManualYieldReasonAccepted, true)
			impl := machine.(*machineImpl)
			backend := &admissionCountingBackend{Backend: impl.backend}
			impl.backend = backend
			checkpoint, err := machine.Hash(t.Context())
			require.NoError(t, err)
			if reqType == AdvanceStateRequest {
				response, err := machine.Advance(t.Context(), []byte("input"), checkpoint, false)
				require.NoError(t, err)
				require.Equal(t, CompletionStatusHalted, response.Status)
			} else {
				response, err := machine.Inspect(t.Context(), []byte("query"))
				require.NoError(t, err)
				require.Equal(t, CompletionStatusHalted, response.Status)
			}
			require.Equal(t, 1, backend.sends)
			after, err := machine.Hash(t.Context())
			require.NoError(t, err)
			require.NotEqual(t, checkpoint, after)
		})
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

// RemoteMachineInterface defines the interface that LibCartesiBackend needs from a remote machine
type RemoteMachineInterface interface {
	SetTimeout(timeoutMs int64) error
	Load(dir string, runtimeConfig string) error
	Run(mcycleEnd uint64) (emulator.BreakReason, error)
	GetRootHash() ([]byte, error)
	ReadReg(reg emulator.RegID) (uint64, error)
	SendCmioResponse(reason uint16, data []byte) error
	ReceiveCmioRequest() (uint8, uint16, []byte, error)
	Store(directory string) error
	Delete()
	ForkServer() (*emulator.RemoteMachine, string, uint32, error)
	ShutdownServer() error
}

func NewLibCartesiBackend(address string, timeout time.Duration) (Backend, string, uint32, error) {
	rm, address, pid, err := emulator.SpawnServer(address, timeout)
	if err != nil {
		return nil, address, pid, err
	}
	return &LibCartesiBackend{inner: rm}, address, pid, nil
}

// LibCartesiBackend is an adapter that implements Backend by wrapping a RemoteMachineInterface.
type LibCartesiBackend struct {
	inner RemoteMachineInterface
}

func (e *LibCartesiBackend) Load(dir string, runtimeConfig string, timeout time.Duration) error {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.Load(dir, runtimeConfig)
}

func (e *LibCartesiBackend) Run(mcycleEnd uint64, timeout time.Duration) (BreakReason, error) {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return Failed, fmt.Errorf("failed to set operation timeout: %w", err)
	}
	br, err := e.inner.Run(mcycleEnd)
	return BreakReason(br), err
}

func (e *LibCartesiBackend) GetRootHash(timeout time.Duration) ([]byte, error) {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return nil, fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.GetRootHash()
}

func (e *LibCartesiBackend) IsAtManualYield(timeout time.Duration) (bool, error) {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return false, fmt.Errorf("failed to set operation timeout: %w", err)
	}
	iflagsY, err := e.inner.ReadReg(emulator.REG_IFLAGS_Y)
	if err != nil {
		return false, err
	}
	return iflagsY == uint64(emulator.ManualYieldReasonAccepted), nil
}

func (e *LibCartesiBackend) ReadMCycle(timeout time.Duration) (uint64, error) {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return 0, fmt.Errorf("failed to set operation timeout: %w", err)
	}
	cycle, err := e.inner.ReadReg(emulator.REG_MCYCLE)
	if err != nil {
		return cycle, err
	}
	return cycle, nil
}

func (e *LibCartesiBackend) SendCmioResponse(reason uint16, data []byte, timeout time.Duration) error {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.SendCmioResponse(reason, data)
}

func (e *LibCartesiBackend) ReceiveCmioRequest(timeout time.Duration) (uint8, uint16, []byte, error) {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return 0, 0, nil, fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.ReceiveCmioRequest()
}

func (e *LibCartesiBackend) Store(directory string, timeout time.Duration) error {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.Store(directory)
}

func (e *LibCartesiBackend) Delete() {
	e.inner.Delete()
}

func (e *LibCartesiBackend) ForkServer(timeout time.Duration) (Backend, string, uint32, error) {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return nil, "", 0, fmt.Errorf("failed to set operation timeout: %w", err)
	}
	rm, s, u, err := e.inner.ForkServer()
	if err != nil {
		return nil, s, u, err
	}
	return &LibCartesiBackend{inner: rm}, s, u, nil
}

func (e *LibCartesiBackend) ShutdownServer(timeout time.Duration) error {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.ShutdownServer()
}

func (e *LibCartesiBackend) NewMachineRuntimeConfig() (string, error) {
	// Convert runtime options to JSON
	jsonConf, err := json.Marshal(emulator.NewMachineRuntimeConfig())
	if err != nil {
		return "", fmt.Errorf("could not marshal machine runtime config: %w", err)
	}
	return string(jsonConf), nil
}

func (e *LibCartesiBackend) CmioRxBufferSize() uint64 {
	return 1 << emulator.CmioRxBufferLog2Size
}

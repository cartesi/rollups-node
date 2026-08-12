// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/json"
	"time"
)

type BreakReason int32

const (
	Failed               BreakReason = 0x0
	Halted               BreakReason = 0x1
	YieldedManually      BreakReason = 0x2
	YieldedAutomatically BreakReason = 0x3
	YieldedSoftly        BreakReason = 0x4
	ReachedTargetMcycle  BreakReason = 0x5
	McycleOverflow       BreakReason = 0x8
)

type HashCollectorState struct {
	MCycleSamplingPeriod  uint64
	MCyclePhase           uint64
	Log2BundleMCycleCount int32
	Hashes                []Hash
	PartialBundle         json.RawMessage
	ConsoleIOError        string
}

// This Backend interface covers the methods used from the emulator / remote machine server.
// It is to abstract the emulator package and allow for easier testing and mocking in unit tests.
type Backend interface {
	Load(dir string, runtimeConfig string, timeout time.Duration) error
	Store(directory string, timeout time.Duration) error

	Run(mcycleEnd uint64, timeout time.Duration) (BreakReason, error)
	// RunAndCollectRootHashes may advance the backend before returning an error.
	// Callers must discard the backend after an error instead of retrying it with
	// the previous HashCollectorState.
	RunAndCollectRootHashes(mcycleEnd uint64, state *HashCollectorState, timeout time.Duration,
	) (reason BreakReason, err error)

	IsAtManualYield(timeout time.Duration) (bool, error)
	ReadMCycle(timeout time.Duration) (uint64, error)

	SendCmioResponse(reason uint16, data []byte, revertRootHash *Hash, timeout time.Duration) error
	ReceiveCmioRequest(timeout time.Duration) (cmd uint8, reason uint16, data []byte, err error)

	WriteMemory(address uint64, data []byte, timeout time.Duration) error

	GetRootHash(timeout time.Duration) (Hash, error)
	GetProof(address uint64, log2size int32, timeout time.Duration) ([]Hash, error)

	Delete()
	ForkServer(timeout time.Duration) (Backend, string, uint32, error)
	ShutdownServer(timeout time.Duration) error

	NewMachineRuntimeConfig() (string, error)
	CmioRxBufferSize() uint64
}

// BackendFactory is a function type that creates a new server instance.
// It takes an address and timeout, and returns a machine Backend, bound address, process ID, and error.
type BackendFactory func(address string, timeout time.Duration) (Backend, string, uint32, error)

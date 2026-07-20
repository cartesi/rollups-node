// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

// RemoteMachineInterface defines the interface that LibCartesiBackend needs from a remote machine
type RemoteMachineInterface interface {
	SetTimeout(timeoutMs int64) error
	Load(dir string, runtimeConfig string) error
	Run(mcycleEnd uint64) (emulator.BreakReason, error)
	GetRootHash() (emulator.Hash, error)
	GetProof(address uint64, log2size int32) (string, error)
	ReadReg(reg emulator.RegID) (uint64, error)
	SendCmioResponse(reason uint16, data []byte, revertRootHash *emulator.Hash) error
	ReceiveCmioRequest() (uint8, uint16, []byte, error)
	WriteMemory(address uint64, data []byte) error
	Store(directory string) error
	Delete()
	ForkServer() (*emulator.RemoteMachine, string, uint32, error)
	ShutdownServer() error
}

type proofJson struct {
	Log2RootSize   int32  `json:"log2_root_size"`
	Log2TargetSize int32  `json:"log2_target_size"`
	RootHash       Hash   `json:"root_hash"`
	Siblings       []Hash `json:"sibling_hashes"`
	TargetAddress  uint64 `json:"target_address"`
	TargetHash     Hash   `json:"target_hash"`
}

func decodeB64To32(dst *Hash, s string) error {
	// accepts Std (with '=') and Raw (without '=')
	n, err := base64.StdEncoding.Decode(dst[:], []byte(s))
	if err == nil {
		if n != HashSize {
			return fmt.Errorf("provided hash base64 size is %d bytes (expected %d)", n, HashSize)
		}
		return nil
	}

	// fallback RawStdEncoding
	n, err2 := base64.RawStdEncoding.Decode(dst[:], []byte(s))
	if err2 != nil {
		return fmt.Errorf("invalid hash base64 (std: %v, raw: %w)", err, err2)
	}
	if n != HashSize {
		return fmt.Errorf("provided hash base64 size is %d bytes (expected %d)", n, HashSize)
	}
	return nil
}

func (p *proofJson) UnmarshalJSON(data []byte) error {
	var aux struct {
		Log2RootSize   int32    `json:"log2_root_size"`
		Log2TargetSize int32    `json:"log2_target_size"`
		RootHash       string   `json:"root_hash"`
		Siblings       []string `json:"sibling_hashes"`
		TargetAddress  uint64   `json:"target_address"`
		TargetHash     string   `json:"target_hash"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	p.Log2RootSize = aux.Log2RootSize
	p.Log2TargetSize = aux.Log2TargetSize
	p.TargetAddress = aux.TargetAddress

	if err := decodeB64To32(&p.RootHash, aux.RootHash); err != nil {
		return fmt.Errorf("root_hash: %w", err)
	}
	if err := decodeB64To32(&p.TargetHash, aux.TargetHash); err != nil {
		return fmt.Errorf("target_hash: %w", err)
	}

	p.Siblings = make([]Hash, len(aux.Siblings))
	for i, s := range aux.Siblings {
		if err := decodeB64To32(&p.Siblings[i], s); err != nil {
			return fmt.Errorf("sibling_hashes[%d]: %w", i, err)
		}
	}
	return nil
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

func (e *LibCartesiBackend) GetRootHash(timeout time.Duration) (Hash, error) {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return Hash{}, fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.GetRootHash()
}

func (e *LibCartesiBackend) GetProof(address uint64, log2size int32, timeout time.Duration) ([]Hash, error) {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return nil, fmt.Errorf("failed to set operation timeout: %w", err)
	}
	jsonMessage, err := e.inner.GetProof(address, log2size)
	if err != nil {
		return nil, fmt.Errorf("failed to get proof: %w", err)
	}
	proof := &proofJson{}
	err = json.Unmarshal([]byte(jsonMessage), proof)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal proof JSON: %w", err)
	}
	return proof.Siblings, nil
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

func (e *LibCartesiBackend) SendCmioResponse(reason uint16, data []byte, revertRootHash *Hash, timeout time.Duration) error {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.SendCmioResponse(reason, data, revertRootHash)
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

func (e *LibCartesiBackend) WriteMemory(address uint64, data []byte, timeout time.Duration) error {
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return fmt.Errorf("failed to set operation timeout: %w", err)
	}
	return e.inner.WriteMemory(address, data)
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

func (e *LibCartesiBackend) RunAndCollectRootHashes(
	mcycleEnd uint64,
	state *HashCollectorState,
	timeout time.Duration,
) (reason BreakReason, err error) {
	if state == nil {
		return Failed, errors.New("nil state")
	}
	if state.Period == 0 {
		return Failed, errors.New("State.Period must be > 0")
	}

	// Set up timeout management: calculate absolute deadline if timeout is specified
	var deadline time.Time
	hasDeadline := timeout > 0
	if hasDeadline {
		deadline = time.Now().Add(timeout)
	}
	remaining := func() time.Duration {
		if !hasDeadline {
			return 0
		}
		d := time.Until(deadline)
		if d <= 0 {
			return time.Nanosecond
		}
		return d
	}
	checkDeadline := func() error {
		if hasDeadline && time.Now().After(deadline) {
			return errors.New("runWithRootHashes: deadline exceeded")
		}
		return nil
	}

	if err := checkDeadline(); err != nil {
		return Failed, err
	}
	cur, err := e.ReadMCycle(remaining())
	if err != nil {
		return Failed, err
	}

	collected := (uint64)(0)

	for {
		if err := checkDeadline(); err != nil {
			return Failed, err
		}
		if cur >= mcycleEnd {
			// No more cycles to execute
			return ReachedTargetMcycle, nil
		}

		// Calculate the next collection point: distance to the next multiple of the period
		// This ensures we collect hashes at regular intervals aligned with the period
		var step uint64
		if r := state.Phase % state.Period; r == 0 {
			step = state.Period
		} else {
			step = state.Period - r
		}

		nextHashCycle := cur + step
		target := min(nextHashCycle, mcycleEnd)

		// Run the machine until target cycle or until it yields/halts
		br, err := e.Run(target, remaining())
		if err != nil {
			return Failed, err
		}

		// Check where we stopped after the run
		if err := checkDeadline(); err != nil {
			return Failed, err
		}
		pos, err := e.ReadMCycle(remaining())
		if err != nil {
			return Failed, err
		}

		advanced := pos - cur
		state.Phase = (state.Phase + advanced) % state.Period
		cur = pos

		// Only collect hash if we reached the exact boundary (pos == nextHashCycle)
		// This ensures "hash after each complete period", matching the C API behavior
		// and avoiding duplicate collections if the machine stops early due to yields
		if pos == nextHashCycle {
			if err := checkDeadline(); err != nil {
				return Failed, err
			}
			h, err := e.GetRootHash(remaining())
			if err != nil {
				return Failed, err
			}

			state.Hashes = append(state.Hashes, h)

			collected++
			if state.MaxHashes > 0 && collected >= state.MaxHashes {
				return YieldedSoftly, nil
			}
		}

		switch br {
		case ReachedTargetMcycle:
			if cur >= mcycleEnd {
				return ReachedTargetMcycle, nil
			}
		case YieldedManually:
			return br, nil
		case YieldedAutomatically, YieldedSoftly, Halted:
			return br, nil
		case Failed:
			return Failed, errors.New("run failed")
		default:
			return Failed, errors.New("unknown break reason")
		}
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
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
	CollectMCycleRootHashes(
		mcycleEnd,
		log2McyclePeriod,
		mcyclePhase uint64,
		log2BundleMcycleCount int32,
		previousPartialBundle json.RawMessage,
	) ([]byte, error)
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
	// Keep this defensive check even though the advancer validates the constants
	// at startup. Other callers can construct a backend directly.
	if err := ValidateEmulatorComputationHashLimits(); err != nil {
		return nil, "", 0, err
	}
	rm, address, pid, err := emulator.SpawnServer(address, timeout)
	if err != nil {
		return nil, address, pid, err
	}
	return &LibCartesiBackend{inner: rm}, address, pid, nil
}

// ValidateEmulatorComputationHashLimits verifies that the node and compiled
// emulator agree on the three exported rollup limits checked here. A mismatch
// would change per-input or per-epoch computation-hash dimensions. This does
// not validate the selected sampling period, bundle exponent, runtime library,
// or hash algorithm.
func ValidateEmulatorComputationHashLimits() error {
	checks := []struct {
		name     string
		node     uint64
		emulator uint64
	}{
		{
			name:     "LOG2_MAX_UARCH_CYCLES_PER_MCYCLE",
			node:     Log2MaxUarchCyclesPerMCycle,
			emulator: emulator.Log2MaxUarchCyclesPerMCycle,
		},
		{
			name:     "LOG2_MAX_MCYCLES_PER_ADVANCE_STATE",
			node:     Log2MaxMCyclesPerAdvanceState,
			emulator: emulator.Log2MaxMCyclesPerAdvanceState,
		},
		{
			name:     "LOG2_MAX_ADVANCE_STATES_PER_EPOCH",
			node:     Log2MaxAdvanceStatesPerEpoch,
			emulator: emulator.Log2MaxAdvanceStatesPerEpoch,
		},
	}
	for _, check := range checks {
		if err := validateEmulatorComputationHashLimit(check.name, check.node, check.emulator); err != nil {
			return err
		}
	}
	return nil
}

// validateEmulatorComputationHashLimit takes explicit values so the mismatch
// path can be tested without requiring a differently compiled emulator library.
func validateEmulatorComputationHashLimit(name string, nodeLog2, emulatorLog2 uint64) error {
	if nodeLog2 == emulatorLog2 {
		return nil
	}
	return fmt.Errorf(
		"node computation-hash dimension 2^%d does not match emulator CM_ROLLUP_%s=%d: %w",
		nodeLog2, name, emulatorLog2, ErrMachineInternal,
	)
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

func decodeBreakReason(s string) (BreakReason, error) {
	switch s {
	case "yielded_automatically":
		return YieldedAutomatically, nil
	case "yielded_manually":
		return YieldedManually, nil
	case "yielded_softly":
		return YieldedSoftly, nil
	case "reached_target_mcycle":
		return ReachedTargetMcycle, nil
	case "mcycle_overflow":
		return McycleOverflow, nil
	case "halted":
		return Halted, nil
	case "failed":
		return Failed, nil
	default:
		return Failed, fmt.Errorf("unknown break reason %q", s)
	}
}

func (e *LibCartesiBackend) RunAndCollectRootHashes(
	mcycleEnd uint64,
	state *HashCollectorState,
	timeout time.Duration,
) (reason BreakReason, err error) {

	if state == nil {
		return Failed, errors.New("nil state")
	}
	if state.MCycleSamplingPeriod == 0 {
		return Failed, errors.New("state period must be greater than zero")
	}
	// Safe: the nonzero uint64 input makes bits.Len64 return a value in [1, 64].
	log2Period := uint64(bits.Len64(state.MCycleSamplingPeriod) - 1) //nolint:gosec
	if uint64(1)<<log2Period != state.MCycleSamplingPeriod {
		return Failed, fmt.Errorf("period must be a power of 2, got %v", state.MCycleSamplingPeriod)
	}
	if state.MCyclePhase >= state.MCycleSamplingPeriod {
		return Failed, fmt.Errorf(
			"phase must be less than period, got phase %v and period %v",
			state.MCyclePhase,
			state.MCycleSamplingPeriod,
		)
	}
	if err := e.inner.SetTimeout(timeout.Milliseconds()); err != nil {
		return Failed, fmt.Errorf("failed to set operation timeout: %w", err)
	}

	rawResult, err := e.inner.CollectMCycleRootHashes(
		mcycleEnd,
		log2Period,
		state.MCyclePhase,
		state.Log2BundleMCycleCount,
		state.PartialBundle,
	)
	if err != nil {
		return Failed, err
	}
	result := struct {
		RootHashes     []string        `json:"hashes"`
		MCyclePhase    uint64          `json:"mcycle_phase"`
		BreakReason    string          `json:"break_reason"`
		PartialBundle  json.RawMessage `json:"partial_bundle,omitempty"`
		ConsoleIOError string          `json:"console_io_error,omitempty"`
	}{}
	err = json.Unmarshal(rawResult, &result)
	if err != nil {
		return Failed, fmt.Errorf("failed to unmarshal CollectMCycleRootHashes result: %w", err)
	}
	reason, err = decodeBreakReason(result.BreakReason)
	if err != nil {
		return Failed, fmt.Errorf("invalid CollectMCycleRootHashes result: %w", err)
	}
	if result.MCyclePhase >= state.MCycleSamplingPeriod {
		return Failed, fmt.Errorf(
			"invalid CollectMCycleRootHashes result: phase %v must be less than period %v",
			result.MCyclePhase,
			state.MCycleSamplingPeriod,
		)
	}

	decodedHashes := make([]Hash, len(result.RootHashes))
	for i, base64Hash := range result.RootHashes {
		if err := decodeB64To32(&decodedHashes[i], base64Hash); err != nil {
			return Failed, fmt.Errorf("invalid collected hash at index %v: %w", i, err)
		}
	}

	state.Hashes = append(state.Hashes, decodedHashes...)
	state.MCyclePhase = result.MCyclePhase
	state.PartialBundle = result.PartialBundle
	state.ConsoleIOError = result.ConsoleIOError

	return reason, nil
}

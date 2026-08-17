// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
)

const AddressSize = 20
const HashSize = 32

// Common type aliases
type (
	Cycle   = uint64
	Output  = []byte
	Report  = []byte
	Address = [AddressSize]byte
	Hash    = [HashSize]byte
)

// CompletionStatus identifies how a guest-machine request completed. Advance
// and Inspect share the same outcomes; their callers decide whether and how a
// completed outcome affects canonical state. If execution does not complete,
// the operation returns CompletionStatusUnknown with an error.
type CompletionStatus uint8

const (
	// CompletionStatusUnknown is the zero-value sentinel. A successful request
	// never returns it.
	CompletionStatusUnknown CompletionStatus = iota
	CompletionStatusAccepted
	CompletionStatusRejected
	CompletionStatusException
	CompletionStatusHalted
	CompletionStatusOverflow
	CompletionStatusUnexpectedYield
)

// IsCompleted reports whether the status is a completed guest-machine outcome.
func (s CompletionStatus) IsCompleted() bool {
	switch s {
	case CompletionStatusAccepted,
		CompletionStatusRejected,
		CompletionStatusException,
		CompletionStatusHalted,
		CompletionStatusOverflow,
		CompletionStatusUnexpectedYield:
		return true
	case CompletionStatusUnknown:
		return false
	default:
		return false
	}
}

// LeafProof proves a 32-byte data block at a known machine-memory address.
// Its shape matches the proof consumed by the released v3 contracts.
type LeafProof struct {
	DataBlock Hash
	Siblings  []Hash
}

// StateProof binds the three state leaves used by the released v3 contracts to
// one machine root. The proof is intentionally outcome-neutral: accepted and
// terminal post-run states have the same Merkle shape, while callers that need
// an accepted post-epoch state must additionally call ValidateAcceptedState.
type StateProof struct {
	MachineHash     Hash
	IflagsYProof    LeafProof
	HtifTohostProof LeafProof
	TxBufferProof   LeafProof
}

// AdvanceResponse contains the result of a completed advance operation.
type AdvanceResponse struct {
	Status  CompletionStatus
	Outputs []Output
	Reports []Report
	// ExceptionData is the raw CMIO payload supplied by the guest when Status
	// is CompletionStatusException. It is nil for every other status; an
	// exception with an empty payload is represented by a non-nil empty slice.
	ExceptionData       []byte
	PeriodicStateHashes []Hash
	PaddingRepetitions  uint64
}

// InspectResponse contains the result of an inspect operation. On incomplete
// execution, Reports contains any reports emitted before the failure, Status
// is CompletionStatusUnknown, and Inspect returns a non-nil error.
type InspectResponse struct {
	Status CompletionStatus
	// ExceptionData has the same status-dependent meaning as
	// AdvanceResponse.ExceptionData.
	ExceptionData []byte
	Reports       []Report
}

// Common errors
var (
	ErrMachineInternal            = errors.New("machine internal error")
	ErrDeadlineExceeded           = fmt.Errorf("machine operation deadline exceeded: %w", context.DeadlineExceeded)
	ErrCanceled                   = fmt.Errorf("machine operation canceled: %w", context.Canceled)
	ErrOrphanServer               = errors.New("machine server was left orphan")
	ErrNotAtManualYield           = errors.New("not at manual yield")
	ErrException                  = errors.New("last request yielded an exception")
	ErrRejected                   = errors.New("last request yielded as rejected")
	ErrUnexpectedYield            = errors.New("last request yielded with an unsupported reason")
	ErrHalted                     = errors.New("machine halted")
	ErrOutputsLimitExceeded       = errors.New("outputs limit exceeded")
	ErrReportsLimitExceeded       = errors.New("reports limit exceeded")
	ErrPayloadLengthLimitExceeded = errors.New("payload length limit exceeded")
	ErrHashLength                 = errors.New("hash does not have the exactly number of bytes")
	ErrReachedLimitMcycle         = errors.New("machine reached limit mcycle")
	ErrInvalidMachineProof        = errors.New("invalid machine validity proof")

	// ErrMcycleOverflow preserves the emulator-reported fact that the machine
	// itself reached imcyclemax, rather than a node target. Canonical overflow
	// eligibility depends on that stop origin. It wraps ErrReachedLimitMcycle so
	// existing execution-limit classification remains unchanged.
	ErrMcycleOverflow = fmt.Errorf("machine reached imcyclemax: %w", ErrReachedLimitMcycle)
)

// IsExecutionLimitError reports whether err is an incomplete execution caused
// by a local payload, response-count, or cycle ceiling.
func IsExecutionLimitError(err error) bool {
	return errors.Is(err, ErrPayloadLengthLimitExceeded) ||
		errors.Is(err, ErrOutputsLimitExceeded) ||
		errors.Is(err, ErrReportsLimitExceeded) ||
		errors.Is(err, ErrReachedLimitMcycle)
}

// The Machine interface covers the core rollups-oriented functionalities of a cartesi
// machine: forking, getting the merkle tree's root hash, sending advance-state requests,
// sending inspect-state requests, and storing machine state.
type Machine interface {
	// Fork forks the machine.
	Fork(ctx context.Context) (Machine, error)
	// Hash returns the machine's merkle tree root hash.
	Hash(ctx context.Context) (Hash, error)
	// StateProof returns a complete, locally verified proof of the current
	// machine root and the three state leaves used by the rollups contracts.
	StateProof(ctx context.Context) (*StateProof, error)

	// Advance sends an input to the machine.
	// The checkpointHash is the machine's root hash before processing the input,
	// sent along with the request so the machine can revert to it if needed.
	// A non-nil response and nil error mean the machine completed with one of
	// the six completed CompletionStatus values. Any incomplete execution—input
	// validation, an operational limit, deadline/cancellation, or infrastructure
	// failure—returns a nil response and a non-nil error. CompletionStatusUnknown
	// is never returned by a successful call.
	Advance(ctx context.Context, input []byte, checkpointHash Hash, computeHashes bool) (*AdvanceResponse, error)

	// Inspect sends a query to the machine. A nil error means the guest completed
	// with one of the six non-unknown CompletionStatus values. On incomplete
	// execution, the response preserves reports emitted before the failure and
	// the error identifies why inspection could not complete.
	Inspect(ctx context.Context, query []byte) (*InspectResponse, error)

	// Store saves the machine state to the specified path.
	Store(ctx context.Context, path string) error

	// Close closes the inner cartesi machine.
	// It returns nil if the machine has already been closed.
	Close() error

	// Server information
	Address() string
}

// MachineConfig contains configuration for a machine instance
type MachineConfig struct { //nolint:revive // Keep the established public API name.
	Address             string                    // Address to connect to the machine backend
	Path                string                    // Path to the machine's directory
	ExecutionParameters model.ExecutionParameters // Execution parameters for the machine
	RuntimeConfig       *string
	BackendFactoryFn    BackendFactory // Optional factory for custom backend creation
}

// DefaultConfig returns a default machine configuration
func DefaultConfig(path string) *MachineConfig {
	return &MachineConfig{
		Address: "127.0.0.1:0",
		Path:    path,
		ExecutionParameters: model.ExecutionParameters{
			AdvanceIncCycles:   1 << 22, //nolint:mnd
			AdvanceMaxCycles:   0,
			InspectIncCycles:   1 << 22, //nolint:mnd
			InspectMaxCycles:   0,
			AdvanceIncDeadline: time.Second * 10,  //nolint:mnd
			AdvanceMaxDeadline: time.Second * 180, //nolint:mnd
			InspectIncDeadline: time.Second * 10,  //nolint:mnd
			InspectMaxDeadline: time.Second * 180, //nolint:mnd
			LoadDeadline:       time.Second * 300, //nolint:mnd
			StoreDeadline:      time.Second * 180, //nolint:mnd
			FastDeadline:       time.Second * 5,   //nolint:mnd
		},
		BackendFactoryFn: DefaultBackendFactory, // Use the default backend factory
	}
}

func DefaultBackendFactory(address string, timeout time.Duration) (Backend, string, uint32, error) {
	return NewLibCartesiBackend(address, timeout)
}

// Load loads a machine from a snapshot or template path
func Load(ctx context.Context, logger *slog.Logger, config *MachineConfig) (Machine, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if config == nil {
		return nil, errors.New("MachineConfig must not be nil")
	}

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	// Use the backend factory from config, or default to emulator.SpawnServer
	backendFactory := config.BackendFactoryFn
	if backendFactory == nil {
		backendFactory = DefaultBackendFactory
	}

	backend, address, pid, err := backendFactory(config.Address, config.ExecutionParameters.FastDeadline)
	if err != nil {
		return nil, errors.Join(ErrMachineInternal, err)
	}

	if config.RuntimeConfig == nil {
		runtimeConf, err := backend.NewMachineRuntimeConfig()
		if err != nil {
			shutdownErr := backend.ShutdownServer(config.ExecutionParameters.FastDeadline)
			backend.Delete()
			err = fmt.Errorf("could not instantiate new machine runtime config: %w", err)
			return nil, errors.Join(ErrMachineInternal, err, shutdownErr)
		}
		config.RuntimeConfig = &runtimeConf
	}

	machine := &machineImpl{
		backend: backend,
		address: address,
		pid:     pid,
		logger:  logger,
		params:  config.ExecutionParameters,
	}

	if err := checkContext(ctx); err != nil {
		machine.Close()
		return nil, err
	}

	// Loads the machine stored at path into the server.
	err = machine.backend.Load(config.Path, *config.RuntimeConfig, machine.params.LoadDeadline)
	if err != nil {
		machine.Close()
		err = fmt.Errorf("could not load the machine: %w", err)
		return nil, errors.Join(ErrMachineInternal, err)
	}

	// Ensures that the machine is at a manual yield.
	isAtManualYield, err := machine.isAtManualYield(ctx)
	if err != nil {
		machine.Close()
		return nil, err
	}
	if !isAtManualYield {
		machine.Close()
		return nil, ErrNotAtManualYield
	}

	// Ensures that the last request left the machine in an accepted manual yield.
	manualResult, err := machine.readManualYieldResult(ctx)
	if err != nil {
		machine.Close()
		return nil, err
	}
	switch manualResult.status {
	case CompletionStatusAccepted:
		return machine, nil
	case CompletionStatusException:
		machine.Close()
		return nil, ErrException
	case CompletionStatusRejected:
		machine.Close()
		return nil, ErrRejected
	case CompletionStatusUnexpectedYield:
		machine.Close()
		return nil, ErrUnexpectedYield
	case CompletionStatusUnknown, CompletionStatusHalted, CompletionStatusOverflow:
		machine.Close()
		return nil, fmt.Errorf(
			"invalid initial completion status %d: %w",
			manualResult.status,
			ErrMachineInternal,
		)
	default:
		machine.Close()
		return nil, fmt.Errorf(
			"unsupported initial completion status %d: %w",
			manualResult.status,
			ErrMachineInternal,
		)
	}
}

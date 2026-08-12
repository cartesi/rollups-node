// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/machine"
)

// InspectResult carries a typed machine completion or an incomplete execution
// failure. Error is non-nil only when Status is machine.CompletionStatusUnknown;
// Reports then contains any reports emitted before the failure.
type InspectResult struct {
	ProcessedInputs uint64
	Status          machine.CompletionStatus
	ExceptionData   []byte
	Reports         [][]byte
	Error           error
}

// MachineInstance defines the interface for a machine instance
type MachineInstance interface {
	Application() *Application
	Advance(ctx context.Context, input []byte, epochIndex uint64, inputIndex uint64, computeHashes bool) (*AdvanceResult, error)
	Inspect(ctx context.Context, query []byte) (*InspectResult, error)
	Synchronize(ctx context.Context, repo MachineRepository, batchSize uint64) error
	CreateSnapshot(ctx context.Context, processedInputs uint64, path string) error
	ProcessedInputs() uint64
	Hash(ctx context.Context) ([32]byte, error)
	OutputsProof(ctx context.Context) (*OutputsProof, error)
	Close() error
}

// MachineProvider defines the interface for accessing machines
type MachineProvider interface {
	// GetMachine retrieves a machine instance for an application
	GetMachine(appID int64) (MachineInstance, bool)

	// Applications returns the list of applications with active machines
	Applications() []*Application

	// UpdateMachines refreshes the list of machines
	UpdateMachines(ctx context.Context) error

	// HasMachine checks if a machine exists for the given application ID
	HasMachine(appID int64) bool

	// Close shuts down all machine instances and releases resources
	Close() error
}

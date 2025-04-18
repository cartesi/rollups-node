// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"

	. "github.com/cartesi/rollups-node/internal/model"
)

// MachineInstance defines the interface for a machine instance
type MachineInstance interface {
	Application() *Application
	Advance(ctx context.Context, input []byte, index uint64) (*AdvanceResult, error)
	Inspect(ctx context.Context, query []byte) (*InspectResult, error)
	Synchronize(ctx context.Context, repo MachineRepository) error
	CreateSnapshot(ctx context.Context, processedInputs uint64, path string) error
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
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machines

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	. "github.com/cartesi/rollups-node/internal/node/model"

	nm "github.com/cartesi/rollups-node/internal/nodemachine"
	"github.com/cartesi/rollups-node/pkg/emulator"
	rm "github.com/cartesi/rollups-node/pkg/rollupsmachine"
	cm "github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
)

type Repository interface {
	// GetMachineConfigurations retrieves a machine configuration for each application.
	GetMachineConfigurations(context.Context) ([]*MachineConfig, error)

	// GetProcessedInputs retrieves the processed inputs of an application with indexes greater or
	// equal to the given input index.
	GetProcessedInputs(_ context.Context, app Address, index uint64) ([]*Input, error)
}

// AdvanceMachine masks nodemachine.NodeMachine to only expose methods required by the Advancer.
type AdvanceMachine interface {
	Advance(_ context.Context, input []byte, index uint64) (*nm.AdvanceResult, error)
}

// InspectMachine masks nodemachine.NodeMachine to only expose methods required by the Inspector.
type InspectMachine interface {
	Inspect(_ context.Context, query []byte) (*nm.InspectResult, error)
}

// Machines is a thread-safe type that manages the pool of cartesi machines being used by the node.
// It contains a map of applications to machines.
type Machines struct {
	mutex    sync.RWMutex
	machines map[Address]*nm.NodeMachine
}

// Load initializes the cartesi machines.
// Load advances a machine to the last processed input stored in the database.
//
// Load does not fail when one of those machines fail to initialize.
// It stores the error to be returned later and continues to initialize the other machines.
func Load(ctx context.Context, repo Repository, verbosity cm.ServerVerbosity) (*Machines, error) {
	configs, err := repo.GetMachineConfigurations(ctx)
	if err != nil {
		return nil, err
	}

	machines := map[Address]*nm.NodeMachine{}
	var errs error

	for _, config := range configs {
		// Creates the machine.
		machine, err := createMachine(ctx, verbosity, config)
		if err != nil {
			err = fmt.Errorf("failed to create machine from snapshot (%v): %w", config, err)
			errs = errors.Join(errs, err)
			continue
		}

		// Advances the machine until it catches up with the state of the database (if necessary).
		err = catchUp(ctx, repo, config.AppAddress, machine, config.SnapshotInputIndex)
		if err != nil {
			err = fmt.Errorf("failed to advance cartesi machine (%v): %w", config, err)
			errs = errors.Join(errs, err, machine.Close())
			continue
		}

		machines[config.AppAddress] = machine
	}

	return &Machines{machines: machines}, errs
}

// GetAdvanceMachine gets the machine associated with the application from the map.
func (m *Machines) GetAdvanceMachine(app Address) AdvanceMachine {
	return m.getMachine(app)
}

// GetInspectMachine gets the machine associated with the application from the map.
func (m *Machines) GetInspectMachine(app Address) InspectMachine {
	return m.getMachine(app)
}

// Add maps a new application to a machine.
// It does nothing if the application is already mapped to some machine.
// It returns true if it was able to add the machine and false otherwise.
func (m *Machines) Add(app Address, machine *nm.NodeMachine) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.machines[app]; ok {
		return false
	} else {
		m.machines[app] = machine
		return true
	}
}

// Delete deletes an application from the map.
// It returns the associated machine, if any.
func (m *Machines) Delete(app Address) *nm.NodeMachine {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if machine, ok := m.machines[app]; ok {
		return nil
	} else {
		delete(m.machines, app)
		return machine
	}
}

// Apps returns the addresses of the applications for which there are machines.
func (m *Machines) Apps() []Address {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	keys := make([]Address, len(m.machines))
	i := 0
	for k := range m.machines {
		keys[i] = k
		i++
	}
	return keys
}

// Close closes all the machines and erases them from the map.
func (m *Machines) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	err := closeMachines(m.machines)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to close some machines: %v", err))
	}
	return err
}

// ------------------------------------------------------------------------------------------------

func (m *Machines) getMachine(app Address) *nm.NodeMachine {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.machines[app]
}

func closeMachines(machines map[Address]*nm.NodeMachine) (err error) {
	for _, machine := range machines {
		err = errors.Join(err, machine.Close())
	}
	for app := range machines {
		delete(machines, app)
	}
	return
}

func createMachine(ctx context.Context,
	verbosity cm.ServerVerbosity,
	config *MachineConfig,
) (*nm.NodeMachine, error) {
	slog.Info("creating machine", "application", config.AppAddress,
		"template-path", config.SnapshotPath, "service", "advancer")
	// Starts the server.
	address, err := cm.StartServer(verbosity, 0, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	// Creates a CartesiMachine from the snapshot.
	runtimeConfig := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cm.Load(ctx, config.SnapshotPath, address, runtimeConfig)
	if err != nil {
		return nil, errors.Join(err, cm.StopServer(address))
	}

	// Creates a RollupsMachine from the CartesiMachine.
	rollupsMachine, err := rm.New(ctx,
		cartesiMachine,
		config.AdvanceIncCycles,
		config.AdvanceMaxCycles)
	if err != nil {
		return nil, errors.Join(err, cartesiMachine.Close(ctx))
	}

	// Creates a NodeMachine from the RollupsMachine.
	nodeMachine, err := nm.New(rollupsMachine,
		config.SnapshotInputIndex,
		config.AdvanceMaxDeadline,
		config.InspectMaxDeadline,
		config.MaxConcurrentInspects)
	if err != nil {
		return nil, errors.Join(err, rollupsMachine.Close(ctx))
	}

	return nodeMachine, err
}

func catchUp(ctx context.Context,
	repo Repository,
	app Address,
	machine *nm.NodeMachine,
	snapshotInputIndex *uint64,
) error {
	// A nil index indicates we should start to process inputs from the beginning (index zero).
	// A non-nil index indicates we should start to process inputs from the next available index.
	firstInputIndexToProcess := uint64(0)
	if snapshotInputIndex != nil {
		firstInputIndexToProcess = *snapshotInputIndex + 1
	}

	slog.Info("catching up unprocessed inputs", "application", app, "service", "advancer")

	inputs, err := repo.GetProcessedInputs(ctx, app, firstInputIndexToProcess)
	if err != nil {
		return err
	}

	for _, input := range inputs {
		// FIXME epoch id to epoch index
		slog.Info("advancing", "application", app, "epochId", input.EpochId,
			"input-index", input.Index, "service", "advancer")
		_, err := machine.Advance(ctx, input.RawData, input.Index)
		if err != nil {
			return err
		}
	}

	return nil
}

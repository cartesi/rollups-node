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

	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/nodemachine"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
)

type Repository interface {
	GetAppData(context.Context) ([]*repository.AppData, error)
}

type AdvanceMachine interface {
	Advance(_ context.Context, input []byte, index uint64) (*nodemachine.AdvanceResult, error)
}

type InspectMachine interface {
	Inspect(_ context.Context, query []byte) (*nodemachine.InspectResult, error)
}

type Machines struct {
	mutex    sync.RWMutex
	machines map[model.Address]*nodemachine.NodeMachine
}

func Load(ctx context.Context, config config.NodeConfig, repo Repository) (*Machines, error) {
	appData, err := repo.GetAppData(ctx)
	if err != nil {
		return nil, err
	}

	machines := map[model.Address]*nodemachine.NodeMachine{}

	maxConcurrentInspects := config.MaxConcurrentInspects

	serverVerbosity := config.MachineServerVerbosity
	machineInc := config.MachineIncCycles
	machineMax := config.MachineMaxCycles
	machineAdvanceTimeout := config.MachineAdvanceTimeout
	machineInspectTimeout := config.MachineInspectTimeout

	for _, appData := range appData {
		appAddress := appData.AppAddress
		snapshotPath := appData.SnapshotPath
		snapshotInputIndex := appData.InputIndex

		address, err := cartesimachine.StartServer(serverVerbosity, 0, os.Stdout, os.Stderr)
		if err != nil {
			return nil, closeMachines(machines)
		}

		config := &emulator.MachineRuntimeConfig{}
		cartesiMachine, err := cartesimachine.Load(ctx, snapshotPath, address, config)
		if err != nil {
			err = errors.Join(err, cartesimachine.StopServer(address), closeMachines(machines))
			return nil, err
		}

		rollupsMachine, err := rollupsmachine.New(ctx, cartesiMachine, machineInc, machineMax)
		if err != nil {
			err = errors.Join(err, cartesiMachine.Close(ctx), closeMachines(machines))
			return nil, err
		}

		nodeMachine, err := nodemachine.New(rollupsMachine,
			snapshotInputIndex,
			machineAdvanceTimeout,
			machineInspectTimeout,
			maxConcurrentInspects)
		if err != nil {
			err = errors.Join(err, rollupsMachine.Close(ctx), closeMachines(machines))
			return nil, err
		}

		machines[appAddress] = nodeMachine
	}

	return &Machines{machines: machines}, nil
}

func (m *Machines) GetAdvanceMachine(app model.Address) AdvanceMachine {
	return m.get(app)
}

func (m *Machines) GetInspectMachine(app model.Address) InspectMachine {
	return m.get(app)
}

func (m *Machines) Set(app model.Address, machine *nodemachine.NodeMachine) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.machines[app]; ok {
		return false
	} else {
		m.machines[app] = machine
		return true
	}
}

func (m *Machines) Remove(app model.Address) *nodemachine.NodeMachine {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if machine, ok := m.machines[app]; ok {
		return nil
	} else {
		delete(m.machines, app)
		return machine
	}
}

func (m *Machines) Keys() []model.Address {
	m.mutex.RLock()
	defer m.mutex.Unlock()

	keys := make([]model.Address, len(m.machines))
	i := 0
	for k := range m.machines {
		keys[i] = k
		i++
	}
	return keys
}

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

func (m *Machines) get(app model.Address) *nodemachine.NodeMachine {
	m.mutex.RLock()
	defer m.mutex.Unlock()
	return m.machines[app]
}

func closeMachines(machines map[model.Address]*nodemachine.NodeMachine) (err error) {
	for _, machine := range machines {
		err = errors.Join(err, machine.Close())
	}
	for app := range machines {
		delete(machines, app)
	}
	return
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package nodemachine

import (
	"context"

	"github.com/cartesi/rollups-node/internal/node/nodemachine/pmutex"
	"github.com/cartesi/rollups-node/pkg/model"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"

	"golang.org/x/sync/semaphore"
)

type NodeMachine struct {
	*rollupsmachine.RollupsMachine

	// Ensures advance/inspect mutual exclusion when accessing the inner RollupsMachine.
	// Advances have a higher priority than Inspects to acquire the lock.
	mutex *pmutex.PMutex

	// Controls how many inspects can be concurrently active.
	inspects *semaphore.Weighted
}

func New(machine *rollupsmachine.RollupsMachine, maxConcurrentInspects int8) *NodeMachine {
	return &NodeMachine{
		RollupsMachine: machine,
		mutex:          pmutex.New(),
		inspects:       semaphore.NewWeighted(int64(maxConcurrentInspects)),
	}
}

func (machine *NodeMachine) Advance(input []byte) (
	outputs []rollupsmachine.Output,
	reports []rollupsmachine.Report,
	outputsHash model.Hash,
	machineHash model.Hash,
	err error) {

	var fork *rollupsmachine.RollupsMachine

	{ // Forks the machine.
		machine.mutex.HLock()
		defer machine.mutex.Unlock()
		fork, err = machine.Fork()
		if err != nil {
			return outputs, reports, outputsHash, machineHash, err
		}
	}

	// Sends the advance-state request.
	outputs, reports, outputsHash, err = fork.Advance(input)
	if err != nil {
		return outputs, reports, outputsHash, machineHash, err
	}

	// Gets the post-advance machine hash.
	machineHash, err = fork.Hash()
	if err != nil {
		return outputs, reports, outputsHash, machineHash, err
	}

	{ // Destroys the old machine and updates the current one.
		machine.mutex.HLock()
		defer machine.mutex.Unlock()
		err = machine.Destroy()
		if err != nil {
			return outputs, reports, outputsHash, machineHash, err
		}
		machine.RollupsMachine = fork
	}

	return outputs, reports, outputsHash, machineHash, err
}

func (machine *NodeMachine) Inspect(ctx context.Context, query []byte) (
	[]rollupsmachine.Report,
	error) {

	// Controls how many inspects can be concurrently active.
	err := machine.inspects.Acquire(ctx, 1)
	if err != nil {
		return nil, err
	}
	defer machine.inspects.Release(1)

	// Forks the machine.
	var forkedMachine *rollupsmachine.RollupsMachine
	{
		machine.mutex.LLock()
		defer machine.mutex.Unlock()
		forkedMachine, err = machine.Fork()
		if err != nil {
			return nil, err
		}
	}

	// Sends the inspect-state request.
	reports, err := forkedMachine.Inspect(query)
	if err != nil {
		return nil, err
	}

	// Destroys the forked machine and returns the reports.
	return reports, forkedMachine.Destroy()
}

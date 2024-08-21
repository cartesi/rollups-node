// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/node/advancer/poller"
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/nodemachine"
)

var (
	ErrInvalidMachines   = errors.New("machines must not be nil")
	ErrInvalidRepository = errors.New("repository must not be nil")

	ErrNoApp    = errors.New("no machine for application")
	ErrNoInputs = errors.New("no inputs")
)

type Advancer struct {
	machines   Machines
	repository Repository
}

// New instantiates a new Advancer.
func New(machines Machines, repository Repository) (*Advancer, error) {
	if machines == nil {
		return nil, ErrInvalidMachines
	}
	if repository == nil {
		return nil, ErrInvalidRepository
	}
	return &Advancer{machines: machines, repository: repository}, nil
}

// Poller instantiates a new poller.Poller using the Advancer.
func (advancer *Advancer) Poller(pollingInterval time.Duration) (*poller.Poller, error) {
	return poller.New("advancer", advancer, pollingInterval)
}

// Step steps the Advancer for one processing cycle.
// It gets unprocessed inputs from the repository,
// runs them through the cartesi machine,
// and updates the repository with the ouputs.
func (advancer *Advancer) Step(ctx context.Context) error {
	apps := keysFrom(advancer.machines)

	// Gets the unprocessed inputs (of all apps) from the repository.
	slog.Info("advancer: getting unprocessed inputs")
	inputs, err := advancer.repository.GetUnprocessedInputs(ctx, apps)
	if err != nil {
		return err
	}

	// Processes each set of inputs.
	for app, inputs := range inputs {
		slog.Info(fmt.Sprintf("advancer: processing %d input(s) from %v", len(inputs), app))
		err := advancer.process(ctx, app, inputs)
		if err != nil {
			return err
		}
	}

	return nil
}

// process sequentially processes inputs from the the application.
func (advancer *Advancer) process(ctx context.Context, app Address, inputs []*Input) error {
	// Asserts that the app has an associated machine.
	machine, ok := advancer.machines[app]
	if !ok {
		panic(fmt.Errorf("%w %s", ErrNoApp, app.String()))
	}

	// Asserts that there are inputs to process.
	if len(inputs) <= 0 {
		panic(ErrNoInputs)
	}

	for _, input := range inputs {
		slog.Info("advancer: processing input", "id", input.Id, "index", input.Index)

		// Sends the input to the cartesi machine.
		res, err := machine.Advance(ctx, input.RawData, input.Index)
		if err != nil {
			return err
		}

		// Stores the result in the database.
		err = advancer.repository.StoreAdvanceResult(ctx, input, res)
		if err != nil {
			return err
		}
	}

	// Updates the status of the epochs based on the last processed input.
	lastInput := inputs[len(inputs)-1]
	err := advancer.repository.UpdateEpochs(ctx, app, lastInput)

	return err
}

// ------------------------------------------------------------------------------------------------

type Repository interface {
	// Only needs Id, Index, and RawData fields from the retrieved Inputs.
	GetUnprocessedInputs(_ context.Context, apps []Address) (map[Address][]*Input, error)

	StoreAdvanceResult(context.Context, *Input, *nodemachine.AdvanceResult) error

	UpdateEpochs(_ context.Context, app Address, lastInput *Input) error
}

// A map of application addresses to machines.
type Machines = map[Address]Machine

type Machine interface {
	Advance(_ context.Context, input []byte, index uint64) (*nodemachine.AdvanceResult, error)
}

// ------------------------------------------------------------------------------------------------

// keysFrom returns a slice with the keysFrom of a map.
func keysFrom[K comparable, V any](m map[K]V) []K {
	keys := make([]K, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	return keys
}

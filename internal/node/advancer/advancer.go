// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/node/advancer/service"
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/node/nodemachine"
)

var (
	ErrInvalidMachines   = errors.New("machines must not be nil")
	ErrInvalidRepository = errors.New("repository must not be nil")
	ErrInvalidAddress    = errors.New("no machine for address")
)

type Advancer struct {
	machines   Machines
	repository Repository
}

func New(machines Machines, repository Repository) (*Advancer, error) {
	if machines == nil {
		return nil, ErrInvalidMachines
	}
	if repository == nil {
		return nil, ErrInvalidRepository
	}
	return &Advancer{machines: machines, repository: repository}, nil
}

func (advancer *Advancer) Poller(pollingInterval time.Duration) (*service.Poller, error) {
	return service.NewPoller("advancer", advancer, pollingInterval)
}

func (advancer *Advancer) Run(ctx context.Context) error {
	appAddresses := keysToSlice(advancer.machines)

	// Gets the unprocessed inputs (of all apps) from the repository.
	slog.Info("advancer: getting unprocessed inputs")
	inputs, err := advancer.repository.GetInputs(ctx, appAddresses)
	if err != nil {
		return err
	}

	// Processes each set of inputs.
	for appAddress, inputs := range inputs {
		slog.Info(fmt.Sprintf("advancer: processing %d input(s) from %v", len(inputs), appAddress))

		machine, ok := advancer.machines[appAddress]
		if !ok {
			return fmt.Errorf("%w %s", ErrInvalidAddress, appAddress.String())
		}

		// Processes inputs from the same application sequentially.
		for _, input := range inputs {
			slog.Info("advancer: processing input", "id", input.Id)

			res, err := machine.Advance(ctx, input.RawData)
			if err != nil {
				return err
			}

			err = advancer.repository.StoreResults(ctx, input, res)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// ------------------------------------------------------------------------------------------------

type Repository interface {
	// Only needs Id and RawData fields from model.Input.
	GetInputs(context.Context, []Address) (map[Address][]*Input, error)

	StoreResults(context.Context, *Input, *nodemachine.AdvanceResult) error
}

// A map of application addresses to machines.
type Machines = map[Address]Machine

type Machine interface {
	Advance(context.Context, []byte) (*nodemachine.AdvanceResult, error)
}

// ------------------------------------------------------------------------------------------------

// keysToSlice returns a slice with the keys of a map.
func keysToSlice[K comparable, V any](m map[K]V) []K {
	keys := make([]K, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	return keys
}

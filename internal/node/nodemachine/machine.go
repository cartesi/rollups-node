// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package nodemachine

import (
	"context"
	"errors"
	"time"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/node/nodemachine/pmutex"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"

	"golang.org/x/sync/semaphore"
)

var ErrTimeLimitExceeded = errors.New("time limit exceeded")

type AdvanceResult struct {
	Status      model.InputCompletionStatus
	Outputs     [][]byte
	Reports     [][]byte
	OutputsHash model.Hash
	MachineHash model.Hash
}

func (res AdvanceResult) StatusOk() bool {
	return res.Status == model.InputStatusAccepted || res.Status == model.InputStatusRejected
}

type InspectResult struct {
	Accepted bool
	Reports  [][]byte
	Err      error
}

type RollupsMachine interface {
	Fork() (*rollupsmachine.RollupsMachine, string, error) // NOTE: returns the concrete type
	Close() error

	Hash() (model.Hash, error)

	Advance([]byte) (bool, [][]byte, [][]byte, model.Hash, error)
	Inspect([]byte) (bool, [][]byte, error)
}

type NodeMachine struct {
	RollupsMachine

	// Timeout in seconds.
	timeout time.Duration

	// Ensures advance/inspect mutual exclusion when accessing the inner RollupsMachine.
	// Advances have a higher priority than Inspects to acquire the lock.
	mutex *pmutex.PMutex

	// Controls how many inspects can be concurrently active.
	inspects *semaphore.Weighted
}

func New(
	rollupsMachine RollupsMachine,
	timeout time.Duration,
	maxConcurrentInspects int8,
) *NodeMachine {
	return &NodeMachine{
		RollupsMachine: rollupsMachine,
		timeout:        timeout,
		mutex:          pmutex.New(),
		inspects:       semaphore.NewWeighted(int64(maxConcurrentInspects)),
	}
}

func (machine *NodeMachine) Advance(ctx context.Context, input []byte) (*AdvanceResult, error) {
	var fork RollupsMachine
	var err error

	// Forks the machine.
	machine.mutex.HLock()
	fork, _, err = machine.Fork()
	machine.mutex.Unlock()
	if err != nil {
		return nil, err
	}

	// Sends the advance-state request to the forked machine.
	accepted, outputs, reports, outputsHash, err := fork.Advance(input)
	status, err := toInputStatus(accepted, err)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}

	res := &AdvanceResult{
		Status:      status,
		Outputs:     outputs,
		Reports:     reports,
		OutputsHash: outputsHash,
	}

	// Only gets the post-advance machine hash if the request was accepted.
	if status == model.InputStatusAccepted {
		res.MachineHash, err = fork.Hash()
		if err != nil {
			return nil, errors.Join(err, fork.Close())
		}
	}

	// If the forked machine is in a valid state:
	if res.StatusOk() {
		// Closes the current machine.
		err = machine.RollupsMachine.Close()
		// Replaces the current machine with the fork.
		machine.mutex.HLock()
		machine.RollupsMachine = fork
		machine.mutex.Unlock()
	} else {
		// Closes the forked machine.
		err = fork.Close()
	}

	return res, err
}

func (machine *NodeMachine) Inspect(ctx context.Context, query []byte) (*InspectResult, error) {
	// Controls how many inspects can be concurrently active.
	err := machine.inspects.Acquire(ctx, 1)
	if err != nil {
		return nil, err
	}
	defer machine.inspects.Release(1)

	var fork RollupsMachine

	// Forks the machine.
	machine.mutex.LLock()
	fork, _, err = machine.RollupsMachine.Fork()
	machine.mutex.Unlock()
	if err != nil {
		return nil, err
	}

	// Sends the inspect-state request to the forked machine.
	res, _, timedOut := runWithTimeout(ctx, machine.timeout, func() (*InspectResult, error) {
		accepted, reports, err := fork.Inspect(query)
		return &InspectResult{Accepted: accepted, Reports: reports, Err: err}, nil
	})
	if timedOut {
		res = &InspectResult{Err: ErrTimeLimitExceeded}
	}

	return res, fork.Close()
}

// ------------------------------------------------------------------------------------------------

func toInputStatus(accepted bool, err error) (status model.InputCompletionStatus, _ error) {
	switch err {
	case nil:
		if accepted {
			return model.InputStatusAccepted, nil
		} else {
			return model.InputStatusRejected, nil
		}
	case rollupsmachine.ErrException:
		return model.InputStatusException, nil
	case rollupsmachine.ErrHalted:
		return model.InputStatusMachineHalted, nil
	case rollupsmachine.ErrCycleLimitExceeded:
		return model.InputStatusCycleLimitExceeded, nil
	case rollupsmachine.ErrOutputsLimitExceeded:
		panic("TODO")
	case rollupsmachine.ErrCartesiMachine,
		rollupsmachine.ErrProgress,
		rollupsmachine.ErrSoftYield:
		return status, err
	default:
		return status, err
	}

	// ErrPayloadLengthLimitExceeded
	// InputStatusPayloadLengthLimitExceeded
}

// Unused.
func runWithTimeout[T any](
	ctx context.Context,
	timeout time.Duration,
	f func() (*T, error),
) (_ *T, _ error, timedOut bool) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	success := make(chan *T, 1)
	failure := make(chan error, 1)
	go func() {
		t, err := f()
		if err != nil {
			failure <- err
		} else {
			success <- t
		}
	}()

	select {
	case <-ctx.Done():
		return nil, nil, true
	case t := <-success:
		return t, nil, false
	case err := <-failure:
		return nil, err, false
	}
}

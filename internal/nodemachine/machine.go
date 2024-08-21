// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package nodemachine

import (
	"context"
	"errors"
	"time"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/nodemachine/pmutex"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"

	"golang.org/x/sync/semaphore"
)

var (
	ErrInvalidAdvanceTimeout        = errors.New("advance timeout must not be negative")
	ErrInvalidInspectTimeout        = errors.New("inspect timeout must not be negative")
	ErrInvalidMaxConcurrentInspects = errors.New("maximum concurrent inspects must not be zero")
	ErrTimeLimitExceeded            = errors.New("time limit exceeded")
)

type AdvanceResult struct {
	Status      model.InputCompletionStatus
	Outputs     [][]byte
	Reports     [][]byte
	OutputsHash model.Hash
	MachineHash model.Hash
}

type InspectResult struct {
	Accepted bool
	Reports  [][]byte
	Error    error
}

type NodeMachine struct {
	inner rollupsmachine.RollupsMachine

	advanceTimeout, inspectTimeout time.Duration // TODO

	// Ensures advance/inspect mutual exclusion when accessing the inner RollupsMachine.
	// Advances have a higher priority than Inspects to acquire the lock.
	mutex *pmutex.PMutex

	// Controls how many inspects can be concurrently active.
	inspects *semaphore.Weighted
}

func New(
	inner rollupsmachine.RollupsMachine,
	advanceTimeout time.Duration,
	inspectTimeout time.Duration,
	maxConcurrentInspects uint8,
) (*NodeMachine, error) {
	if advanceTimeout < 0 {
		return nil, ErrInvalidAdvanceTimeout
	}
	if inspectTimeout < 0 {
		return nil, ErrInvalidInspectTimeout
	}
	if maxConcurrentInspects == 0 {
		return nil, ErrInvalidMaxConcurrentInspects
	}
	return &NodeMachine{
		inner:          inner,
		advanceTimeout: advanceTimeout,
		inspectTimeout: inspectTimeout,
		mutex:          pmutex.New(),
		inspects:       semaphore.NewWeighted(int64(maxConcurrentInspects)),
	}, nil
}

func (machine *NodeMachine) Advance(ctx context.Context, input []byte) (*AdvanceResult, error) {
	var fork rollupsmachine.RollupsMachine
	var err error

	// Forks the machine.
	machine.mutex.HLock()
	fork, err = machine.inner.Fork()
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
	if res.Status == model.InputStatusAccepted || res.Status == model.InputStatusRejected {
		// Closes the current machine.
		err = machine.inner.Close()
		// Replaces the current machine with the fork.
		machine.mutex.HLock()
		machine.inner = fork
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

	var fork rollupsmachine.RollupsMachine

	// Forks the machine.
	machine.mutex.LLock()
	fork, err = machine.inner.Fork()
	machine.mutex.Unlock()
	if err != nil {
		return nil, err
	}

	// Sends the inspect-state request to the forked machine.
	accepted, reports, err := fork.Inspect(query)
	res := &InspectResult{Accepted: accepted, Reports: reports, Error: err}

	return res, fork.Close()
}

func (machine *NodeMachine) Close() error {
	// TODO: not enough
	machine.mutex.HLock()
	err := machine.inner.Close()
	machine.mutex.Unlock()
	return err
}

// ------------------------------------------------------------------------------------------------

func toInputStatus(accepted bool, err error) (status model.InputCompletionStatus, _ error) {
	if err == nil {
		if accepted {
			return model.InputStatusAccepted, nil
		} else {
			return model.InputStatusRejected, nil
		}
	}

	switch {
	case errors.Is(err, rollupsmachine.ErrException):
		return model.InputStatusException, nil
	case errors.Is(err, rollupsmachine.ErrHalted):
		return model.InputStatusMachineHalted, nil
	case errors.Is(err, rollupsmachine.ErrOutputsLimitExceeded):
		return model.InputStatusOutputsLimitExceeded, nil
	case errors.Is(err, rollupsmachine.ErrCycleLimitExceeded):
		return model.InputStatusCycleLimitExceeded, nil
	case errors.Is(err, rollupsmachine.ErrPayloadLengthLimitExceeded):
		return model.InputStatusPayloadLengthLimitExceeded, nil
	case errors.Is(err, cartesimachine.ErrCartesiMachine),
		errors.Is(err, rollupsmachine.ErrProgress),
		errors.Is(err, rollupsmachine.ErrSoftYield):
		fallthrough
	default:
		return status, err
	}
}

// // Unused.
// func runWithTimeout[T any](
// 	ctx context.Context,
// 	timeout time.Duration,
// 	f func() (*T, error),
// ) (_ *T, _ error, timedOut bool) {
// 	ctx, cancel := context.WithTimeout(ctx, timeout)
// 	defer cancel()
//
// 	success := make(chan *T, 1)
// 	failure := make(chan error, 1)
// 	go func() {
// 		t, err := f()
// 		if err != nil {
// 			failure <- err
// 		} else {
// 			success <- t
// 		}
// 	}()
//
// 	select {
// 	case <-ctx.Done():
// 		return nil, nil, true
// 	case t := <-success:
// 		return t, nil, false
// 	case err := <-failure:
// 		return nil, err, false
// 	}
// }

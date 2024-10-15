// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package nodemachine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/nodemachine/pmutex"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"

	"golang.org/x/sync/semaphore"
)

var (
	ErrInvalidAdvanceTimeout        = errors.New("advance timeout must not be negative")
	ErrInvalidInspectTimeout        = errors.New("inspect timeout must not be negative")
	ErrInvalidMaxConcurrentInspects = errors.New("maximum concurrent inspects must not be zero")

	ErrClosed = errors.New("machine closed")
)

type AdvanceResult struct {
	Status      model.InputCompletionStatus
	Outputs     [][]byte
	Reports     [][]byte
	OutputsHash model.Hash
	MachineHash *model.Hash
}

type InspectResult struct {
	InputIndex *uint64
	Accepted   bool
	Reports    [][]byte
	Error      error
}

type NodeMachine struct {
	inner rollupsmachine.RollupsMachine

	// Index of the last Input that was processed.
	// Can be nil if no inputs were processed.
	lastInputIndex *uint64

	// How long a call to inner.Advance or inner.Inspect can take.
	advanceTimeout, inspectTimeout time.Duration

	// Maximum number of concurrent Inspects.
	maxConcurrentInspects int64

	// Controls concurrency between Advances and Inspects.
	// Advances and Inspects can be called concurrently, but Advances have a higher priority than
	// Inspects to acquire the lock.
	mutex *pmutex.PMutex

	// Controls concurrency between Advances.
	// Only one call to Advance can be active at a time (others will wait).
	concurrentAdvances sync.Mutex

	// Controls concurrency between Inspects.
	// At most N calls to Inspect can be active at the same time (others will wait).
	concurrentInspects *semaphore.Weighted
}

func New(
	inner rollupsmachine.RollupsMachine,
	inputIndex *uint64,
	advanceTimeout time.Duration,
	inspectTimeout time.Duration,
	maxConcurrentInspects int64,
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
		inner:                 inner,
		lastInputIndex:        inputIndex,
		advanceTimeout:        advanceTimeout,
		inspectTimeout:        inspectTimeout,
		maxConcurrentInspects: maxConcurrentInspects,
		mutex:                 pmutex.New(),
		concurrentInspects:    semaphore.NewWeighted(int64(maxConcurrentInspects)),
	}, nil
}

func (machine *NodeMachine) Advance(ctx context.Context,
	input []byte,
	index uint64,
) (*AdvanceResult, error) {
	// Only one advance can be active at a time.
	machine.concurrentAdvances.Lock()
	defer machine.concurrentAdvances.Unlock()

	var fork rollupsmachine.RollupsMachine
	var err error

	// Forks the machine.
	machine.mutex.HLock()
	if machine.inner == nil {
		return nil, ErrClosed
	}
	fork, err = machine.inner.Fork(ctx)
	machine.mutex.Unlock()
	if err != nil {
		return nil, err
	}

	advanceCtx, cancel := context.WithTimeout(ctx, machine.advanceTimeout)
	defer cancel()

	// Sends the advance-state request to the forked machine.
	accepted, outputs, reports, outputsHash, err := fork.Advance(advanceCtx, input)
	status, err := toInputStatus(accepted, err)
	if err != nil {
		return nil, errors.Join(err, fork.Close(ctx))
	}

	res := &AdvanceResult{
		Status:      status,
		Outputs:     outputs,
		Reports:     reports,
		OutputsHash: outputsHash,
	}

	// Only gets the post-advance machine hash if the request was accepted.
	if status == model.InputStatusAccepted {
		hash, err := fork.Hash(ctx)
		if err != nil {
			return nil, errors.Join(err, fork.Close(ctx))
		}
		res.MachineHash = (*model.Hash)(&hash)
	}

	// If the forked machine is in a valid state:
	if res.Status == model.InputStatusAccepted || res.Status == model.InputStatusRejected {
		// Closes the current machine.
		err = machine.inner.Close(ctx)
		// Replaces the current machine with the fork and updates lastInputIndex.
		machine.mutex.HLock()
		machine.inner = fork
		machine.lastInputIndex = &index
		machine.mutex.Unlock()
	} else {
		// Closes the forked machine.
		err = fork.Close(ctx)
		// Updates lastInputIndex.
		machine.mutex.HLock()
		machine.lastInputIndex = &index
		machine.mutex.Unlock()
	}

	return res, err
}

func (machine *NodeMachine) Inspect(ctx context.Context, query []byte) (*InspectResult, error) {
	// Controls how many inspects can be concurrently active.
	err := machine.concurrentInspects.Acquire(ctx, 1)
	if err != nil {
		return nil, err
	}
	defer machine.concurrentInspects.Release(1)

	var fork rollupsmachine.RollupsMachine

	// Forks the machine.
	machine.mutex.LLock()
	if machine.inner == nil {
		return nil, ErrClosed
	}
	fork, err = machine.inner.Fork(ctx)
	inputIndex := machine.lastInputIndex
	machine.mutex.Unlock()
	if err != nil {
		return nil, err
	}

	inspectCtx, cancel := context.WithTimeout(ctx, machine.inspectTimeout)
	defer cancel()

	// Sends the inspect-state request to the forked machine.
	accepted, reports, err := fork.Inspect(inspectCtx, query)
	res := &InspectResult{InputIndex: inputIndex, Accepted: accepted, Reports: reports, Error: err}

	return res, fork.Close(ctx)
}

func (machine *NodeMachine) Close() error {
	ctx := context.Background()

	// Makes sure no thread is accessing the machine before closing it.
	machine.concurrentAdvances.Lock()
	defer machine.concurrentAdvances.Unlock()
	for i := 0; i < int(machine.maxConcurrentInspects); i++ {
		_ = machine.concurrentInspects.Acquire(ctx, 1)
		defer machine.concurrentInspects.Release(1)
	}

	err := machine.inner.Close(ctx)
	machine.inner = nil
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

	if errors.Is(err, cartesimachine.ErrTimedOut) {
		return model.InputStatusTimeLimitExceeded, nil
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

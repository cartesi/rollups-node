// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package nodemachine

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/nodemachine/pmutex"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
	"github.com/ethereum/go-ethereum/common"

	"golang.org/x/sync/semaphore"
)

var (
	ErrInvalidApplication           = errors.New("application must not be nil")
	ErrInvalidAdvanceTimeout        = errors.New("advance timeout must not be negative")
	ErrInvalidInputIndex            = errors.New("advance input index must be equal to processed input count")
	ErrInvalidInspectTimeout        = errors.New("inspect timeout must not be negative")
	ErrInvalidMaxConcurrentInspects = errors.New("maximum concurrent inspects must not be zero")

	ErrClosed = errors.New("machine closed")
)

type NodeMachine struct {
	Application *Application

	inner rollupsmachine.RollupsMachine

	// How many inputs were processed by the machine.
	processedInputs uint64

	// How long a call to inner.Advance or inner.Inspect can take.
	advanceTimeout, inspectTimeout time.Duration

	// Maximum number of concurrent Inspects.
	maxConcurrentInspects uint32

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

func NewNodeMachine(
	app *Application,
	inner rollupsmachine.RollupsMachine,
	processedInputs uint64,
	advanceTimeout time.Duration,
	inspectTimeout time.Duration,
	maxConcurrentInspects uint32,
) (*NodeMachine, error) {
	if app == nil {
		return nil, ErrInvalidApplication
	}
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
		Application:           app,
		inner:                 inner,
		processedInputs:       processedInputs,
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
		machine.mutex.Unlock()
		return nil, ErrClosed
	}
	if machine.processedInputs != index {
		machine.mutex.Unlock()
		return nil, ErrInvalidInputIndex
	}
	fork, err = machine.inner.Fork(ctx)
	machine.mutex.Unlock()
	if err != nil {
		return nil, err
	}

	prevMachineHash, err := fork.Hash(ctx)
	if err != nil {
		return nil, errors.Join(err, fork.Close(ctx))
	}

	prevOutputsHash, err := fork.OutputsHash(ctx)
	if err != nil {
		return nil, errors.Join(err, fork.Close(ctx))
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
		InputIndex:  index,
		Status:      status,
		Outputs:     outputs,
		Reports:     reports,
		OutputsHash: outputsHash,
	}

	// If the forked machine is in a valid state:
	if res.Status == InputCompletionStatus_Accepted {
		// Only gets the post-advance machine hash if the request was accepted.
		machineHash, err := fork.Hash(ctx)
		if err != nil {
			return nil, errors.Join(err, fork.Close(ctx))
		}
		res.MachineHash = (*common.Hash)(&machineHash)

		// Replaces the current machine with the fork and updates lastInputIndex.
		machine.mutex.HLock()
		// Closes the current machine.
		err = machine.inner.Close(ctx)
		if err != nil {
			machine.mutex.Unlock()
			return nil, err
		}
		machine.inner = fork
		machine.processedInputs++
		machine.mutex.Unlock()
	} else {
		res.MachineHash = (*common.Hash)(&prevMachineHash)
		res.OutputsHash = prevOutputsHash
		// Closes the forked machine.
		err = fork.Close(ctx)
		// Updates lastInputIndex.
		machine.mutex.HLock()
		machine.processedInputs++
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
	processedInputs := machine.processedInputs
	machine.mutex.Unlock()
	if err != nil {
		return nil, err
	}

	inspectCtx, cancel := context.WithTimeout(ctx, machine.inspectTimeout)
	defer cancel()

	// Sends the inspect-state request to the forked machine.
	accepted, reports, err := fork.Inspect(inspectCtx, query)
	res := &InspectResult{ProcessedInputs: processedInputs, Accepted: accepted, Reports: reports, Error: err}

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

func toInputStatus(accepted bool, err error) (status InputCompletionStatus, _ error) {
	if err == nil {
		if accepted {
			return InputCompletionStatus_Accepted, nil
		} else {
			return InputCompletionStatus_Rejected, nil
		}
	}

	if errors.Is(err, cartesimachine.ErrTimedOut) {
		return InputCompletionStatus_TimeLimitExceeded, nil
	}

	switch {
	case errors.Is(err, rollupsmachine.ErrException):
		return InputCompletionStatus_Exception, nil
	case errors.Is(err, rollupsmachine.ErrHalted):
		return InputCompletionStatus_MachineHalted, nil
	case errors.Is(err, rollupsmachine.ErrOutputsLimitExceeded):
		return InputCompletionStatus_OutputsLimitExceeded, nil
	case errors.Is(err, rollupsmachine.ErrCycleLimitExceeded):
		return InputCompletionStatus_CycleLimitExceeded, nil
	case errors.Is(err, rollupsmachine.ErrPayloadLengthLimitExceeded):
		return InputCompletionStatus_PayloadLengthLimitExceeded, nil
	case errors.Is(err, cartesimachine.ErrCartesiMachine),
		errors.Is(err, rollupsmachine.ErrProgress),
		errors.Is(err, rollupsmachine.ErrSoftYield):
		fallthrough
	default:
		return status, err
	}
}

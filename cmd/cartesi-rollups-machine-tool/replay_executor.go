// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
)

// inputStatusLister is the repository surface the acceptedOnlyExecutor needs
// to load the persisted input statuses of the replay range.
type inputStatusLister interface {
	ListInputs(
		ctx context.Context,
		nameOrAddress string,
		f repository.InputFilter,
		p repository.Pagination,
		descending bool,
	) ([]*model.Input, uint64, error)
}

// acceptedOnlyExecutor drives a single machine over the replay range,
// executing only the inputs the database records as accepted. Rejected inputs
// leave the machine state unchanged: the guest reverts to the checkpoint and
// the node persisted the predecessor state for them, so the executor reports
// the predecessor state proof without running the machine. A terminal input is
// an error: by construction it can only be the last processed input, which the
// caller rejects up front when the range reaches it.
type acceptedOnlyExecutor struct {
	machine   machine.Machine
	repo      inputStatusLister
	app       string
	to        uint64
	chunkSize uint64
	statuses  map[uint64]model.InputCompletionStatus
	loadedTo  uint64
	processed uint64
	prev      model.StateProof
}

// newAcceptedOnlyExecutor captures the template state as the initial
// predecessor state and verifies that it is an accepted state before replay.
func newAcceptedOnlyExecutor(
	ctx context.Context,
	m machine.Machine,
	repo inputStatusLister,
	app string,
	to uint64,
	chunkSize uint64,
) (*acceptedOnlyExecutor, error) {
	proof, err := m.StateProof(ctx)
	if err != nil {
		return nil, fmt.Errorf("get template state proof: %w", err)
	}
	if err := machine.ValidateAcceptedState(proof); err != nil {
		return nil, fmt.Errorf("template is not at an accepted state: %w", err)
	}
	prev, err := toModelStateProof(proof)
	if err != nil {
		return nil, fmt.Errorf("template state proof: %w", err)
	}
	return &acceptedOnlyExecutor{
		machine:   m,
		repo:      repo,
		app:       app,
		to:        to,
		chunkSize: chunkSize,
		statuses:  make(map[uint64]model.InputCompletionStatus),
		prev:      prev,
	}, nil
}

// ProcessedInputs is the count of inputs the executor has reported for,
// accepted and skipped rejected alike.
func (e *acceptedOnlyExecutor) ProcessedInputs() uint64 {
	return e.processed
}

func (e *acceptedOnlyExecutor) Advance(
	ctx context.Context,
	input []byte,
	epochIndex uint64,
	inputIndex uint64,
	computeHashes bool,
) (*model.AdvanceResult, error) {
	status, err := e.statusFor(ctx, inputIndex)
	if err != nil {
		return nil, err
	}
	switch {
	case status == model.InputCompletionStatus_Accepted:
		return e.advanceAccepted(ctx, input, epochIndex, inputIndex, computeHashes)
	case status == model.InputCompletionStatus_Rejected:
		e.processed++
		return &model.AdvanceResult{
			StateProof: e.prev,
			EpochIndex: epochIndex,
			InputIndex: inputIndex,
			Status:     model.InputCompletionStatus_Rejected,
		}, nil
	default:
		return nil, fmt.Errorf(
			"input %d has terminal status %s; the machine cannot be snapshotted",
			inputIndex, status,
		)
	}
}

func (e *acceptedOnlyExecutor) advanceAccepted(
	ctx context.Context,
	input []byte,
	epochIndex uint64,
	inputIndex uint64,
	computeHashes bool,
) (*model.AdvanceResult, error) {
	resp, err := e.machine.Advance(ctx, input, machine.Hash(e.prev.MachineHash), computeHashes)
	if err != nil {
		return nil, fmt.Errorf("advance input %d: %w", inputIndex, err)
	}
	if resp.Status != machine.CompletionStatusAccepted {
		return nil, fmt.Errorf(
			"input %d: machine completed with %v, database records accepted",
			inputIndex, resp.Status,
		)
	}
	proof, err := e.machine.StateProof(ctx)
	if err != nil {
		return nil, fmt.Errorf("get state proof for input %d: %w", inputIndex, err)
	}
	if err := machine.ValidateAcceptedState(proof); err != nil {
		return nil, fmt.Errorf("input %d: %w", inputIndex, err)
	}
	next, err := toModelStateProof(proof)
	if err != nil {
		return nil, fmt.Errorf("input %d: %w", inputIndex, err)
	}
	e.prev = next
	e.processed++
	return &model.AdvanceResult{
		StateProof:          e.prev,
		EpochIndex:          epochIndex,
		InputIndex:          inputIndex,
		Status:              model.InputCompletionStatus_Accepted,
		Outputs:             resp.Outputs,
		Reports:             resp.Reports,
		PeriodicStateHashes: resp.PeriodicStateHashes,
		PaddingRepetitions:  resp.PaddingRepetitions,
	}, nil
}

// statusFor returns the persisted completion status of index, loading the
// chunk that contains it just in time.
func (e *acceptedOnlyExecutor) statusFor(ctx context.Context, index uint64) (model.InputCompletionStatus, error) {
	if index >= e.loadedTo {
		if err := e.loadChunk(ctx, e.loadedTo); err != nil {
			return "", err
		}
	}
	status, ok := e.statuses[index]
	if !ok {
		return "", fmt.Errorf("no persisted status for input %d", index)
	}
	return status, nil
}

// loadChunk loads the persisted statuses of [from, min(from+chunkSize, to))
// in one ListInputs call. The replay feeds inputs strictly in index order, so
// a chunk is loaded right before its first input is processed.
func (e *acceptedOnlyExecutor) loadChunk(ctx context.Context, from uint64) error {
	// Inputs are consumed strictly in index order, so the previous chunk's
	// statuses are no longer needed; drop them to bound memory by chunkSize.
	clear(e.statuses)
	to := min(from+e.chunkSize, e.to)
	inputs, _, err := e.repo.ListInputs(ctx, e.app,
		repository.InputFilter{IndexRange: &repository.Range{Start: from, End: to - 1}},
		repository.Pagination{Limit: to - from},
		false)
	if err != nil {
		return fmt.Errorf("load persisted statuses for inputs [%d, %d): %w", from, to, err)
	}
	if uint64(len(inputs)) != to-from {
		return fmt.Errorf(
			"expected %d persisted inputs in [%d, %d), got %d",
			to-from, from, to, len(inputs),
		)
	}
	for i, input := range inputs {
		expected := from + uint64(i)
		if input.Index != expected {
			return fmt.Errorf(
				"persisted input index %d out of sequence, expected %d",
				input.Index, expected,
			)
		}
		if !input.Status.IsCompleted() {
			return fmt.Errorf("input %d has non-completed status %s", input.Index, input.Status)
		}
		e.statuses[input.Index] = input.Status
	}
	e.loadedTo = to
	return nil
}

// toModelStateProof maps a machine state proof to the persisted model shape,
// checking completeness the same way the manager does.
func toModelStateProof(proof *machine.StateProof) (model.StateProof, error) {
	if proof == nil {
		return model.StateProof{}, fmt.Errorf(
			"machine returned no state proof: %w", machine.ErrInvalidMachineProof,
		)
	}
	result := model.StateProof{
		MachineHash:         common.Hash(proof.MachineHash),
		TxBufferDataBlock:   common.Hash(proof.TxBufferProof.DataBlock),
		TxBufferProof:       proof.TxBufferProof.Siblings,
		IflagsYDataBlock:    common.Hash(proof.IflagsYProof.DataBlock),
		IflagsYProof:        proof.IflagsYProof.Siblings,
		HtifTohostDataBlock: common.Hash(proof.HtifTohostProof.DataBlock),
		HtifTohostProof:     proof.HtifTohostProof.Siblings,
	}
	if !result.IsComplete() {
		return model.StateProof{}, fmt.Errorf(
			"machine returned an incomplete state proof: %w", machine.ErrInvalidMachineProof,
		)
	}
	return result, nil
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/stretchr/testify/require"
)

// testAcceptedStateProof builds a machine state proof that passes
// machine.ValidateAcceptedState: a non-zero iflags_Y word and an HTIF tohost
// word signalling an accepted manual yield.
func testAcceptedStateProof(root machine.Hash) *machine.StateProof {
	var iflagsY machine.Hash
	binary.LittleEndian.PutUint64(iflagsY[8:16], 1)
	var tohost machine.Hash
	accepted := uint64(2)<<56 | uint64(1)<<48 | uint64(1)<<32
	binary.LittleEndian.PutUint64(tohost[16:24], accepted)
	siblings := func() []machine.Hash {
		out := make([]machine.Hash, model.StateProofSiblingCount)
		for i := range out {
			out[i][0] = byte(i)
		}
		return out
	}
	return &machine.StateProof{
		MachineHash:     root,
		IflagsYProof:    machine.LeafProof{DataBlock: iflagsY, Siblings: siblings()},
		HtifTohostProof: machine.LeafProof{DataBlock: tohost, Siblings: siblings()},
		TxBufferProof:   machine.LeafProof{DataBlock: machine.Hash{0xaa}, Siblings: siblings()},
	}
}

func testInputs(count int, status model.InputCompletionStatus) []*model.Input {
	inputs := make([]*model.Input, count)
	for i := range inputs {
		inputs[i] = &model.Input{Index: uint64(i), Status: status}
	}
	return inputs
}

func testRange(start, end uint64) *repository.Range {
	return &repository.Range{Start: start, End: end}
}

// fakeMachine is a machine.Machine test double. StateProof returns the
// template proof until the first advance, then the post-advance proof.
type fakeMachine struct {
	proof          *machine.StateProof
	advanceProof   *machine.StateProof
	advanceResp    *machine.AdvanceResponse
	advanceErr     error
	advanceCalls   int
	lastInput      []byte
	lastCheckpoint machine.Hash
	lastCompute    bool
}

func (f *fakeMachine) Fork(context.Context) (machine.Machine, error) { return nil, nil }

func (f *fakeMachine) Hash(context.Context) (machine.Hash, error) {
	return f.proof.MachineHash, nil
}

func (f *fakeMachine) StateProof(context.Context) (*machine.StateProof, error) {
	if f.advanceCalls > 0 && f.advanceProof != nil {
		return f.advanceProof, nil
	}
	return f.proof, nil
}

func (f *fakeMachine) Advance(
	_ context.Context,
	input []byte,
	checkpoint machine.Hash,
	computeHashes bool,
) (*machine.AdvanceResponse, error) {
	f.advanceCalls++
	f.lastInput = input
	f.lastCheckpoint = checkpoint
	f.lastCompute = computeHashes
	return f.advanceResp, f.advanceErr
}

func (f *fakeMachine) Inspect(context.Context, []byte) (*machine.InspectResponse, error) {
	return nil, nil
}

func (f *fakeMachine) Store(context.Context, string) error { return nil }

func (f *fakeMachine) Close() error { return nil }

func (f *fakeMachine) Address() string { return "fake-machine" }

func (f *fakeMachine) GetProof(context.Context, uint64, int32, int32) (*machine.MemoryProof, error) {
	return nil, nil
}

// fakeRepo overrides the repository methods the tool uses; every other method
// is inherited from the embedded interface and must not be called in tests.
type fakeRepo struct {
	repository.Repository
	inputs     []*model.Input
	listReturn []*model.Input // when set, returned verbatim, ignoring the filter
	listCalls  []repository.InputFilter
	listLimits []uint64
	last       *model.Input
	lastCalls  int
	epoch      *model.Epoch
}

func (f *fakeRepo) ListInputs(
	_ context.Context,
	_ string,
	filter repository.InputFilter,
	p repository.Pagination,
	_ bool,
) ([]*model.Input, uint64, error) {
	f.listCalls = append(f.listCalls, filter)
	f.listLimits = append(f.listLimits, p.Limit)
	if f.listReturn != nil {
		return f.listReturn, uint64(len(f.listReturn)), nil
	}
	var out []*model.Input
	if filter.IndexRange != nil {
		for _, input := range f.inputs {
			if input.Index >= filter.IndexRange.Start && input.Index <= filter.IndexRange.End {
				out = append(out, input)
			}
		}
	}
	return out, uint64(len(out)), nil
}

func (f *fakeRepo) GetLastProcessedInput(context.Context, string) (*model.Input, error) {
	f.lastCalls++
	return f.last, nil
}

func (f *fakeRepo) GetEpoch(context.Context, string, uint64) (*model.Epoch, error) {
	return f.epoch, nil
}

func TestNewAcceptedOnlyExecutor(t *testing.T) {
	ctx := context.Background()

	t.Run("template state is accepted", func(t *testing.T) {
		t.Parallel()
		m := &fakeMachine{proof: testAcceptedStateProof(machine.Hash{1})}
		repo := &fakeRepo{}

		e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 10, 500)
		require.NoError(t, err)
		require.Equal(t, machine.Hash{1}, machine.Hash(e.prev.MachineHash))
		require.Zero(t, e.ProcessedInputs())
	})

	t.Run("template state not accepted", func(t *testing.T) {
		t.Parallel()
		proof := testAcceptedStateProof(machine.Hash{1})
		proof.IflagsYProof.DataBlock = machine.Hash{} // zero iflags_Y
		m := &fakeMachine{proof: proof}
		repo := &fakeRepo{}

		_, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 10, 500)
		require.Error(t, err)
		require.Contains(t, err.Error(), "template is not at an accepted state")
	})
}

func TestAcceptedOnlyExecutorAcceptedInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	templateRoot := machine.Hash{1}
	nextRoot := machine.Hash{2}
	m := &fakeMachine{
		proof:        testAcceptedStateProof(templateRoot),
		advanceProof: testAcceptedStateProof(nextRoot),
		advanceResp: &machine.AdvanceResponse{
			Status:              machine.CompletionStatusAccepted,
			Outputs:             []machine.Output{[]byte("out")},
			Reports:             []machine.Report{[]byte("rep")},
			PeriodicStateHashes: []machine.Hash{{9}},
			PaddingRepetitions:  3,
		},
	}
	repo := &fakeRepo{inputs: testInputs(2, model.InputCompletionStatus_Accepted)}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 2, 500)
	require.NoError(t, err)

	result, err := e.Advance(ctx, []byte("input-0"), 0, 0, false)
	require.NoError(t, err)
	require.Equal(t, 1, m.advanceCalls)
	require.Equal(t, []byte("input-0"), m.lastInput)
	require.Equal(t, templateRoot, m.lastCheckpoint)
	require.False(t, m.lastCompute)
	require.Equal(t, model.InputCompletionStatus_Accepted, result.Status)
	require.Zero(t, result.EpochIndex)
	require.Zero(t, result.InputIndex)
	require.Equal(t, nextRoot, machine.Hash(result.MachineHash))
	require.Equal(t, [][]byte{[]byte("out")}, result.Outputs)
	require.Equal(t, [][]byte{[]byte("rep")}, result.Reports)
	require.Equal(t, [][32]byte{{9}}, result.PeriodicStateHashes)
	require.Equal(t, uint64(3), result.PaddingRepetitions)
	require.Equal(t, uint64(1), e.ProcessedInputs())

	// the adopted state is the checkpoint of the next advance
	_, err = e.Advance(ctx, []byte("input-1"), 0, 1, false)
	require.NoError(t, err)
	require.Equal(t, 2, m.advanceCalls)
	require.Equal(t, nextRoot, m.lastCheckpoint)
	require.Equal(t, uint64(2), e.ProcessedInputs())
}

func TestAcceptedOnlyExecutorAcceptedButMachineDisagrees(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, status := range []machine.CompletionStatus{
		machine.CompletionStatusRejected,
		machine.CompletionStatusException,
	} {
		m := &fakeMachine{
			proof:       testAcceptedStateProof(machine.Hash{1}),
			advanceResp: &machine.AdvanceResponse{Status: status},
		}
		repo := &fakeRepo{inputs: testInputs(1, model.InputCompletionStatus_Accepted)}
		e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 1, 500)
		require.NoError(t, err)

		_, err = e.Advance(ctx, []byte("input"), 0, 0, false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "input 0: machine completed with")
		require.Contains(t, err.Error(), "database records accepted")
		require.Zero(t, e.ProcessedInputs())
	}
}

func TestAcceptedOnlyExecutorRejectedInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	templateRoot := machine.Hash{1}
	m := &fakeMachine{proof: testAcceptedStateProof(templateRoot)}
	repo := &fakeRepo{inputs: testInputs(1, model.InputCompletionStatus_Rejected)}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 1, 500)
	require.NoError(t, err)

	result, err := e.Advance(ctx, []byte("input"), 0, 0, false)
	require.NoError(t, err)
	require.Zero(t, m.advanceCalls)
	require.Equal(t, model.InputCompletionStatus_Rejected, result.Status)
	require.Equal(t, templateRoot, machine.Hash(result.MachineHash))
	require.Equal(t, uint64(1), e.ProcessedInputs())
}

func TestAcceptedOnlyExecutorTerminalStatus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &fakeMachine{proof: testAcceptedStateProof(machine.Hash{1})}
	repo := &fakeRepo{inputs: testInputs(1, model.InputCompletionStatus_Exception)}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 1, 500)
	require.NoError(t, err)

	_, err = e.Advance(ctx, []byte("input"), 0, 0, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "input 0 has terminal status EXCEPTION")
	require.Zero(t, m.advanceCalls)
	require.Zero(t, e.ProcessedInputs())
}

func TestAcceptedOnlyExecutorMissingStatus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &fakeMachine{proof: testAcceptedStateProof(machine.Hash{1})}
	repo := &fakeRepo{inputs: testInputs(1, model.InputCompletionStatus_Accepted)}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 1, 500)
	require.NoError(t, err)
	// simulate a range whose statuses were never persisted
	e.loadedTo = 1

	_, err = e.Advance(ctx, []byte("input"), 0, 0, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no persisted status for input 0")
}

func TestAcceptedOnlyExecutorLoadsStatusChunksOnDemand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &fakeMachine{proof: testAcceptedStateProof(machine.Hash{1})}
	repo := &fakeRepo{inputs: testInputs(1200, model.InputCompletionStatus_Rejected)}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 1200, 500)
	require.NoError(t, err)

	for i := uint64(0); i < 1200; i++ {
		_, err := e.Advance(ctx, []byte("input"), 0, i, false)
		require.NoError(t, err)
	}

	require.Equal(t, uint64(1200), e.ProcessedInputs())
	require.Len(t, repo.listCalls, 3)
	require.Equal(t, testRange(0, 499), repo.listCalls[0].IndexRange)
	require.Equal(t, testRange(500, 999), repo.listCalls[1].IndexRange)
	require.Equal(t, testRange(1000, 1199), repo.listCalls[2].IndexRange)
	require.Equal(t, []uint64{500, 500, 200}, repo.listLimits)
}

func TestAcceptedOnlyExecutorPartialChunk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &fakeMachine{proof: testAcceptedStateProof(machine.Hash{1})}
	repo := &fakeRepo{inputs: testInputs(120, model.InputCompletionStatus_Rejected)}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 120, 500)
	require.NoError(t, err)

	_, err = e.Advance(ctx, []byte("input"), 0, 0, false)
	require.NoError(t, err)

	require.Len(t, repo.listCalls, 1)
	require.Equal(t, testRange(0, 119), repo.listCalls[0].IndexRange)
	require.Equal(t, []uint64{120}, repo.listLimits)
}

func TestAcceptedOnlyExecutorShortChunkPage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &fakeMachine{proof: testAcceptedStateProof(machine.Hash{1})}
	repo := &fakeRepo{inputs: testInputs(499, model.InputCompletionStatus_Rejected)}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 500, 500)
	require.NoError(t, err)

	_, err = e.Advance(ctx, []byte("input"), 0, 0, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 500 persisted inputs in [0, 500), got 499")
}

func TestAcceptedOnlyExecutorOutOfSequenceChunk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &fakeMachine{proof: testAcceptedStateProof(machine.Hash{1})}
	repo := &fakeRepo{listReturn: []*model.Input{
		{Index: 0, Status: model.InputCompletionStatus_Rejected},
		{Index: 2, Status: model.InputCompletionStatus_Rejected}, // gap at 1
	}}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 2, 500)
	require.NoError(t, err)

	_, err = e.Advance(ctx, []byte("input"), 0, 0, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "persisted input index 2 out of sequence, expected 1")
}

func TestAcceptedOnlyExecutorNonCompletedStatusInChunk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &fakeMachine{proof: testAcceptedStateProof(machine.Hash{1})}
	inputs := testInputs(2, model.InputCompletionStatus_Rejected)
	inputs[1].Status = model.InputCompletionStatus_None
	repo := &fakeRepo{inputs: inputs}
	e, err := newAcceptedOnlyExecutor(ctx, m, repo, "app", 2, 500)
	require.NoError(t, err)

	_, err = e.Advance(ctx, []byte("input"), 0, 0, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "input 1 has non-completed status NONE")
}

func TestResolveReplayUpperBound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	app := &model.Application{Name: "app"}

	t.Run("to-epoch returns the epoch's upper bound", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{epoch: &model.Epoch{InputIndexUpperBound: 42}}

		bound, err := resolveReplayUpperBound(ctx, repo, app, replayOptions{HasToEpoch: true, ToEpoch: 7})
		require.NoError(t, err)
		require.Equal(t, uint64(42), bound)
	})

	t.Run("to-epoch with a missing epoch", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{}

		_, err := resolveReplayUpperBound(ctx, repo, app, replayOptions{HasToEpoch: true, ToEpoch: 7})
		require.Error(t, err)
		require.Contains(t, err.Error(), "epoch 7 not found")
	})

	t.Run("to-input-index adds one", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{}

		bound, err := resolveReplayUpperBound(ctx, repo, app, replayOptions{HasToInputIndex: true, ToInputIndex: 9})
		require.NoError(t, err)
		require.Equal(t, uint64(10), bound)
	})

	t.Run("to-input-index overflow", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{}

		_, err := resolveReplayUpperBound(ctx, repo, app, replayOptions{HasToInputIndex: true, ToInputIndex: math.MaxUint64})
		require.Error(t, err)
		require.Contains(t, err.Error(), "overflows")
	})
}

func TestCheckReplayRangeNotTerminal(t *testing.T) {
	ctx := context.Background()

	t.Run("range ends at terminal last input", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{last: &model.Input{Index: 9, Status: model.InputCompletionStatus_MachineHalted}}

		err := checkReplayRangeNotTerminal(ctx, repo, "app", 10, 10)
		require.Error(t, err)
		require.Contains(t, err.Error(), "last processed input 9 has terminal status MACHINE_HALTED")
		require.Equal(t, 1, repo.lastCalls)
	})

	t.Run("range ends at non-terminal last input", func(t *testing.T) {
		t.Parallel()
		for _, status := range []model.InputCompletionStatus{
			model.InputCompletionStatus_Accepted,
			model.InputCompletionStatus_Rejected,
		} {
			repo := &fakeRepo{last: &model.Input{Index: 9, Status: status}}

			require.NoError(t, checkReplayRangeNotTerminal(ctx, repo, "app", 10, 10))
			require.Equal(t, 1, repo.lastCalls)
		}
	})

	t.Run("range ends before last processed input", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{last: &model.Input{Index: 9, Status: model.InputCompletionStatus_MachineHalted}}

		require.NoError(t, checkReplayRangeNotTerminal(ctx, repo, "app", 10, 5))
		require.Zero(t, repo.lastCalls)
	})
}

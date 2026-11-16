// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// Keep the advancer ahead of validation: epochs 0/1 are complete but unclaimed,
// while input 2 in epoch 2 is still pending. Proof cryptography is not this
// repository fixture's oracle; it exercises transactional and eligibility rules.
func seedProblemEpochs(t *testing.T, r repository.Repository) (*model.Application, []*model.Epoch) {
	t.Helper()
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	epochs := make([]*model.Epoch, 3)
	rows := make(map[*model.Epoch][]*model.Input)
	for i := range uint64(3) {
		epochs[i] = repotest.NewEpochBuilder(app.ID).WithIndex(i).WithStatus(model.EpochStatus_Closed).
			WithBlocks(i*10, i*10+9).WithInputBounds(i, i+1).Build()
		input := repotest.NewInputBuilder().WithIndex(i).WithBlockNumber(i*10 + 5).Build()
		rows[epochs[i]] = []*model.Input{input}
	}
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, rows, 30))
	for i := range uint64(2) {
		proof := repotest.DummyStateProof()
		require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
			EpochIndex: i, InputIndex: i, Status: model.InputCompletionStatus_Accepted, StateProof: *proof,
		}))
		require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, i, proof))
		stored, err := r.GetEpoch(t.Context(), app.Name, i)
		require.NoError(t, err)
		stored.CalculatedOutputsRoot = stored.TxBufferDataBlock
		epochs[i] = stored
	}
	return app, epochs
}

func recordTestRuntimeStop(t *testing.T, r repository.Repository, app *model.Application, status model.InputCompletionStatus) error {
	t.Helper()
	result := &model.AdvanceResult{EpochIndex: 2, InputIndex: 2, Status: status, StateProof: *repotest.DummyStateProof()}
	if status == model.InputCompletionStatus_Exception {
		result.ExceptionData = []byte("guest exception")
	}
	return r.StoreAdvanceResult(t.Context(), app.ID, result)
}

func TestApplicationProblemRuntimeSourcesSurviveReconnect(t *testing.T) {
	for _, status := range []model.InputCompletionStatus{model.InputCompletionStatus_Exception,
		model.InputCompletionStatus_MachineHalted, model.InputCompletionStatus_Overflow,
		model.InputCompletionStatus_UnexpectedYield, model.InputCompletionStatus_InvalidOutputsRoot} {
		t.Run(status.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t)
			app, _ := seedProblemEpochs(t, r)
			require.NoError(t, recordTestRuntimeStop(t, r, app, status))
			endpoint, err := db.GetTestDatabaseEndpoint()
			require.NoError(t, err)
			reconnected, err := NewPostgresRepository(t.Context(), endpoint, 1, 0)
			require.NoError(t, err)
			defer reconnected.Close()
			state, err := reconnected.GetEpochCompletionState(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.NotNil(t, state.Problem)
			kind, _ := status.TerminalApplicationStatus()
			require.Equal(t, kind, state.Application.Status)
			require.Equal(t, &model.ApplicationProblem{EpochIndex: 2, Kind: kind, InputIndex: new(uint64(2))}, state.Problem)
			require.Equal(t, uint64(3), state.Application.ProcessedInputs)
			allowed, err := state.AllowsEpochCompletion()
			require.NoError(t, err)
			require.True(t, allowed)
		})
	}
}

func TestApplicationProblemCorruptedPreservesStopBoundary(t *testing.T) {
	for _, stop := range []model.ApplicationStatus{
		model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot,
	} {
		t.Run(stop.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t)
			app, _ := seedProblemEpochs(t, r)
			inputStatus := model.InputCompletionStatus_MachineHalted
			if stop == model.ApplicationStatus_InvalidOutputsRoot {
				inputStatus = model.InputCompletionStatus_InvalidOutputsRoot
			}
			require.NoError(t, recordTestRuntimeStop(t, r, app, inputStatus))
			wantProblem := &model.ApplicationProblem{EpochIndex: 2, Kind: stop, InputIndex: new(uint64(2))}

			before, err := r.GetEpochCompletionState(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, stop, before.Application.Status)
			require.Equal(t, wantProblem, before.Problem)

			reason := "persisted staged claim contradicts trusted chain observation"
			require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus_Corrupted, &reason))
			after, err := r.GetEpochCompletionState(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, model.ApplicationStatus_Corrupted, after.Application.Status)
			require.Equal(t, &reason, after.Application.Reason)
			require.Equal(t, wantProblem, after.Problem, "integrity escalation must preserve epoch, kind and input")
			require.Equal(t, before.Application.ProcessedInputs, after.Application.ProcessedInputs)
			require.Equal(t, before.Epoch, after.Epoch)
			allowed, err := after.AllowsEpochCompletion()
			require.NoError(t, err)
			require.False(t, allowed, "retained boundary is diagnostic context, not publication authorization")
		})
	}
}

func TestApplicationProblemRuntimeWriteRollsBackTogether(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, _ := seedProblemEpochs(t, r)
	_, err := r.db.Exec(t.Context(), `ALTER TABLE application ADD CONSTRAINT node3_runtime_rollback
        CHECK (status <> 'MACHINE_HALTED') NOT VALID`)
	require.NoError(t, err)
	err = recordTestRuntimeStop(t, r, app, model.InputCompletionStatus_MachineHalted)
	require.Error(t, err)
	state, err := r.GetEpochCompletionState(t.Context(), app.Name, 2)
	require.NoError(t, err)
	require.Nil(t, state.Problem)
	require.Equal(t, model.ApplicationStatus_OK, state.Application.Status)
	require.Equal(t, uint64(2), state.Application.ProcessedInputs)
	input, err := r.GetInput(t.Context(), app.Name, 2)
	require.NoError(t, err)
	require.Equal(t, model.InputCompletionStatus_None, input.Status)
	require.Nil(t, input.MachineHash)
	require.Nil(t, state.Epoch.MachineHash)
}

func TestApplicationProblemValidationRejectsStopVisibleBeforeWrite(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_Failed,
		model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		t.Run(status.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t)
			app, epochs := seedProblemEpochs(t, r)
			reason := "independent service or integrity failure"
			require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, status, &reason))
			// A stop arriving during the claim batch need not roll back valid work.
			require.ErrorIs(t, r.StoreClaimAndProofs(t.Context(), epochs[0], nil), repository.ErrApplicationNotRunnable)
			state, err := r.GetEpochCompletionState(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, status, state.Application.Status)
			require.Equal(t, &reason, state.Application.Reason)
			require.Nil(t, state.Problem)
			require.Equal(t, model.EpochStatus_InputsProcessed, state.Epoch.Status)
		})
	}
}

func TestApplicationProblemReaderDistinguishesAbsenceAndSnapshot(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, _ := seedProblemEpochs(t, r)
	missing, err := r.GetEpochCompletionState(t.Context(), "missing-application", 0)
	require.NoError(t, err)
	require.Nil(t, missing)
	state, err := r.GetEpochCompletionState(t.Context(), app.Name, 99)
	require.NoError(t, err)
	require.NotNil(t, state.Application)
	require.Nil(t, state.Epoch)
	// Exercise the exact private reads used by the public composed reader,
	// with a real concurrent commit between them in its REPEATABLE READ view.
	tx, err := beginReadTx(t.Context(), r.db)
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	before, problem, err := getApplication(t.Context(), tx, app.Name)
	require.NoError(t, err)
	require.Nil(t, problem)
	require.NoError(t, recordTestRuntimeStop(t, r, app, model.InputCompletionStatus_MachineHalted))
	epoch, err := getEpoch(t.Context(), tx, app.Name, 2)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_OK, before.Status)
	require.Nil(t, epoch.MachineHash, "both reads precede the committed stop in this snapshot")
	state, err = r.GetEpochCompletionState(t.Context(), app.Name, 2)
	require.NoError(t, err)
	require.NotNil(t, state.Problem)
	require.NotNil(t, state.Epoch.MachineHash)
}

func TestDecodeApplicationProblemRequiresInput(t *testing.T) {
	for _, kind := range []model.ApplicationStatus{model.ApplicationStatus_GuestException,
		model.ApplicationStatus_MachineHalted, model.ApplicationStatus_McycleOverflow,
		model.ApplicationStatus_UnexpectedYield, model.ApplicationStatus_InvalidOutputsRoot} {
		t.Run(kind.String(), func(t *testing.T) {
			epoch := uint64(0)
			problem, err := decodeApplicationProblem(&epoch, &kind, nil)
			require.Error(t, err, "every retained input-stop kind requires its input source")
			require.Nil(t, problem)
			input := uint64(0)
			problem, err = decodeApplicationProblem(&epoch, &kind, &input)
			require.NoError(t, err)
			require.Equal(t, &model.ApplicationProblem{EpochIndex: epoch, Kind: kind, InputIndex: &input}, problem,
				"input zero is a present source, including legacy input IOR")
		})
	}
}

func TestApplicationProblemSchemaRejectsPartialSources(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, _ := seedProblemEpochs(t, r)
	for _, query := range []string{
		`UPDATE application SET problem_epoch_index=0 WHERE id=$1`,
		`UPDATE application SET problem_kind='INVALID_OUTPUTS_ROOT' WHERE id=$1`,
		`UPDATE application SET problem_epoch_index=0,problem_kind='OK' WHERE id=$1`,
		`UPDATE application SET problem_epoch_index=0,problem_kind='MACHINE_HALTED' WHERE id=$1`,
		`UPDATE application SET problem_epoch_index=0,problem_kind='INVALID_OUTPUTS_ROOT' WHERE id=$1`,
	} {
		_, err := r.db.Exec(t.Context(), query, app.ID)
		var pgerr *pgconn.PgError
		require.ErrorAs(t, err, &pgerr)
		require.Equal(t, "application_problem_shape", pgerr.ConstraintName)
	}
	// No physical problem relation or second lifecycle survives app removal.
	require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
	require.NoError(t, r.DeleteApplication(t.Context(), app.ID))
	state, err := r.GetEpochCompletionState(context.Background(), app.Name, 0)
	require.NoError(t, err)
	require.Nil(t, state)
}

func storeTestIORClaim(t *testing.T, r repository.Repository, app *model.Application, index uint64) *model.Epoch {
	t.Helper()
	epoch, err := r.GetEpoch(t.Context(), app.Name, index)
	require.NoError(t, err)
	calculated := merkle.CreatePostContext()[merkle.TREE_DEPTH]
	require.NotEqual(t, &calculated, epoch.TxBufferDataBlock)
	epoch.CalculatedOutputsRoot = &calculated
	if app.IsDaveConsensus() {
		epoch.Commitment = new(repotest.UniqueHash())
		epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
	}
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
	return epoch
}

func TestFinalIORAndOpenSuccessorTerminalKeepSeparateEvidence(t *testing.T) {
	for _, terminalFirst := range []bool{false, true} {
		name := "IOR before terminal"
		if terminalFirst {
			name = "terminal before IOR"
		}
		t.Run(name, func(t *testing.T) {
			r := newEpochPublicationRepository(t)
			app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
			epoch := repotest.NewEpochBuilder(app.ID).WithIndex(0).WithStatus(model.EpochStatus_Closed).
				WithBlocks(0, 9).WithInputBounds(0, 1).Build()
			successor := repotest.NewEpochBuilder(app.ID).WithIndex(1).WithStatus(model.EpochStatus_Open).
				WithBlocks(9, 19).WithInputBounds(1, 3).Build()
			input := repotest.NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
			nextInput := repotest.NewInputBuilder().WithIndex(1).WithBlockNumber(9).Build()
			pendingInput := repotest.NewInputBuilder().WithIndex(2).WithBlockNumber(10).Build()
			require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
				map[*model.Epoch][]*model.Input{epoch: {input}, successor: {nextInput, pendingInput}}, 19))
			proof := repotest.DummyStateProof()
			require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
				EpochIndex: 0, InputIndex: 0, Status: model.InputCompletionStatus_Accepted, StateProof: *proof}))
			require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, 0, proof))
			terminal := func() {
				require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
					EpochIndex: 1, InputIndex: 1, Status: model.InputCompletionStatus_MachineHalted,
					StateProof: *repotest.DummyStateProof()}))
			}
			if terminalFirst {
				terminal()
			}
			computed := storeTestIORClaim(t, r, app, 0)
			if !terminalFirst {
				terminal()
			}
			state, err := r.GetEpochCompletionState(t.Context(), app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, model.ApplicationStatus_MachineHalted, state.Application.Status)
			require.Equal(t, uint64(2), state.Application.ProcessedInputs)
			require.ErrorIs(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
				EpochIndex: 1, InputIndex: 2, Status: model.InputCompletionStatus_Accepted,
				StateProof: *repotest.DummyStateProof()}), repository.ErrAdvanceAfterTerminal)
			pending, err := r.GetInput(t.Context(), app.Name, 2)
			require.NoError(t, err)
			require.Equal(t, model.InputCompletionStatus_None, pending.Status)
			require.Equal(t, &model.ApplicationProblem{EpochIndex: 1, Kind: model.ApplicationStatus_MachineHalted,
				InputIndex: new(uint64(1))}, state.Problem)
			require.Equal(t, model.EpochStatus_ClaimComputed, state.Epoch.Status)
			require.Equal(t, computed.CalculatedOutputsRoot, state.Epoch.CalculatedOutputsRoot)
			require.True(t, state.Epoch.HasInvalidOutputsRoot())
			require.NotNil(t, state.Epoch.Commitment)
			allowed, err := state.AllowsEpochCompletion()
			require.NoError(t, err)
			require.True(t, allowed, "the completed current epoch remains eligible for root join")
			next, err := r.GetEpochCompletionState(t.Context(), app.Name, 1)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_Open, next.Epoch.Status)
			allowed, err = next.AllowsEpochCompletion()
			require.NoError(t, err)
			require.False(t, allowed)
		})
	}
}

func TestFinalIORProofBatchAllowsOpenSuccessorTerminal(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	r := newEpochPublicationRepository(t)
	app, epoch := seedEpochPublication(t, r)
	storedInput, err := r.GetInput(ctx, app.Name, 0)
	require.NoError(t, err)
	stateProof := repotest.DummyStateProof()
	stateProof.MachineHash = *storedInput.MachineHash
	stateProof.TxBufferDataBlock = *storedInput.TxBufferDataBlock
	require.NoError(t, r.UpdateEpochInputsProcessed(ctx, app.Name, epoch.Index, stateProof))
	successor := repotest.NewEpochBuilder(app.ID).WithIndex(1).WithStatus(model.EpochStatus_Open).
		WithBlocks(9, 19).WithInputBounds(1, 2).Build()
	input := repotest.NewInputBuilder().WithIndex(1).WithBlockNumber(9).Build()
	require.NoError(t, r.CreateEpochsAndInputs(ctx, app.Name, map[*model.Epoch][]*model.Input{successor: {input}}, 19))
	output, err := r.GetOutput(ctx, app.Name, 0)
	require.NoError(t, err)
	pristine := merkle.CreatePostContext()
	proof := repository.OutputProof{Index: output.Index, Hash: crypto.Keccak256Hash(output.RawData),
		Siblings: pristine[:merkle.TREE_DEPTH]}
	calculated := merkletest.ComputeRootHashFromProof(proof.Index, proof.Hash, proof.Siblings)
	require.NotEqual(t, stateProof.TxBufferDataBlock, calculated)
	epoch.CalculatedOutputsRoot = &calculated
	epoch.Commitment = new(repotest.UniqueHash())
	epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)

	endpoint, err := db.GetTestDatabaseEndpoint()
	require.NoError(t, err)
	advancer, err := NewPostgresRepository(ctx, endpoint, 1, 0)
	require.NoError(t, err)
	defer advancer.Close()
	paused := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	completed := false
	t.Cleanup(func() {
		cancel()
		if !completed {
			select {
			case err := <-result:
				if err != nil {
					t.Logf("claim callback stopped during cleanup: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("claim callback did not stop after cancellation")
			}
		}
	})
	go func() {
		result <- r.StoreClaimAndProofs(ctx, epoch, func(ctx context.Context, writer repository.ClaimProofWriter) error {
			if err := writer.StoreOutputProofs(ctx, []repository.OutputProof{proof}); err != nil {
				return err
			}
			close(paused)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-paused:
	case err := <-result:
		completed = true
		t.Fatalf("claim callback stopped before its first proof batch: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	pending, err := advancer.GetEpoch(ctx, app.Name, 0)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_InputsProcessed, pending.Status)
	require.Nil(t, pending.CalculatedOutputsRoot)
	pendingOutput, err := advancer.GetOutput(ctx, app.Name, output.Index)
	require.NoError(t, err)
	require.Nil(t, pendingOutput.Hash)

	terminalCtx, stopTerminal := context.WithTimeout(ctx, 3*time.Second)
	err = advancer.StoreAdvanceResult(terminalCtx, app.ID, &model.AdvanceResult{
		EpochIndex: 1, InputIndex: 1, Status: model.InputCompletionStatus_MachineHalted,
		StateProof: *repotest.DummyStateProof()})
	stopTerminal()
	require.NoError(t, err, "the real input writer must not wait for the earlier proof callback")
	during, err := advancer.GetEpochCompletionState(ctx, app.Name, 0)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_MachineHalted, during.Application.Status)
	expectedProblem := &model.ApplicationProblem{EpochIndex: 1, Kind: model.ApplicationStatus_MachineHalted,
		InputIndex: new(uint64(1))}
	require.Equal(t, expectedProblem, during.Problem)
	require.Equal(t, uint64(2), during.Application.ProcessedInputs)
	require.Nil(t, during.Epoch.CalculatedOutputsRoot)
	close(release)
	select {
	case err := <-result:
		completed = true
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stored, err := advancer.GetEpochCompletionState(ctx, app.Name, 0)
	require.NoError(t, err)
	require.Equal(t, expectedProblem, stored.Problem)
	require.Equal(t, model.ApplicationStatus_MachineHalted, stored.Application.Status)
	require.Equal(t, model.EpochStatus_ClaimComputed, stored.Epoch.Status)
	require.Equal(t, &calculated, stored.Epoch.CalculatedOutputsRoot)
	require.Equal(t, epoch.Commitment, stored.Epoch.Commitment)
	require.Equal(t, epoch.CommitmentProof, stored.Epoch.CommitmentProof)
	require.True(t, stored.Epoch.HasInvalidOutputsRoot())
	storedOutput, err := advancer.GetOutput(ctx, app.Name, output.Index)
	require.NoError(t, err)
	require.Equal(t, &proof.Hash, storedOutput.Hash)
	require.Equal(t, proof.Siblings, storedOutput.OutputHashesSiblings)
	allowed, err := stored.AllowsEpochCompletion()
	require.NoError(t, err)
	require.True(t, allowed)
}

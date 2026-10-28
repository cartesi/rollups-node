// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
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

func TestApplicationProblemEarlierIORAndFirstIntegrityStop(t *testing.T) {
	r := newEpochPublicationRepository(t)
	app, epochs := seedProblemEpochs(t, r)
	require.NoError(t, recordTestRuntimeStop(t, r, app, model.InputCompletionStatus_MachineHalted))
	require.NoError(t, r.StoreInvalidOutputsRoot(t.Context(), app.ID, 1, "epoch 1 root mismatch"))
	require.NoError(t, r.StoreInvalidOutputsRoot(t.Context(), app.ID, 1, "same evidence retried"))
	state, err := r.GetEpochCompletionState(t.Context(), app.Name, 0)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_MachineHalted, state.Application.Status)
	require.Equal(t, &model.ApplicationProblem{EpochIndex: 1, Kind: model.ApplicationStatus_InvalidOutputsRoot}, state.Problem)
	require.Equal(t, uint64(3), state.Application.ProcessedInputs, "publication cutoff does not rewind execution")
	// Defensive stale-caller case: the ordered, single validator does not
	// normally compute epoch 1 concurrently with its own earlier IOR finding.
	// A cut already visible when the write starts still rejects that epoch.
	require.ErrorIs(t, r.StoreClaimAndProofs(t.Context(), epochs[1], nil), repository.ErrApplicationNotRunnable)
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), epochs[0], nil))
	for _, kind := range []model.ApplicationStatus{model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		reason := "integrity finding"
		err := r.UpdateApplicationStatus(t.Context(), app.ID, kind, &reason)
		if kind == model.ApplicationStatus_Diverged {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	state, err = r.GetEpochCompletionState(t.Context(), app.Name, 1)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Diverged, state.Application.Status)
	require.Equal(t, uint64(1), state.Problem.EpochIndex)
	allowed, err := state.AllowsEpochCompletion()
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestApplicationProblemCorruptedPreservesStopBoundary(t *testing.T) {
	for _, stop := range []model.ApplicationStatus{
		model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot,
	} {
		t.Run(stop.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t)
			app, _ := seedProblemEpochs(t, r)
			wantProblem := &model.ApplicationProblem{EpochIndex: 1, Kind: model.ApplicationStatus_InvalidOutputsRoot}
			if stop == model.ApplicationStatus_MachineHalted {
				require.NoError(t, recordTestRuntimeStop(t, r, app, model.InputCompletionStatus_MachineHalted))
				wantProblem = &model.ApplicationProblem{EpochIndex: 2, Kind: stop, InputIndex: new(uint64(2))}
			} else {
				require.NoError(t, r.StoreInvalidOutputsRoot(t.Context(), app.ID, 1, "epoch 1 root mismatch"))
			}
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

func TestApplicationProblemValidatorWriteRollsBackTogether(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, _ := seedProblemEpochs(t, r)
	_, err := r.db.Exec(t.Context(), `ALTER TABLE application ADD CONSTRAINT node3_validator_rollback
        CHECK (status <> 'INVALID_OUTPUTS_ROOT') NOT VALID`)
	require.NoError(t, err)
	require.Error(t, r.StoreInvalidOutputsRoot(t.Context(), app.ID, 0, "root mismatch"))
	state, err := r.GetEpochCompletionState(t.Context(), app.Name, 0)
	require.NoError(t, err)
	require.Nil(t, state.Problem)
	require.Equal(t, model.ApplicationStatus_OK, state.Application.Status)
	require.Equal(t, model.EpochStatus_InputsProcessed, state.Epoch.Status)
}

func TestApplicationProblemValidationRejectsStopVisibleBeforeWrite(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_Failed,
		model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		t.Run(status.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t)
			app, epochs := seedProblemEpochs(t, r)
			reason := "independent service or integrity failure"
			require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, status, &reason))
			// Both paths begin their writes after the durable stop is visible.
			// A stop arriving during the claim batch need not roll back valid work.
			err := r.StoreInvalidOutputsRoot(t.Context(), app.ID, 0, "later root finding")
			require.ErrorIs(t, err, repository.ErrApplicationNotRunnable)
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

func TestApplicationProblemConcurrentIOROnlyLowersBoundary(t *testing.T) {
	r := newEpochPublicationRepository(t)
	app, _ := seedProblemEpochs(t, r)
	require.NoError(t, recordTestRuntimeStop(t, r, app, model.InputCompletionStatus_MachineHalted))
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, index := range []uint64{0, 1} {
		go func() { <-start; results <- r.StoreInvalidOutputsRoot(t.Context(), app.ID, index, "root mismatch") }()
	}
	close(start)
	for range 2 {
		err := <-results
		require.True(t, err == nil || errors.Is(err, repository.ErrApplicationNotRunnable), "unexpected writer result: %v", err)
	}
	state, err := r.GetEpochCompletionState(t.Context(), app.Name, 0)
	require.NoError(t, err)
	require.Equal(t, &model.ApplicationProblem{EpochIndex: 0, Kind: model.ApplicationStatus_InvalidOutputsRoot}, state.Problem)
	require.Equal(t, model.ApplicationStatus_MachineHalted, state.Application.Status)
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

func TestApplicationProblemSchemaRejectsPartialSources(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, _ := seedProblemEpochs(t, r)
	for _, query := range []string{
		`UPDATE application SET problem_epoch_index=0 WHERE id=$1`,
		`UPDATE application SET problem_kind='INVALID_OUTPUTS_ROOT' WHERE id=$1`,
		`UPDATE application SET problem_epoch_index=0,problem_kind='OK' WHERE id=$1`,
		`UPDATE application SET problem_epoch_index=0,problem_kind='MACHINE_HALTED' WHERE id=$1`,
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

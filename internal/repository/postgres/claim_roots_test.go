// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

func claimValidated(t *testing.T, r *PostgresRepository, appID int64, index uint64) bool {
	t.Helper()
	var validated bool
	require.NoError(t, r.db.QueryRow(t.Context(), `SELECT claim_validated FROM epoch WHERE application_id=$1 AND index=$2`, appID, index).
		Scan(&validated))
	return validated
}

func TestClaimValidationSurvivesBothForeclosureWriters(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	proofApp, proofEpoch, sourceProof := machinePublicationEpoch(t, r, false)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), proofApp.Name, proofEpoch.Index, sourceProof))
	// These are repository writer/trigger tests, not on-chain exploit fixtures.
	for _, writer := range []string{"single", "bulk"} {
		for _, state := range []model.EpochStatus{
			model.EpochStatus_Closed, model.EpochStatus_InputsProcessed, model.EpochStatus_ClaimComputed,
		} {
			t.Run(writer+"/"+state.String(), func(t *testing.T) {
				app, epoch, proof := machinePublicationEpoch(t, r, false)
				if state == model.EpochStatus_Closed {
					// Complete bytes in CLOSED still do not establish validation.
					_, err := r.db.Exec(t.Context(), `UPDATE epoch e SET
 (machine_hash,tx_buffer_data_block,tx_buffer_proof,iflags_y_data_block,iflags_y_proof,htif_tohost_data_block,htif_tohost_proof)=
 (SELECT machine_hash,tx_buffer_data_block,tx_buffer_proof,iflags_y_data_block,iflags_y_proof,htif_tohost_data_block,htif_tohost_proof
 FROM epoch WHERE application_id=$3 AND index=$4) WHERE e.application_id=$1 AND e.index=$2`,
						app.ID, epoch.Index, proofApp.ID, proofEpoch.Index)
					require.NoError(t, err)
				}
				if state != model.EpochStatus_Closed {
					require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
				}
				if state == model.EpochStatus_ClaimComputed {
					require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
				}
				want := state == model.EpochStatus_ClaimComputed
				require.Equal(t, want, claimValidated(t, r, app.ID, epoch.Index))
				require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 49, repotest.UniqueHash(), 80))
				if writer == "single" {
					require.NoError(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, epoch.Index))
				} else {
					count, err := r.ForecloseUnacceptedEpochsAtOrAfterBlock(t.Context(), app.ID, 49)
					require.NoError(t, err)
					require.EqualValues(t, 1, count)
				}
				require.Equal(t, want, claimValidated(t, r, app.ID, epoch.Index))
				// An idempotent CF write must neither erase validation nor infer it from
				// the non-null proof carried by CLOSED/IP. It also ignores a stale value.
				_, err := r.db.Exec(t.Context(), `UPDATE epoch SET status='CLAIM_FORECLOSED',claim_validated=$3
     WHERE application_id=$1 AND index=$2`, app.ID, epoch.Index, !want)
				require.NoError(t, err)
				require.Equal(t, want, claimValidated(t, r, app.ID, epoch.Index))
				got, err := r.ReadClaimRoots(t.Context(), app.ID, 59)
				require.NoError(t, err)
				require.Equal(t, want, got.Ready)
				if want {
					require.Equal(t, proof.MachineHash, got.MachineHash)
					require.Equal(t, proof.TxBufferDataBlock, got.OutputsRoot)
				} else {
					require.Contains(t, got.Reason, "CLAIM_FORECLOSED")
				}
			})
		}
	}
}

func TestClaimValidationRollbackAndPrefix(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch, proof := machinePublicationEpoch(t, r, false)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
	tx, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	_, err = tx.Exec(t.Context(), `UPDATE epoch SET status='CLAIM_COMPUTED' WHERE application_id=$1 AND index=$2`, app.ID, epoch.Index)
	require.NoError(t, err)
	var validated bool
	require.NoError(t, tx.QueryRow(t.Context(),
		`SELECT claim_validated FROM epoch WHERE application_id=$1 AND index=$2`, app.ID, epoch.Index).
		Scan(&validated))
	require.True(t, validated)
	require.NoError(t, tx.Rollback(t.Context()))
	require.False(t, claimValidated(t, r, app.ID, epoch.Index))
	got, err := r.ReadClaimRoots(t.Context(), app.ID, 59)
	require.NoError(t, err)
	require.False(t, got.Ready, "IP proof bytes are not validation")
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
	got, err = r.ReadClaimRoots(t.Context(), app.ID, 59)
	require.NoError(t, err)
	require.True(t, got.Ready)
	require.Equal(t, proof.MachineHash, got.MachineHash)
	require.Equal(t, proof.TxBufferDataBlock, got.OutputsRoot, "the empty successor has no row and carries the prior roots")
	require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus_Diverged, new("unrelated integrity finding")))
	got, err = r.ReadClaimRoots(t.Context(), app.ID, 59)
	require.NoError(t, err)
	require.True(t, got.Ready, "application health cannot hide already validated local evidence")
	got, err = r.ReadClaimRoots(t.Context(), app.ID, 39)
	require.NoError(t, err)
	require.True(t, got.Ready, "future epochs do not block an empty earlier prefix")
	require.Equal(t, app.TemplateHash, got.MachineHash)
	require.Equal(t, merkle.CreatePostContext()[merkle.TREE_DEPTH], got.OutputsRoot)
	got, err = r.ReadClaimRoots(t.Context(), app.ID, 58)
	require.NoError(t, err)
	require.False(t, got.Ready, "partial epochs cannot use carry-forward")
	_, err = r.db.Exec(t.Context(), `UPDATE application SET last_input_check_block=58 WHERE id=$1`, app.ID)
	require.NoError(t, err)
	got, err = r.ReadClaimRoots(t.Context(), app.ID, 59)
	require.NoError(t, err)
	require.False(t, got.Ready, "unread inputs forbid root comparison")
	require.Contains(t, got.Reason, "input coverage")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = r.ReadClaimRoots(ctx, app.ID, 59)
	require.ErrorIs(t, err, context.Canceled)
	_, err = r.ReadClaimRoots(t.Context(), app.ID+100000, 59)
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func TestClaimValidationRequiresEveryPredecessor(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch, proof := machinePublicationEpoch(t, r, false)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
	// A complete last epoch does not validate an earlier unfinished predecessor.
	// This explicit repository fixture does not emulate machine execution order.
	_, err := r.db.Exec(t.Context(), `INSERT INTO epoch(application_id,index,virtual_index,first_block,last_block,
 input_index_lower_bound,input_index_upper_bound,status) VALUES($1,2,2,20,29,0,0,'CLOSED')`, app.ID)
	require.NoError(t, err)
	got, err := r.ReadClaimRoots(t.Context(), app.ID, 59)
	require.NoError(t, err)
	require.False(t, got.Ready)
	require.Contains(t, got.Reason, "epoch 2")
}

func TestClaimCheckCursorIsPrivateAndMonotonic(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	cursor, err := r.GetEventLastCheckBlock(t.Context(), app.ID, model.MonitoredEvent_ClaimAccepted)
	require.NoError(t, err)
	require.Zero(t, cursor)
	for _, block := range []uint64{9, 4, 9, 12} {
		require.NoError(t, r.UpdateEventLastCheckBlock(t.Context(), []int64{app.ID}, model.MonitoredEvent_ClaimAccepted, block))
	}
	cursor, err = r.GetEventLastCheckBlock(t.Context(), app.ID, model.MonitoredEvent_ClaimAccepted)
	require.NoError(t, err)
	require.EqualValues(t, 12, cursor)
	stored, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	raw, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "last_claim_check_block")
	require.Equal(t, app.LastInputCheckBlock, stored.LastInputCheckBlock, "claim progress does not certify input coverage")
}

func TestClaimRootsPlansBoundFutureAndValidatedHistory(t *testing.T) {
	for _, history := range []string{"future backlog", "validated prefix"} {
		t.Run(history, func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app, epoch, proof := machinePublicationEpoch(t, r, false)
			require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
			require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
			_, err := r.db.Exec(t.Context(), `INSERT INTO epoch(application_id,index,virtual_index,first_block,last_block,
 input_index_lower_bound,input_index_upper_bound,status)
 SELECT $1,i,i,i*10,i*10+9,i,i+1,'CLOSED' FROM generate_series(100,20099) i`, app.ID)
			require.NoError(t, err)
			covered := uint64(59)
			if history == "validated prefix" {
				covered = 200999
				// This synthetic history exercises the actual qualification transition.
				_, err = r.db.Exec(t.Context(), `UPDATE epoch e SET
 (machine_hash,tx_buffer_data_block,tx_buffer_proof,iflags_y_data_block,iflags_y_proof,htif_tohost_data_block,htif_tohost_proof,status)=
 (SELECT machine_hash,tx_buffer_data_block,tx_buffer_proof,iflags_y_data_block,iflags_y_proof,htif_tohost_data_block,htif_tohost_proof,
 'INPUTS_PROCESSED'::"EpochStatus" FROM epoch WHERE application_id=$1 AND index=4)
 WHERE e.application_id=$1 AND e.index>=100`, app.ID)
				require.NoError(t, err)
				_, err = r.db.Exec(t.Context(), `UPDATE epoch SET status='CLAIM_COMPUTED' WHERE application_id=$1 AND index>=100`, app.ID)
				require.NoError(t, err)
			}
			// Measure steady history. Qualification creates obsolete partial-index
			// entries; they cannot be vacuumed inside the transaction being measured.
			_, err = r.db.Exec(t.Context(), `VACUUM ANALYZE epoch`)
			require.NoError(t, err)
			_, err = r.db.Exec(t.Context(), `ANALYZE application`)
			require.NoError(t, err)
			tx, err := r.db.Begin(t.Context())
			require.NoError(t, err)
			defer rollbackDeliveryTx(tx)
			_, err = tx.Exec(t.Context(), "PREPARE claim_roots AS "+claimRootsSQL)
			require.NoError(t, err)
			defer func() { _, e := tx.Exec(t.Context(), "DEALLOCATE claim_roots"); require.NoError(t, e) }()
			for _, mode := range []string{epochCustomPlanMode, epochGenericPlanMode} {
				_, err = tx.Exec(t.Context(), "SET LOCAL plan_cache_mode="+mode)
				require.NoError(t, err)
				execute := fmt.Sprintf("EXECUTE claim_roots(%d,%d)", app.ID, covered)
				var raw []byte
				require.NoError(t, tx.QueryRow(t.Context(), "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+execute).Scan(&raw))
				require.Contains(t, string(raw), "epoch_unvalidated_idx", "the real query must use the partial prefix index")
				requireBoundedEpochPlan(t, tx, execute)
			}
		})
	}
}

func TestClaimValidatedProofSurvivesForeclosure(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch, proof := machinePublicationEpoch(t, r, false)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
	require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 80, repotest.UniqueHash(), 80))
	require.NoError(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, epoch.Index))
	_, err := r.db.Exec(t.Context(), `UPDATE epoch SET machine_hash=NULL,tx_buffer_data_block=NULL,tx_buffer_proof=NULL,
 iflags_y_data_block=NULL,iflags_y_proof=NULL,htif_tohost_data_block=NULL,htif_tohost_proof=NULL
 WHERE application_id=$1 AND index=$2`, app.ID, epoch.Index)
	require.ErrorContains(t, err, "epoch_published_validity_proof_check")
	require.True(t, claimValidated(t, r, app.ID, epoch.Index))
	roots, err := r.ReadClaimRoots(t.Context(), app.ID, 59)
	require.NoError(t, err)
	require.True(t, roots.Ready)
	require.Equal(t, proof.MachineHash, roots.MachineHash)
}

// Repository fixtures test row selection. The integration positives obtain
// authentic roots from the machine and stage their complete on-chain proof.
func TestClaimRootsSelectLatestValidatedEpochBeforeCoveredBlock(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, first, firstProof := machinePublicationEpoch(t, r, false)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, first.Index, firstProof))
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), first, nil))
	second := repotest.NewEpochBuilder(app.ID).WithIndex(5).WithBlocks(50, 59).
		WithStatus(model.EpochStatus_Closed).WithInputBounds(1, 2).Build()
	input := repotest.NewInputBuilder().WithIndex(1).WithBlockNumber(55).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*model.Epoch][]*model.Input{second: {input}}, 80))
	secondProof := repotest.DummyStateProof()
	secondProof.MachineHash, secondProof.TxBufferDataBlock = repotest.UniqueHash(), repotest.UniqueHash()
	require.NoError(t, r.StoreAdvanceResult(t.Context(), app.ID, &model.AdvanceResult{
		EpochIndex: second.Index, InputIndex: 1, Status: model.InputCompletionStatus_Accepted, StateProof: *secondProof,
	}))
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, second.Index, secondProof))
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), second, nil))
	_, err := r.db.Exec(t.Context(), `INSERT INTO epoch(application_id,index,virtual_index,first_block,last_block,
 input_index_lower_bound,input_index_upper_bound,status) VALUES($1,7,7,70,79,2,3,'CLOSED')`, app.ID)
	require.NoError(t, err)
	for _, test := range []struct {
		covered uint64
		proof   *model.StateProof
	}{
		{49, firstProof}, {59, secondProof}, {69, secondProof},
	} {
		got, readErr := r.ReadClaimRoots(t.Context(), app.ID, test.covered)
		require.NoError(t, readErr)
		require.True(t, got.Ready, "a future unvalidated row must not block this prefix")
		require.Equal(t, test.proof.MachineHash, got.MachineHash)
		require.Equal(t, test.proof.TxBufferDataBlock, got.OutputsRoot)
	}
	got, err := r.ReadClaimRoots(t.Context(), app.ID, 79)
	require.NoError(t, err)
	require.False(t, got.Ready)
	require.Equal(t, "validation/CLOSED", got.ReasonClass)
}

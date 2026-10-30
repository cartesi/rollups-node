// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// PRT's selected CLOSED object can precede the accepted-state proof publication.
// A refused drain must preserve the durable IP prefix, including after a row wait.
func TestPRTForeclosureDrainPreservesConcurrentPublication(t *testing.T) {
	for _, wait := range []bool{false, true} {
		name := "publication before drain"
		if wait {
			name = "publication while drain waits"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app, selected := seedEpochPublication(t, r)
			require.Equal(t, model.EpochStatus_Closed, selected.Status)
			require.NoError(t, r.UpdateApplicationForeclosure(ctx, app.ID, 20, repotest.UniqueHash(), 20))
			proof := repotest.DummyStateProof()
			if wait {
				publishWhileDrainWaits(ctx, t, r, app, selected, proof)
			} else {
				require.NoError(t, r.UpdateEpochInputsProcessed(ctx, app.Name, selected.Index, proof))
				require.ErrorIs(t, r.UpdateEpochWithForeclosedClaim(ctx, app.ID, selected.Index), repository.ErrNoUpdate)
			}
			stored, err := r.GetEpoch(ctx, app.Name, selected.Index)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_InputsProcessed, stored.Status)
			require.NotNil(t, stored.MachineHash)
			require.Equal(t, proof.MachineHash, *stored.MachineHash)
			current, err := r.GetApplication(ctx, app.Name)
			require.NoError(t, err)
			require.True(t, current.Enabled, "this refusal requires neither removal nor disable")
			require.Equal(t, model.ApplicationStatus_OK, current.Status)

		})
	}
}

func publishWhileDrainWaits(
	ctx context.Context, t *testing.T, r *PostgresRepository, app *model.Application, epoch *model.Epoch, proof *model.StateProof,
) {
	t.Helper()
	endpoint, err := db.GetTestDatabaseEndpoint()
	require.NoError(t, err)
	control, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	defer func() { _ = control.Close(context.Background()) }()
	tx, err := control.Begin(ctx)
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	var locked uint64
	require.NoError(t, tx.QueryRow(ctx, "SELECT index FROM epoch WHERE application_id=$1 AND index=$2 FOR UPDATE",
		app.ID, epoch.Index).Scan(&locked))
	result := make(chan error, 1)
	go func() { result <- r.UpdateEpochWithForeclosedClaim(ctx, app.ID, epoch.Index) }()
	require.Eventually(t, func() bool {
		var waiting bool
		queryErr := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
 WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)))`, control.PgConn().PID()).Scan(&waiting)
		return queryErr == nil && waiting
	}, 5*time.Second, 10*time.Millisecond)
	// Execute the production publication's atomic status/proof write inside the
	// held transaction. The drain began on CLOSED and must recheck it after IP commits.
	_, err = tx.Exec(ctx, `UPDATE epoch SET status='INPUTS_PROCESSED',machine_hash=$3,
 tx_buffer_data_block=$4,tx_buffer_proof=$5,iflags_y_data_block=$6,iflags_y_proof=$7,
 htif_tohost_data_block=$8,htif_tohost_proof=$9 WHERE application_id=$1 AND index=$2`, app.ID, epoch.Index,
		proof.MachineHash[:], proof.TxBufferDataBlock[:], encodeSiblings(proof.TxBufferProof), proof.IflagsYDataBlock[:],
		encodeSiblings(proof.IflagsYProof), proof.HtifTohostDataBlock[:], encodeSiblings(proof.HtifTohostProof))
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	select {
	case err = <-result:
		require.ErrorIs(t, err, repository.ErrNoUpdate)
	case <-ctx.Done():
		t.Fatal("drain did not return after publication committed:", ctx.Err())
	}
}

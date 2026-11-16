// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestCalculatedOutputsRootInsertRequiresComputedLifecycle(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, source, proof := machinePublicationEpoch(t, r, false)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, source.Index, proof))
	calculated := common.Hash{}
	tests := []struct {
		name      string
		status    model.EpochStatus
		validated bool
		root      *common.Hash
		reject    bool
	}{
		{"open before computation", model.EpochStatus_Open, false, nil, false},
		{"closed before computation", model.EpochStatus_Closed, false, nil, false},
		{"processed before computation", model.EpochStatus_InputsProcessed, false, nil, false},
		{"open with premature result", model.EpochStatus_Open, true, &calculated, true},
		{"closed with premature result", model.EpochStatus_Closed, true, &calculated, true},
		{"processed with premature result", model.EpochStatus_InputsProcessed, true, &calculated, true},
		{"foreclosed before computation", model.EpochStatus_ClaimForeclosed, false, nil, false},
		{"foreclosed after computation", model.EpochStatus_ClaimForeclosed, true, &calculated, false},
	}
	nextIndex := source.Index + 1
	for _, test := range tests {
		index := nextIndex
		nextIndex++
		t.Run(test.name, func(t *testing.T) {
			var root []byte
			if test.root != nil {
				root = test.root.Bytes()
			}
			// Supply complete source proofs so another CHECK cannot hide a premature result.
			_, err := r.db.Exec(t.Context(), `INSERT INTO epoch (
 application_id,index,virtual_index,first_block,last_block,input_index_lower_bound,input_index_upper_bound,
 status,claim_validated,calculated_outputs_root,machine_hash,tx_buffer_data_block,tx_buffer_proof,
 iflags_y_data_block,iflags_y_proof,htif_tohost_data_block,htif_tohost_proof)
 SELECT application_id,$1,$1,first_block,last_block,input_index_lower_bound,input_index_upper_bound,
 $2::"EpochStatus",$3,$4::hash,machine_hash,tx_buffer_data_block,tx_buffer_proof,
 iflags_y_data_block,iflags_y_proof,htif_tohost_data_block,htif_tohost_proof
 FROM epoch WHERE application_id=$5 AND index=$6`, index, test.status.String(), test.validated, root, app.ID, source.Index)
			if test.reject {
				var failure *pgconn.PgError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, "23514", failure.Code)
				require.Equal(t, "epoch_calculated_outputs_root_check", failure.ConstraintName)
				var count int
				require.NoError(t, r.db.QueryRow(t.Context(),
					`SELECT count(*) FROM epoch WHERE application_id=$1 AND index=$2`, app.ID, index).Scan(&count))
				require.Zero(t, count, "a rejected INSERT must leave no epoch")
				return
			}
			require.NoError(t, err)
			stored, err := r.GetEpoch(t.Context(), app.Name, index)
			require.NoError(t, err)
			require.Equal(t, test.status, stored.Status)
			require.Equal(t, test.root, stored.CalculatedOutputsRoot)
			require.Equal(t, test.validated, claimValidated(t, r, app.ID, index))
		})
	}
}

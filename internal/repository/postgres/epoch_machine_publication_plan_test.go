// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

const (
	epochCustomPlanMode  = "force_custom_plan"
	epochGenericPlanMode = "force_generic_plan"
)

type epochQueryPlan struct {
	NodeType    string           `json:"Node Type"`
	IndexName   string           `json:"Index Name"`
	Rows        float64          `json:"Actual Rows"`
	Loops       float64          `json:"Actual Loops"`
	Filtered    float64          `json:"Rows Removed by Filter"`
	JoinRemoved float64          `json:"Rows Removed by Join Filter"`
	HitBlocks   float64          `json:"Shared Hit Blocks"`
	ReadBlocks  float64          `json:"Shared Read Blocks"`
	Plans       []epochQueryPlan `json:"Plans"`
}

// Count successful scan/sort input as well as rejected rows, including repeated
// subplans. A bitmap scan feeding every claim to a Sort must not pass this gate.
func (p epochQueryPlan) rowWork() float64 {
	total := (p.Rows + p.Filtered + p.JoinRemoved) * p.Loops
	for _, child := range p.Plans {
		total += child.rowWork()
	}
	return total
}

func requireBoundedEpochPlan(t *testing.T, tx pgx.Tx, execute string) {
	t.Helper()
	var raw []byte
	require.NoError(t, tx.QueryRow(t.Context(), "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+execute).Scan(&raw))
	var plans []struct{ Plan epochQueryPlan }
	require.NoError(t, json.Unmarshal(raw, &plans))
	require.Len(t, plans, 1)
	plan := plans[0].Plan
	require.Less(t, plan.rowWork(), float64(256), "bounded rows for %s: %s", execute, raw)
	require.Less(t, plan.HitBlocks+plan.ReadBlocks, float64(256), "bounded buffers for %s: %s", execute, raw)
	t.Logf("%s rows=%g buffers=%g plan=%s", execute, plan.rowWork(), plan.HitBlocks+plan.ReadBlocks, raw)
}

func TestMachinePublicationGateWithSharedEpochBacklogs(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	published, epoch, proof := machinePublicationEpoch(t, r, false)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), published.Name, epoch.Index, proof))
	terminal := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	closed := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	for _, app := range []*model.Application{published, terminal, closed} {
		require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 400000, repotest.UniqueHash(), 400000))
	}
	_, err := r.db.Exec(t.Context(), `INSERT INTO epoch(application_id,index,virtual_index,
 first_block,last_block,input_index_lower_bound,input_index_upper_bound,machine_hash,tx_buffer_data_block,
 tx_buffer_proof,iflags_y_data_block,iflags_y_proof,htif_tohost_data_block,htif_tohost_proof,status)
 SELECT application_id,i,i,i*10,i*10+9,i,i+1,machine_hash,tx_buffer_data_block,tx_buffer_proof,
 iflags_y_data_block,iflags_y_proof,htif_tohost_data_block,htif_tohost_proof,'INPUTS_PROCESSED'
 FROM epoch CROSS JOIN generate_series(100,20099) i WHERE application_id=$1 AND index=4`, published.ID)
	require.NoError(t, err)
	for _, app := range []*model.Application{terminal, closed} {
		status := model.EpochStatus_ClaimForeclosed
		if app == closed {
			status = model.EpochStatus_Closed
		}
		_, err = r.db.Exec(t.Context(), `INSERT INTO epoch(application_id,index,virtual_index,
 first_block,last_block,input_index_lower_bound,input_index_upper_bound,status)
 SELECT $1,i,i,i*10,i*10+9,i,i+1,$2::"EpochStatus" FROM generate_series(100,20099) i`, app.ID, status.String())
		require.NoError(t, err)
	}
	// A caught-up A/Q layout can retain one OPEN epoch at the foreclosure
	// boundary. It must not force a walk over all earlier durable IP rows.
	_, err = r.db.Exec(t.Context(), `INSERT INTO epoch(application_id,index,virtual_index,
 first_block,last_block,input_index_lower_bound,input_index_upper_bound,status)
 VALUES ($1,40000,40000,400000,400009,20001,20002,'OPEN')`, published.ID)
	require.NoError(t, err)
	for range 100 {
		peer := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
		_, err = r.db.Exec(t.Context(), `INSERT INTO epoch(application_id,index,virtual_index,
 first_block,last_block,input_index_lower_bound,input_index_upper_bound,status)
 SELECT $1,i,i,i*10,i*10+9,i,i+1,'CLAIM_FORECLOSED' FROM generate_series(0,99) i`, peer.ID)
		require.NoError(t, err)
	}
	_, err = r.db.Exec(t.Context(), "ANALYZE epoch; ANALYZE application")
	require.NoError(t, err)
	wantPresence := make(map[int64]bool, 3)
	for _, app := range []*model.Application{published, terminal, closed} {
		got, lookupErr := r.HasUnpublishedEpochBeforeBlock(t.Context(), app.ID, 400000)
		require.NoError(t, lookupErr)
		require.Equal(t, app == closed, got)
		wantPresence[app.ID] = got
	}
	tx, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	// Prepare one shape; each application supplies its actual Sql() arguments below.
	query, _ := unpublishedEpochBeforeBlockSQL(published.ID, 400000)
	_, err = tx.Exec(t.Context(), "PREPARE machine_publication AS "+query)
	require.NoError(t, err)
	defer func() {
		_, cleanupErr := tx.Exec(t.Context(), "DEALLOCATE machine_publication")
		require.NoError(t, cleanupErr)
	}()
	for _, mode := range []string{epochCustomPlanMode, epochGenericPlanMode} {
		_, err = tx.Exec(t.Context(), "SET LOCAL plan_cache_mode="+mode)
		require.NoError(t, err)
		for _, app := range []*model.Application{published, terminal, closed} {
			appQuery, args := unpublishedEpochBeforeBlockSQL(app.ID, 400000)
			require.Equal(t, query, appQuery, "prepared shape must match the real query")
			execute := machinePublicationExecute(t, args)
			t.Run(mode+"/"+app.Name, func(t *testing.T) {
				rows, queryErr := tx.Query(t.Context(), execute)
				require.NoError(t, queryErr)
				present := rows.Next()
				rows.Close()
				require.NoError(t, rows.Err())
				require.Equal(t, wantPresence[app.ID], present, "EXPLAIN must measure the real publication predicate")
				requireBoundedEpochPlan(t, tx, execute)
			})
		}
	}
}

// These are the parameter types emitted by unpublishedEpochBeforeBlockSQL.
// Values and their order come from Sql(), including after harmless query edits.
func machinePublicationExecute(t *testing.T, args []any) string {
	t.Helper()
	values := make([]string, len(args))
	for i, arg := range args {
		switch value := arg.(type) {
		case int64:
			values[i] = strconv.FormatInt(value, 10)
		case bool:
			values[i] = strconv.FormatBool(value)
		case string:
			values[i] = "E'" + strings.NewReplacer("\\", "\\\\", "'", "''").Replace(value) + "'"
		default:
			t.Fatalf("unexpected publication parameter type %T", arg)
		}
	}
	return "EXECUTE machine_publication(" + strings.Join(values, ",") + ")"
}

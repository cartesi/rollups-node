// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type disputeSchemaFixture struct {
	repo       *PostgresRepository
	appID      int64
	tournament common.Address
	one        common.Hash
	two        common.Hash
	match      common.Hash
}

func newDisputeSchemaFixture(t *testing.T) disputeSchemaFixture {
	t.Helper()
	repo := newEpochPublicationRepository(t).(*PostgresRepository)
	app, _ := seedEpochPublication(t, repo)
	f := disputeSchemaFixture{repo: repo, appID: app.ID, tournament: common.Address{1},
		one: common.Hash{2}, two: common.Hash{3}, match: common.Hash{4}}
	_, err := repo.db.Exec(t.Context(), `INSERT INTO tournaments
		(application_id,epoch_index,address,max_level,level,log2step,height,initial_hash,base_cycle,kind,
		 start_instant,allowance,bond_value,creation_block_number,creation_tx_hash,creation_log_index,
		 as_of_block,standing,accepts_joins,winner_expires_at,bond_disposition)
		VALUES ($1,0,$2,2,0,38,54,$3,0,'NON_LEAF',10,20,1,NULL,NULL,NULL,20,'MATCHES_ACTIVE',true,0,'TOURNAMENT_RUNNING')`,
		app.ID, f.tournament.Bytes(), common.Hash{}.Bytes())
	require.NoError(t, err)
	for i, commitment := range []common.Hash{f.one, f.two} {
		_, err = repo.db.Exec(t.Context(), `INSERT INTO commitments
			(application_id,epoch_index,tournament_address,commitment,final_state_hash,submitter_address,
			 block_number,tx_hash,log_index,as_of_block,claimer,clock_running,clock_deadline,clock_allowance)
			VALUES ($1,0,$2,$3,$4,$5,10,$6,$7,20,$5,false,0,5)`, app.ID, f.tournament.Bytes(),
			commitment.Bytes(), common.Hash{}.Bytes(), app.IApplicationAddress.Bytes(), common.Hash{5}.Bytes(), i+1)
		require.NoError(t, err)
	}
	_, err = repo.db.Exec(t.Context(), `INSERT INTO matches
		(application_id,epoch_index,tournament_address,id_hash,commitment_one,commitment_two,left_of_two,
		 block_number,tx_hash,log_index,eliminable_at,responder_deadline,as_of_block,phase,timeout_outcome,
		 deferred_charge,revealing_parent,waiting_left,waiting_right,segment_start_position,segment_start_cycle,
		 current_height,responder,winner,deletion_reason)
		VALUES ($1,0,$2,$3,$4,$5,$4,11,$6,3,30,20,20,'BISECTING','NONE',0,$4,$4,$5,0,0,54,'ONE','NONE','NOT_DELETED')`,
		app.ID, f.tournament.Bytes(), f.match.Bytes(), f.one.Bytes(), f.two.Bytes(), common.Hash{5}.Bytes())
	require.NoError(t, err)
	return f
}

func requireDisputeSchemaError(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, code, pgErr.Code)
	if constraint != "" {
		require.Equal(t, constraint, pgErr.ConstraintName)
	}
}

func TestDisputeSchemaTournamentFacts(t *testing.T) {
	const creationEventConstraint = "tournaments_creation_event_check"
	f := newDisputeSchemaFixture(t)
	for _, test := range []struct {
		name, assignment, code, constraint string
	}{
		{"positive bond", "bond_value=0", "23514", "tournaments_bond_value_check"},
		{"required bond", "bond_value=NULL", "23502", ""},
		{"root contested states", "contested_final_state_one=initial_hash,contested_final_state_two=initial_hash",
			"23514", "tournaments_nested_dispute_check"},
		{"root creation block", "creation_block_number=10", "23514", creationEventConstraint},
		{"root creation transaction", "creation_tx_hash=initial_hash", "23514", creationEventConstraint},
		{"root creation index", "creation_log_index=0", "23514", creationEventConstraint},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.repo.db.Exec(t.Context(), "UPDATE tournaments SET "+test.assignment+" WHERE application_id=$1", f.appID)
			requireDisputeSchemaError(t, err, test.code, test.constraint)
		})
	}
	child := common.Address{6}
	insert := `INSERT INTO tournaments
		(application_id,epoch_index,address,parent_tournament_address,parent_match_id_hash,max_level,level,
		 log2step,height,initial_hash,base_cycle,kind,start_instant,allowance,bond_value,
		 contested_final_state_one,contested_final_state_two,creation_block_number,creation_tx_hash,creation_log_index,
		 as_of_block,standing,accepts_joins,winner_expires_at,inner_disposition,inner_paused_allowance,bond_disposition)
		VALUES ($1,0,$2,$3,$4,2,1,0,38,$5,0,'LEAF',12,20,1,$6,$7,12,$8,4,20,
		 'AWAITING_CLOSURE',false,0,'UNSETTLED',0,'TOURNAMENT_RUNNING')`
	_, err := f.repo.db.Exec(t.Context(), insert, f.appID, child.Bytes(), f.tournament.Bytes(), f.match.Bytes(),
		common.Hash{}.Bytes(), nil, nil, common.Hash{5}.Bytes())
	requireDisputeSchemaError(t, err, "23514", "tournaments_nested_dispute_check")
	_, err = f.repo.db.Exec(t.Context(), insert, f.appID, child.Bytes(), f.tournament.Bytes(), f.match.Bytes(),
		common.Hash{}.Bytes(), common.Hash{}.Bytes(), common.Hash{}.Bytes(), common.Hash{5}.Bytes())
	require.NoError(t, err, "present zero participant states are valid")
	for _, test := range []struct{ name, assignment, constraint string }{
		{"wrong creation block", "creation_block_number=11", "tournaments_creation_block_check"},
		{"missing creation block", "creation_block_number=NULL", creationEventConstraint},
		{"missing creation transaction", "creation_tx_hash=NULL", creationEventConstraint},
		{"missing creation index", "creation_log_index=NULL", creationEventConstraint},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.repo.db.Exec(t.Context(), "UPDATE tournaments SET "+test.assignment+" WHERE application_id=$1 AND address=$2",
				f.appID, child.Bytes())
			requireDisputeSchemaError(t, err, "23514", test.constraint)
		})
	}

	_, err = f.repo.db.Exec(t.Context(), `UPDATE tournaments SET contested_final_state_one=NULL
		WHERE application_id=$1 AND address=$2`, f.appID, child.Bytes())
	requireDisputeSchemaError(t, err, "23514", "tournaments_nested_dispute_check")
	_, err = f.repo.db.Exec(t.Context(), `UPDATE tournaments SET contested_final_state_two=NULL
		WHERE application_id=$1 AND address=$2`, f.appID, child.Bytes())
	requireDisputeSchemaError(t, err, "23514", "tournaments_nested_dispute_check")
}

func TestDisputeSchemaMatchSchedulesAndSeal(t *testing.T) {
	f := newDisputeSchemaFixture(t)
	for _, test := range []struct {
		name, assignment, code, constraint string
	}{
		{"required deadline", "responder_deadline=NULL", "23502", ""},
		{"creation schedule", "responder_deadline=31", "23514", "matches_creation_schedule_check"},
		{"partial seal", "seal_eliminable_at=30", "23514", "matches_seal_tuple_check"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.repo.db.Exec(t.Context(), "UPDATE matches SET "+test.assignment+" WHERE application_id=$1", f.appID)
			requireDisputeSchemaError(t, err, test.code, test.constraint)
		})
	}
	_, err := f.repo.db.Exec(t.Context(), `UPDATE matches SET responder_deadline=eliminable_at WHERE application_id=$1`, f.appID)
	require.NoError(t, err)
	_, err = f.repo.db.Exec(t.Context(), `UPDATE matches SET seal_eliminable_at=30,seal_agree_state=$2,
		seal_divergence_position=2,seal_final_state_one=$3,seal_final_state_two=$4,
		seal_deadline_one=20,seal_deadline_two=30,seal_block_number=15,seal_tx_hash=$5,seal_log_index=5
		WHERE application_id=$1`, f.appID, common.Hash{}.Bytes(), f.one.Bytes(), f.two.Bytes(), common.Hash{5}.Bytes())
	require.NoError(t, err)
	_, err = f.repo.db.Exec(t.Context(), `UPDATE matches SET seal_eliminable_at=29 WHERE application_id=$1`, f.appID)
	requireDisputeSchemaError(t, err, "23514", "matches_seal_schedule_check")
	_, err = f.repo.db.Exec(t.Context(), `UPDATE matches SET seal_deadline_one=40,seal_eliminable_at=40
		WHERE application_id=$1`, f.appID)
	require.NoError(t, err)
	_, err = f.repo.db.Exec(t.Context(), `UPDATE matches SET seal_eliminable_at=30 WHERE application_id=$1`, f.appID)
	requireDisputeSchemaError(t, err, "23514", "matches_seal_schedule_check")
	_, err = f.repo.db.Exec(t.Context(), `UPDATE matches SET phase='SEALED',revealing_parent=NULL,waiting_left=NULL,
		waiting_right=NULL,segment_start_position=NULL,segment_start_cycle=NULL,current_height=NULL,responder=NULL,
		agree_state=seal_agree_state,divergence_position=seal_divergence_position,divergence_cycle=2,
		final_state_one=seal_final_state_one,final_state_two=seal_final_state_two WHERE application_id=$1`, f.appID)
	require.NoError(t, err)
	for _, field := range []string{"agree_state", "divergence_position", "final_state_one", "final_state_two"} {
		t.Run("sealed view "+field, func(t *testing.T) {
			assignment := field + "=$2"
			var value any = common.Hash{9}.Bytes()
			if field == "divergence_position" {
				value = 3
			}
			_, err := f.repo.db.Exec(t.Context(), "UPDATE matches SET "+assignment+" WHERE application_id=$1", f.appID, value)
			requireDisputeSchemaError(t, err, "23514", "matches_sealed_view_check")
		})
	}
	_, err = f.repo.db.Exec(t.Context(), `UPDATE matches SET seal_deadline_two=NULL WHERE application_id=$1`, f.appID)
	requireDisputeSchemaError(t, err, "23514", "matches_seal_tuple_check")
}

func TestDisputeSchemaAdvanceHeightAndSchedule(t *testing.T) {
	f := newDisputeSchemaFixture(t)
	insert := `INSERT INTO match_advances
		(application_id,epoch_index,tournament_address,id_hash,other_parent,left_node,segment_start_position,
		 eliminable_at,current_height,responder_deadline,block_number,tx_hash,log_index)
		VALUES ($1,0,$2,$3,$4,$5,0,30,$6,$7,12,$8,$9)
		ON CONFLICT (application_id,tx_hash,log_index) DO NOTHING`
	for _, test := range []struct {
		name             string
		height, deadline any
		code, constraint string
	}{
		{"zero height", 0, 20, "23514", "match_advances_height_check"},
		{"required height", nil, 20, "23502", ""},
		{"required deadline", 53, nil, "23502", ""},
		{"schedule", 53, 31, "23514", "match_advances_schedule_check"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.repo.db.Exec(t.Context(), insert, f.appID, f.tournament.Bytes(), f.match.Bytes(), f.one.Bytes(),
				f.two.Bytes(), test.height, test.deadline, common.Hash{5}.Bytes(), 6)
			requireDisputeSchemaError(t, err, test.code, test.constraint)
		})
	}
	for i, height := range []int{53, 52, 1} {
		deadline := 20
		if height == 1 {
			deadline = 30
		}
		_, err := f.repo.db.Exec(t.Context(), insert, f.appID, f.tournament.Bytes(), f.match.Bytes(), f.one.Bytes(),
			f.two.Bytes(), height, deadline, common.Hash{5}.Bytes(), i+6)
		require.NoError(t, err)
	}
	_, err := f.repo.db.Exec(t.Context(), insert, f.appID, f.tournament.Bytes(), f.match.Bytes(), f.one.Bytes(),
		f.two.Bytes(), 53, 20, common.Hash{5}.Bytes(), 9)
	requireDisputeSchemaError(t, err, "23505", "match_advances_height_key")
}

func TestDisputeSchemaStandingFacts(t *testing.T) {
	const liveMatchesCheck = "standing_changes_live_matches_check"
	const winnerCheck = "standing_changes_winner_check"
	f := newDisputeSchemaFixture(t)
	insert := `INSERT INTO standing_changes
		(application_id,epoch_index,tournament_address,match_count,dangling,parent_commitment,result_at,
		 winner_expires_at,block_number,tx_hash,log_index) VALUES ($1,0,$2,$3,$4,$5,$6,$7,15,$8,9)`
	for _, test := range []struct {
		name             string
		count            int
		dangling, parent any
		result, expires  int
		constraint       string
	}{
		{name: "live result", count: 1, result: 20, constraint: liveMatchesCheck},
		{name: "live parent", count: 1, dangling: f.one.Bytes(), parent: f.two.Bytes(), expires: 20,
			constraint: liveMatchesCheck},
		{name: "live expiration", count: 1, expires: 20, constraint: liveMatchesCheck},
		{name: "missing result", constraint: "standing_changes_result_check"},
		{name: "expiration without parent", result: 20, expires: 30, constraint: winnerCheck},
		{name: "winner without dangling", parent: f.two.Bytes(), result: 20, expires: 30,
			constraint: winnerCheck},
		{name: "winner expiration", dangling: f.one.Bytes(), parent: f.two.Bytes(), result: 20, expires: 19,
			constraint: winnerCheck},
		{name: "zero dangling reference", count: 1, dangling: common.Hash{}.Bytes(),
			constraint: "standing_changes_dangling_fkey"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.repo.db.Exec(t.Context(), insert, f.appID, f.tournament.Bytes(), test.count, test.dangling,
				test.parent, test.result, test.expires, common.Hash{5}.Bytes())
			code := "23514"
			if test.constraint == "standing_changes_dangling_fkey" {
				code = "23503"
			}
			requireDisputeSchemaError(t, err, code, test.constraint)
		})
	}
	_, err := f.repo.db.Exec(t.Context(), insert, f.appID, common.Address{9}.Bytes(), 1, nil, nil, 0, 0,
		common.Hash{5}.Bytes())
	requireDisputeSchemaError(t, err, "23503", "standing_changes_tournament_fkey")
	_, err = f.repo.db.Exec(t.Context(), insert, f.appID, f.tournament.Bytes(), 1, f.one.Bytes(), nil, 0, 0,
		common.Hash{5}.Bytes())
	require.NoError(t, err)
	_, err = f.repo.db.Exec(t.Context(), insert, f.appID, f.tournament.Bytes(), 1, f.one.Bytes(), nil, 0, 0,
		common.Hash{5}.Bytes())
	requireDisputeSchemaError(t, err, "23505", "standing_changes_pkey")
	var before, after string
	require.NoError(t, f.repo.db.QueryRow(t.Context(), `SELECT updated_at::text FROM standing_changes
		WHERE application_id=$1`, f.appID).Scan(&before))
	_, err = f.repo.db.Exec(t.Context(), `UPDATE standing_changes SET block_number=16 WHERE application_id=$1`, f.appID)
	require.NoError(t, err)
	require.NoError(t, f.repo.db.QueryRow(t.Context(), `SELECT updated_at::text FROM standing_changes
		WHERE application_id=$1`, f.appID).Scan(&after))
	require.NotEqual(t, before, after)
	_, err = f.repo.db.Exec(t.Context(), `UPDATE standing_changes SET match_count=0,dangling=NULL,result_at=20
		WHERE application_id=$1`, f.appID)
	require.NoError(t, err)
	_, err = f.repo.db.Exec(t.Context(), `UPDATE standing_changes SET dangling=$2,parent_commitment=$3,winner_expires_at=20
		WHERE application_id=$1`, f.appID, f.one.Bytes(), f.two.Bytes())
	require.NoError(t, err)
	_, err = f.repo.db.Exec(t.Context(), `DELETE FROM commitments WHERE application_id=$1 AND commitment=$2`, f.appID, f.one.Bytes())
	require.NoError(t, err)
	var count int
	require.NoError(t, f.repo.db.QueryRow(t.Context(), `SELECT count(*) FROM standing_changes
		WHERE application_id=$1`, f.appID).Scan(&count))
	require.Zero(t, count)
}

func TestDisputeSchemaMigrationRoundTrip(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skip(err)
	}
	migration, err := schema.New(endpoint)
	require.NoError(t, err)
	t.Cleanup(migration.Close)
	t.Cleanup(func() { require.NoError(t, migration.Upgrade()) })
	conn, err := pgx.Connect(t.Context(), endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(context.Background())) })
	for _, omitNewTable := range []bool{false, true} {
		t.Run(fmt.Sprintf("new table absent %t", omitNewTable), func(t *testing.T) {
			require.NoError(t, migration.Upgrade())
			if omitNewTable {
				_, err := conn.Exec(t.Context(), `DROP TABLE standing_changes`)
				require.NoError(t, err)
			}
			require.NoError(t, migration.Downgrade())
			for _, query := range []string{
				`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
				 WHERE n.nspname='public' AND c.relname NOT IN ('schema_migrations','schema_migrations_pkey')`,
				`SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public'`,
				`SELECT count(*) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid
				 JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND NOT t.tgisinternal`,
			} {
				var count int
				require.NoError(t, conn.QueryRow(t.Context(), query).Scan(&count))
				require.Zero(t, count, query)
			}
		})
	}
}

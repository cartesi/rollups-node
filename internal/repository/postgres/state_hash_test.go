// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"math"
	"strconv"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/stretchr/testify/require"
)

func TestUint64ExprBindsUint64(t *testing.T) {
	t.Parallel()
	var expectedSQL string
	for _, value := range []uint64{0, 1, math.MaxInt64 + 1, math.MaxUint64} {
		query, args := postgres.SELECT(uint64Expr(value)).Sql()
		if expectedSQL == "" {
			expectedSQL = query
		}
		require.Equal(t, expectedSQL, query)
		require.Equal(t, []any{strconv.FormatUint(value, 10)}, args)
	}
}

func TestPostgresEpochStateHashLargeOrdinals(t *testing.T) {
	repo := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch := seedEpochPublication(t, repo)
	hash := repotest.UniqueHash()
	indexes := []uint64{0, math.MaxInt64 + 17, math.MaxUint64}
	for _, index := range indexes {
		_, err := repo.db.Exec(t.Context(), `INSERT INTO state_hashes
			(input_epoch_application_id, epoch_index, input_index, index, machine_hash, repetitions)
			VALUES ($1, $2, 0, CAST($3 AS NUMERIC), $4, 1)`,
			app.ID, epoch.Index, strconv.FormatUint(index, 10), hash.Bytes())
		require.NoError(t, err)
	}
	count, err := repo.CountEpochStateHashes(t.Context(), app.ID, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, uint64(len(indexes)), count)
	page, err := repo.ListEpochStateHashes(t.Context(), app.ID, epoch.Index, nil, 3)
	require.NoError(t, err)
	require.Len(t, page, len(indexes))
	for position, index := range indexes {
		require.Equal(t, index, page[position].Index)
		require.Equal(t, hash, page[position].MachineHash)
	}
	page, err = repo.ListEpochStateHashes(t.Context(), app.ID, epoch.Index, &indexes[1], 3)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, uint64(math.MaxUint64), page[0].Index)
	page, err = repo.ListEpochStateHashes(t.Context(), app.ID, epoch.Index, &indexes[2], 3)
	require.NoError(t, err)
	require.Empty(t, page)
	for _, absentEpoch := range []uint64{math.MaxInt64 + 1, math.MaxUint64} {
		count, err := repo.CountEpochStateHashes(t.Context(), app.ID, absentEpoch)
		require.NoError(t, err)
		require.Zero(t, count)
		page, err := repo.ListEpochStateHashes(t.Context(), app.ID, absentEpoch, nil, 3)
		require.NoError(t, err)
		require.Empty(t, page)
	}
}

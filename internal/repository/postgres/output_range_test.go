// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const (
	outputScanCallbackEvent = "callback"
	outputScanNextEvent     = "next"
	outputScanReadEvent     = "scan"
	outputScanCloseEvent    = "close"
)

type outputScanRows struct {
	pgx.Rows
	indexes []uint64
	data    [][]byte
	events  []string
	current int
	nexts   int
	closed  bool
	scanErr error
	rowsErr error
}

func (r *outputScanRows) Next() bool {
	r.events = append(r.events, outputScanNextEvent)
	r.current = r.nexts
	r.nexts++
	return r.current < len(r.indexes)
}

func (r *outputScanRows) Scan(destinations ...any) error {
	r.events = append(r.events, outputScanReadEvent)
	if r.scanErr != nil {
		return r.scanErr
	}
	*destinations[0].(*uint64) = r.indexes[r.current]
	*destinations[1].(*[]byte) = append([]byte(nil), r.data[r.current]...)
	return nil
}

func (r *outputScanRows) Close() {
	r.events = append(r.events, outputScanCloseEvent)
	r.closed = true
}

func (r *outputScanRows) Err() error { return r.rowsErr }

type outputScanQuery struct {
	rows pgx.Rows
	err  error
}

func (q *outputScanQuery) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	return q.rows, q.err
}

func TestScanOutputsStreamsBeforeNext(t *testing.T) {
	t.Parallel()
	rows := &outputScanRows{indexes: []uint64{10, 11}, data: [][]byte{[]byte("first"), []byte("second")}}
	query := &outputScanQuery{rows: rows}
	var indexes []uint64
	err := scanOutputs(t.Context(), query, 7, 10, 11, func(index uint64, rawData []byte) error {
		rows.events = append(rows.events, outputScanCallbackEvent)
		require.Equal(t, rows.data[len(indexes)], rawData)
		indexes = append(indexes, index)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, rows.indexes, indexes)
	require.Equal(t, []string{
		outputScanNextEvent, outputScanReadEvent, outputScanCallbackEvent,
		outputScanNextEvent, outputScanReadEvent, outputScanCallbackEvent,
		outputScanNextEvent, outputScanCloseEvent,
	}, rows.events)
	require.True(t, rows.closed)
}

func TestScanOutputsCallbackErrorStopsAndCloses(t *testing.T) {
	t.Parallel()
	rows := &outputScanRows{indexes: []uint64{10, 11}, data: [][]byte{[]byte("first"), []byte("second")}}
	want := errors.New("callback failed")
	err := scanOutputs(t.Context(), &outputScanQuery{rows: rows}, 7, 10, 11, func(uint64, []byte) error {
		rows.events = append(rows.events, outputScanCallbackEvent)
		return want
	})
	require.ErrorIs(t, err, want)
	require.Equal(t, []string{outputScanNextEvent, outputScanReadEvent, outputScanCallbackEvent, outputScanCloseEvent}, rows.events)
	require.True(t, rows.closed)
}

func TestScanOutputsPropagatesReadErrors(t *testing.T) {
	t.Parallel()
	want := errors.New("read failed")
	t.Run("query", func(t *testing.T) {
		err := scanOutputs(t.Context(), &outputScanQuery{err: want}, 7, 0, 0, func(uint64, []byte) error { return nil })
		require.ErrorIs(t, err, want)
	})
	for _, scanError := range []bool{true, false} {
		rows := &outputScanRows{}
		if scanError {
			rows.indexes, rows.scanErr = []uint64{0}, want
		} else {
			rows.rowsErr = want
		}
		err := scanOutputs(t.Context(), &outputScanQuery{rows: rows}, 7, 0, 0, func(uint64, []byte) error {
			t.Fatal("callback must not run after a read error")
			return nil
		})
		require.ErrorIs(t, err, want)
		require.True(t, rows.closed)
	}
}

func TestPostgresOutputsRefuseIdentityOrRawDataUpdate(t *testing.T) {
	repo := newEpochPublicationRepository(t).(*PostgresRepository)
	app, _ := seedEpochPublication(t, repo)
	for _, column := range []string{"input_epoch_application_id", "input_index", "index", "raw_data"} {
		identifier := pgx.Identifier{column}.Sanitize()
		_, err := repo.db.Exec(t.Context(), "UPDATE output SET "+identifier+" = "+identifier+
			" WHERE input_epoch_application_id = $1", app.ID)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		require.Equal(t, "23001", pgErr.Code)
		require.Equal(t, "output identity or raw data is immutable", pgErr.Message)
	}
	hash := repotest.UniqueHash()
	_, err := repo.db.Exec(t.Context(), `UPDATE output SET hash = $2 WHERE input_epoch_application_id = $1`, app.ID, hash.Bytes())
	require.NoError(t, err)
	siblings := make([][]byte, 63)
	for index := range siblings {
		siblings[index] = hash.Bytes()
	}
	_, err = repo.db.Exec(t.Context(), `UPDATE output SET output_hashes_siblings = $2
		WHERE input_epoch_application_id = $1`, app.ID, siblings)
	require.NoError(t, err)
	_, err = repo.db.Exec(t.Context(), `UPDATE output SET execution_transaction_hash = $2
		WHERE input_epoch_application_id = $1`, app.ID, hash.Bytes())
	require.NoError(t, err)
}

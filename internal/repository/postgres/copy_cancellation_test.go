// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
)

func TestNormalizeCopyError(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	deadline, stopDeadline := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stopDeadline()
	const network = "tcp"
	closed := &net.OpError{Op: "read", Net: network, Err: net.ErrClosed}
	timeout := &net.OpError{Op: "write", Net: network, Err: os.ErrDeadlineExceeded}
	constraint := &pgconn.PgError{Code: "23502", Message: "null raw_data"}
	for _, tc := range []struct {
		name       string
		ctx        context.Context
		err        error
		normalized bool
	}{
		{"success", canceled, nil, false},
		{"cancellation", canceled, context.Canceled, false},
		{"closed socket during shutdown", canceled, closed, true},
		{"wrapped closed socket", canceled, fmt.Errorf("copy: %w", closed), true},
		{"timeout during shutdown", canceled, timeout, true},
		{"closed socket on live context", t.Context(), closed, false},
		{"timeout on live context", t.Context(), timeout, false},
		{"closed socket on deadline", deadline, closed, false},
		{"context deadline during shutdown", canceled, context.DeadlineExceeded, false},
		{"server error during shutdown", canceled, constraint, false},
		{"driver connection already closed", canceled, pgconn.ErrConnClosed, false},
		{"encoding error during shutdown", canceled, errors.New("cannot encode row"), false},
		{"peer EOF during shutdown", canceled, io.EOF, false},
		{"peer reset during shutdown", canceled, &net.OpError{Op: "read", Net: network, Err: syscall.ECONNRESET}, false},
		{"peer timeout during shutdown", canceled, &net.OpError{Op: "write", Net: network, Err: syscall.ETIMEDOUT}, false},
		{"mixed server error", canceled, errors.Join(closed, constraint), false},
		{"mixed peer error", canceled, fmt.Errorf("copy: %w", errors.Join(closed, io.EOF)), false},
		{"mixed cancellation", canceled, errors.Join(closed, context.Canceled), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeCopyError(tc.ctx, tc.err)
			if !tc.normalized {
				require.Equal(t, tc.err, got, "independent failures must keep their causes")
				return
			}
			require.ErrorIs(t, got, context.Canceled)
			require.True(t, errutil.IsOnlyCancellation(got))
			require.ErrorContains(t, got, tc.err.Error(), "keep the transport diagnostic")
		})
	}
}

func TestAdvanceCopyCancellationCause(t *testing.T) {
	for _, tc := range []struct {
		name   string
		insert func(context.Context, pgx.Tx) error
	}{
		{table.Output.TableName(), func(ctx context.Context, tx pgx.Tx) error {
			return insertOutputs(ctx, tx, 1, 0, [][]byte{[]byte("output")})
		}},
		{table.Report.TableName(), func(ctx context.Context, tx pgx.Tx) error {
			return insertReports(ctx, tx, 1, 0, [][]byte{[]byte("report")})
		}},
		{table.StateHashes.TableName(), func(ctx context.Context, tx pgx.Tx) error {
			return insertStateHashes(ctx, tx, 1, 0, 0, nil, repotest.UniqueHash(), model.InputHashCollectionCapacity)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err := tc.insert(ctx, &canceledCopyTx{cancel: cancel})
			require.ErrorIs(t, err, context.Canceled)
			require.True(t, errutil.IsOnlyCancellation(err))
			require.ErrorContains(t, err, net.ErrClosed.Error())
		})
	}
}

// canceledCopyTx returns the closed-socket variant of a canceled COPY,
// independently of pgx's reader/writer scheduling in the live database test.
type canceledCopyTx struct {
	pgx.Tx
	cancel context.CancelFunc
}

func (tx *canceledCopyTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	tx.cancel()
	return 0, fmt.Errorf("COPY socket: %w", net.ErrClosed)
}

func (tx *canceledCopyTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return copyIndexRow{}
}

type copyIndexRow struct{}

func (copyIndexRow) Scan(dest ...any) error {
	*dest[0].(*uint64) = 0
	return nil
}

func TestStoreAdvanceResultCancellationDuringCopy(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	for _, tableName := range []string{table.Output.TableName(), table.Report.TableName()} {
		t.Run(tableName, func(t *testing.T) {
			require.NoError(t, db.SetupTestPostgres(endpoint))
			ctx := t.Context()
			repo, err := NewPostgresRepository(ctx, endpoint, 1, 0)
			require.NoError(t, err)
			t.Cleanup(repo.Close)
			control, err := pgx.Connect(ctx, endpoint)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, control.Close(context.Background())) })
			seed := repotest.Seed(ctx, t, repo)
			const copyLock = 739107
			_, err = control.Exec(ctx, `CREATE FUNCTION pause_advance_copy() RETURNS trigger AS $$
				BEGIN
					PERFORM pg_advisory_xact_lock(739107);
					RETURN NEW;
				END;
				$$ LANGUAGE plpgsql`)
			require.NoError(t, err)
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := control.Exec(cleanupCtx, "DROP FUNCTION pause_advance_copy() CASCADE")
				require.NoError(t, err)
			})
			_, err = control.Exec(ctx, "CREATE TRIGGER pause_advance_copy BEFORE INSERT ON "+tableName+
				" FOR EACH ROW EXECUTE FUNCTION pause_advance_copy()")
			require.NoError(t, err)
			_, err = control.Exec(ctx, "SELECT pg_advisory_lock($1)", copyLock)
			require.NoError(t, err)
			// The trigger pauses COPY at its first row so cancellation happens before Commit.
			const payloadCount = 16
			payload := bytes.Repeat([]byte("x"), 2*1024*1024)
			payloads := make([][]byte, payloadCount)
			for i := range payloads {
				payloads[i] = payload
			}
			result := &model.AdvanceResult{
				Status:     model.InputCompletionStatus_Accepted,
				Outputs:    [][]byte{[]byte("output before reports")},
				Reports:    payloads,
				StateProof: *repotest.DummyStateProof(),
			}
			if tableName == table.Output.TableName() {
				result.Outputs, result.Reports = result.Reports, result.Outputs
			}
			writeCtx, cancel := context.WithCancel(ctx)
			done := make(chan error, 1)
			finished := make(chan struct{})
			t.Cleanup(func() {
				cancel()
				cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				// Release the backend's trigger wait before dropping its function.
				_, err := control.Exec(cleanupCtx, "SELECT pg_advisory_unlock($1)", copyLock)
				require.NoError(t, err)
				select {
				case <-finished:
				case <-cleanupCtx.Done():
					t.Error("COPY goroutine did not stop during cleanup")
				}
			})
			go func() {
				defer close(finished)
				done <- repo.StoreAdvanceResult(writeCtx, seed.App.ID, result)
			}()
			require.Eventually(t, func() bool {
				var paused bool
				err := control.QueryRow(ctx, `SELECT EXISTS (
					SELECT 1 FROM pg_stat_activity
					WHERE datname = current_database() AND query LIKE 'copy %' AND wait_event = 'advisory'
				)`).Scan(&paused)
				return err == nil && paused
			}, 5*time.Second, 10*time.Millisecond, "COPY must reach the paused trigger")
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
				require.True(t, errutil.IsOnlyCancellation(err), "%v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("canceled COPY did not return")
			}
			_, err = control.Exec(ctx, "SELECT pg_advisory_unlock($1)", copyLock)
			require.NoError(t, err)
			for _, tableName := range []string{table.Output.TableName(), table.Report.TableName()} {
				var rows uint64
				require.NoError(t, control.QueryRow(ctx, "SELECT count(*) FROM "+tableName).Scan(&rows))
				require.Zero(t, rows)
			}
			input, err := repo.GetInput(ctx, seed.App.Name, 0)
			require.NoError(t, err)
			require.Equal(t, model.InputCompletionStatus_None, input.Status)
			app, err := repo.GetApplication(ctx, seed.App.Name)
			require.NoError(t, err)
			require.Zero(t, app.ProcessedInputs)
			// A fresh connection can persist the result after the aborted transaction.
			require.NoError(t, repo.StoreAdvanceResult(ctx, seed.App.ID, result))
		})
	}
}

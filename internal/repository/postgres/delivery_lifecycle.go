// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

// The caller bounds cascade execution. Lock acquisition remains short even for
// applications whose history takes several minutes to delete.
func setDeletionTimeouts(ctx context.Context, tx pgx.Tx) error {
	settings := []postgres.Projection{postgres.Func("set_config", postgres.String("lock_timeout"),
		postgres.String("2s"), postgres.Bool(true))}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.DeadlineExceeded
		}
		settings = append(settings, postgres.Func("set_config", postgres.String("statement_timeout"),
			postgres.String(fmt.Sprintf("%dms", min(int64(math.MaxInt32), max(int64(1), remaining.Milliseconds())))), postgres.Bool(true)))
	}
	stmt := postgres.SELECT(settings[0], settings[1:]...)
	query, args := stmt.Sql()
	_, err := tx.Exec(ctx, query, args...)
	return err
}

// DeleteApplication serializes with observation and enable/disable changes.
// The enabled predicate is checked again after taking the application row lock.
func (r *PostgresRepository) DeleteApplication(ctx context.Context, id int64) error {
	if _, bounded := ctx.Deadline(); !bounded {
		var cancel context.CancelFunc
		ctx, cancel = beginDatabaseOperation(ctx)
		defer cancel()
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer rollbackDeliveryTx(tx)
	if err = setDeletionTimeouts(ctx, tx); err != nil {
		return fmt.Errorf("setting deletion timeouts: %w", err)
	}
	lock := table.Application.SELECT(table.Application.Enabled).
		WHERE(table.Application.ID.EQ(postgres.Int64(id))).FOR(postgres.UPDATE())
	query, args := lock.Sql()
	var enabled bool
	if err = tx.QueryRow(ctx, query, args...).Scan(&enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.ErrNotFound
		}
		return err
	}
	if enabled {
		return repository.ErrApplicationEnabled
	}
	stmt := table.Application.DELETE().WHERE(table.Application.ID.EQ(postgres.Int64(id)).
		AND(table.Application.Enabled.EQ(postgres.Bool(false))))
	query, args = stmt.Sql()
	command, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return repository.ErrNoUpdate
	}
	return tx.Commit(ctx)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

// StoreTournamentEvents seeds legacy constraint/read fixtures only. It is built
// exclusively into this package's tests; production uses StoreTournamentInterval.
func (r *PostgresRepository) StoreTournamentEvents(
	ctx context.Context, appID int64, batches []*repository.TournamentEventBatch, lastBlock uint64,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tournament event window: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Keep the application aggregate lock before every child-row lock.
	lockStmt := table.Application.SELECT(table.Application.ID).
		WHERE(table.Application.ID.EQ(postgres.Int64(appID))).FOR(postgres.NO_KEY_UPDATE())
	lockSQL, lockArgs := lockStmt.Sql()
	var lockedID int64
	if err := tx.QueryRow(ctx, lockSQL, lockArgs...).Scan(&lockedID); errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("locking tournament application %d: %w", appID, err)
	}
	for _, batch := range batches {
		if err := validateTournamentBatch(appID, batch, lastBlock); err != nil {
			return err
		}
		if err := storeTournamentProjection(ctx, tx, appID, batch.Tournament); err != nil {
			return err
		}
		if err := insertChunked(ctx, tx, appID, batch.Commitments, insertCommitmentChunk); err != nil {
			return fmt.Errorf("storing tournament commitments: %w", err)
		}
		if err := insertChunked(ctx, tx, appID, batch.Matches, insertMatchChunk); err != nil {
			return fmt.Errorf("storing tournament matches: %w", err)
		}
		if err := insertChunked(ctx, tx, appID, batch.MatchAdvances, insertMatchAdvancedChunk); err != nil {
			return fmt.Errorf("storing tournament match advances: %w", err)
		}
		if err := insertChunked(ctx, tx, appID, batch.StandingChanges, insertStandingChangedChunk); err != nil {
			return fmt.Errorf("storing tournament standing changes: %w", err)
		}
		if err := insertChunked(ctx, tx, appID, batch.BondEvents, insertBondEventChunk); err != nil {
			return fmt.Errorf("storing tournament bonds: %w", err)
		}
	}
	if err := updateLastProcessedBlock(ctx, tx, appID, lastBlock); err != nil {
		return fmt.Errorf("storing tournament checkpoint: %w", err)
	}
	return tx.Commit(ctx)
}

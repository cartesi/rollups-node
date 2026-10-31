// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

// StoreTournamentInterval commits one complete interval and the first complete
// observations of new descendants. Known children have independent cursors.
func (r *PostgresRepository) StoreTournamentInterval(
	ctx context.Context, appID int64, expectedAsOf *uint64, batches []*repository.TournamentEventBatch,
) error {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	if len(batches) == 0 || batches[0] == nil || batches[0].Tournament == nil {
		return errors.New("tournament interval requires a primary projection")
	}
	through := batches[0].Tournament.Snapshot.AsOfBlock
	for _, batch := range batches {
		if err := validateTournamentBatch(appID, batch, through); err != nil {
			return err
		}
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("beginning tournament interval: %w", err)
	}
	defer rollbackDeliveryTx(tx)
	// Match guarded deletion: application first, before tournament or event rows.
	stmt := table.Application.SELECT(table.Application.ID).
		WHERE(table.Application.ID.EQ(postgres.Int64(appID))).FOR(postgres.NO_KEY_UPDATE())
	query, args := stmt.Sql()
	var lockedID int64
	if err = tx.QueryRow(ctx, query, args...).Scan(&lockedID); errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("locking tournament application %d: %w", appID, err)
	}
	if err = validateTournamentInterval(ctx, tx, appID, expectedAsOf, batches, through); err != nil {
		return err
	}
	for _, batch := range batches {
		if err = storeTournamentProjection(ctx, tx, appID, batch.Tournament); err != nil {
			return err
		}
		if err = insertChunked(ctx, tx, appID, batch.Commitments, insertCommitmentChunk); err != nil {
			return fmt.Errorf("storing tournament commitments: %w", err)
		}
		if err = insertChunked(ctx, tx, appID, batch.Matches, insertMatchChunk); err != nil {
			return fmt.Errorf("storing tournament matches: %w", err)
		}
		if err = insertChunked(ctx, tx, appID, batch.MatchAdvances, insertMatchAdvancedChunk); err != nil {
			return fmt.Errorf("storing tournament match advances: %w", err)
		}
		if err := insertChunked(ctx, tx, appID, batch.StandingChanges, insertStandingChangedChunk); err != nil {
			return fmt.Errorf("storing tournament standing changes: %w", err)
		}
		if err = insertChunked(ctx, tx, appID, batch.BondEvents, insertBondEventChunk); err != nil {
			return fmt.Errorf("storing tournament bonds: %w", err)
		}
	}
	// This is a diagnostic high-water mark, not application-wide coverage.
	if err = updateLastProcessedBlock(ctx, tx, appID, through); err != nil {
		return fmt.Errorf("storing tournament high-water mark: %w", err)
	}
	return tx.Commit(ctx)
}

func validateTournamentInterval(
	ctx context.Context, tx pgx.Tx, appID int64, expectedAsOf *uint64,
	batches []*repository.TournamentEventBatch, through uint64,
) error {
	type includedTournament struct {
		projection *model.Tournament
		from       uint64
	}
	included := make(map[common.Address]includedTournament, len(batches))
	primary := batches[0].Tournament
	for index, batch := range batches {
		t := batch.Tournament
		if _, duplicate := included[t.Address]; duplicate {
			return errors.New("duplicate tournament in interval")
		}
		if t.EpochIndex != primary.EpochIndex || t.Address == (common.Address{}) || t.StartInstant > through {
			return errors.New("invalid tournament interval identity or start")
		}
		stmt := table.Tournaments.SELECT(table.Tournaments.AsOfBlock).WHERE(
			table.Tournaments.ApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Tournaments.EpochIndex.EQ(uint64Expr(t.EpochIndex))).
				AND(table.Tournaments.Address.EQ(postgres.Bytea(t.Address.Bytes()))))
		query, args := stmt.Sql()
		var cursor uint64
		err := tx.QueryRow(ctx, query, args...).Scan(&cursor)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reading tournament cursor: %w", err)
		}
		exists := err == nil
		from := t.StartInstant
		if index == 0 {
			if (expectedAsOf == nil && exists) || (expectedAsOf != nil && (!exists || cursor != *expectedAsOf)) {
				return repository.ErrTournamentIntervalStale
			}
			if exists {
				if cursor == math.MaxUint64 || through <= cursor {
					return fmt.Errorf("%w: tournament interval must advance its cursor", repository.ErrTournamentIntervalStale)
				}
				from = max(from, cursor+1)
			} else {
				if t.Level != 0 || t.ParentTournamentAddress != nil || t.ParentMatchIDHash != nil || t.CreationEvent != nil {
					return errors.New("a new primary tournament must be a root")
				}
				root := table.Epoch.SELECT(table.Epoch.TournamentAddress, table.Epoch.LastBlock).WHERE(
					table.Epoch.ApplicationID.EQ(postgres.Int64(appID)).AND(table.Epoch.Index.EQ(uint64Expr(t.EpochIndex))))
				query, args = root.Sql()
				var address *common.Address
				var sealBlock uint64
				if err = tx.QueryRow(ctx, query, args...).Scan(&address, &sealBlock); err != nil {
					return fmt.Errorf("reading new root provenance: %w", err)
				}
				if address == nil || *address != t.Address || sealBlock > through ||
					t.StartInstant != sealBlock {
					return errors.New("new root does not match its observed epoch seal")
				}
			}
		} else {
			if exists {
				return fmt.Errorf("%w: descendant %s already exists", repository.ErrTournamentIntervalStale, t.Address)
			}
			if t.ParentTournamentAddress == nil || t.ParentMatchIDHash == nil || t.CreationEvent == nil {
				return errors.New("new descendant requires parent and creation provenance")
			}
			parent, ok := included[*t.ParentTournamentAddress]
			if !ok || t.Level != parent.projection.Level+1 || t.MaxLevel != parent.projection.MaxLevel ||
				t.CreationEvent.BlockNumber != t.StartInstant || t.StartInstant < parent.from {
				return errors.New("new descendant does not belong to its parent's interval")
			}
		}
		for _, event := range batch.MatchAdvances {
			if event.BlockNumber < from {
				return errors.New("match advance precedes tournament interval")
			}
		}
		for _, event := range batch.StandingChanges {
			if event.BlockNumber < from {
				return errors.New("standing change precedes tournament interval")
			}
		}
		for _, event := range batch.BondEvents {
			if event.BlockNumber < from {
				return errors.New("bond event precedes tournament interval")
			}
		}
		if err := validatePriorTournamentParticipants(ctx, tx, appID, batch, from); err != nil {
			return err
		}
		included[t.Address] = includedTournament{projection: t, from: from}
	}
	return nil
}

// Refreshed participants may carry older creation, seal and deletion facts.
// Every such fact must already exist; the upsert checks its full identity.
func validatePriorTournamentParticipants(
	ctx context.Context, tx pgx.Tx, appID int64, batch *repository.TournamentEventBatch, from uint64,
) error {
	t := batch.Tournament
	var commitments, matches, seals, deletions [][]byte
	for _, value := range batch.Commitments {
		if value.BlockNumber < from {
			commitments = append(commitments, value.Commitment.Bytes())
		}
	}
	for _, value := range batch.Matches {
		if value.BlockNumber < from {
			matches = append(matches, value.IDHash.Bytes())
		}
		if value.LeafSeal != nil && value.LeafSeal.BlockNumber < from {
			seals = append(seals, value.IDHash.Bytes())
		}
		if value.DeletionBlockNumber > 0 && value.DeletionBlockNumber < from {
			deletions = append(deletions, value.IDHash.Bytes())
		}
	}
	commitmentScope := table.Commitments.ApplicationID.EQ(postgres.Int64(appID)).
		AND(table.Commitments.EpochIndex.EQ(uint64Expr(t.EpochIndex))).
		AND(table.Commitments.TournamentAddress.EQ(postgres.Bytea(t.Address.Bytes())))
	matchScope := table.Matches.ApplicationID.EQ(postgres.Int64(appID)).
		AND(table.Matches.EpochIndex.EQ(uint64Expr(t.EpochIndex))).
		AND(table.Matches.TournamentAddress.EQ(postgres.Bytea(t.Address.Bytes())))
	for _, check := range []struct {
		source postgres.Table
		column postgres.ColumnBytea
		scope  postgres.BoolExpression
		keys   [][]byte
		kind   string
	}{
		{table.Commitments, table.Commitments.Commitment, commitmentScope, commitments, "commitment"},
		{table.Matches, table.Matches.IDHash, matchScope, matches, "match"},
		{table.Matches, table.Matches.IDHash, matchScope.AND(table.Matches.SealTxHash.IS_NOT_NULL()), seals, "leaf seal"},
		{table.Matches, table.Matches.IDHash, matchScope.AND(table.Matches.DeletionTxHash.IS_NOT_NULL()), deletions, "match deletion"},
	} {
		if len(check.keys) == 0 {
			continue
		}
		// One bound array avoids both PostgreSQL's parameter ceiling and the
		// repeated full-prefix scans of generic plans for large IN lists.
		keys := postgres.SELECT(postgres.Func("unnest", postgres.ByteaArray(check.keys...)).AS("identity")).AsTable("prior_keys")
		key := postgres.ByteaColumn("identity").From(keys)
		stmt := check.source.INNER_JOIN(keys, check.column.EQ(key)).SELECT(postgres.COUNT(postgres.STAR)).WHERE(check.scope)
		query, args := stmt.Sql()
		var count int
		if err := tx.QueryRow(ctx, query, args...).Scan(&count); err != nil {
			return fmt.Errorf("checking prior %s: %w", check.kind, err)
		}
		if count != len(check.keys) {
			return fmt.Errorf("new %s precedes tournament interval", check.kind)
		}
	}
	return nil
}

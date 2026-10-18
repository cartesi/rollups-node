// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Row locks do not protect absent configuration. This documented raw SQL
// exception uses PostgreSQL's two-key advisory-lock function with bound keys.
const registrationLockNamespace int32 = 1415074375 // TXRG
const deliveryClaimerKey = "claimer"
const deliveryPRTKey = "prt"
const deliveryObserverKey = "evm-reader"
const deliveryDatabaseTimeout = 5 * time.Second

func registrationLock(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1,$2)", registrationLockNamespace, int32(0))
	return err
}

func rollbackDeliveryTx(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), deliveryDatabaseTimeout)
	defer cancel()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		level := slog.LevelError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
			(tx.Conn() != nil && tx.Conn().IsClosed()) {
			level = slog.LevelDebug
		}
		slog.Log(ctx, level, "failed to roll back delivery transaction", "error", err)
	}
}

func validateObserverPolicy(c config.PersistentChainConfig) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.DefaultBlock == model.DefaultBlock_Pending {
		return errors.New("pending durable observation is unsupported")
	}
	return nil
}

// Registration locks the complete key set in the same order. The advisory
// lock also protects configuration keys that do not exist yet.
func lockDeliveryConfigs(ctx context.Context, tx pgx.Tx) (map[string][]byte, error) {
	stmt := table.NodeConfig.SELECT(table.NodeConfig.Key, table.NodeConfig.Value).
		WHERE(table.NodeConfig.Key.IN(
			postgres.String(deliveryClaimerKey), postgres.String(deliveryObserverKey), postgres.String(deliveryPRTKey),
		)).
		ORDER_BY(table.NodeConfig.Key.ASC()).FOR(postgres.UPDATE())
	query, args := stmt.Sql()
	rows, err := tx.Query(ctx, query, args...)
	return deliveryConfigRecords(rows, err)
}

func deliveryConfigRecords(rows pgx.Rows, err error) (map[string][]byte, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make(map[string][]byte)
	for rows.Next() {
		var key string
		var raw []byte
		if err = rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		records[key] = raw
	}
	return records, rows.Err()
}

func hasUnprovenData(ctx context.Context, tx pgx.Tx, key string) (bool, error) {
	existsEpoch := postgres.EXISTS(table.Epoch.SELECT(postgres.CAST(postgres.Int(1)).AS_BIGINT()))
	existsInput := postgres.EXISTS(table.Input.SELECT(postgres.CAST(postgres.Int(1)).AS_BIGINT()))
	var predicate postgres.BoolExpression
	switch key {
	case deliveryObserverKey:
		predicate = existsEpoch.OR(existsInput).OR(postgres.EXISTS(table.Application.
			SELECT(postgres.CAST(postgres.Int(1)).AS_BIGINT()).WHERE(
			table.Application.LastEpochCheckBlock.GT(uint64Expr(0)).OR(table.Application.LastInputCheckBlock.GT(uint64Expr(0))).
				OR(table.Application.LastOutputCheckBlock.GT(uint64Expr(0))).
				OR(table.Application.LastForecloseCheckBlock.GT(uint64Expr(0))),
		)))
	case deliveryPRTKey:
		predicate = postgres.EXISTS(table.Tournaments.SELECT(postgres.CAST(postgres.Int(1)).AS_BIGINT())).OR(postgres.EXISTS(
			table.Application.SELECT(postgres.CAST(postgres.Int(1)).AS_BIGINT()).
				WHERE(table.Application.ConsensusType.EQ(enum.Consensus.Prt).
					AND(table.Application.LastTournamentCheckBlock.GT(uint64Expr(0)))),
		))
	case deliveryClaimerKey:
		predicate = postgres.EXISTS(table.Epoch.SELECT(postgres.CAST(postgres.Int(1)).AS_BIGINT()).FROM(table.Epoch.INNER_JOIN(
			table.Application, table.Application.ID.EQ(table.Epoch.ApplicationID),
		)).WHERE(table.Application.ConsensusType.IN(enum.Consensus.Authority, enum.Consensus.Quorum).
			AND(table.Epoch.Status.NOT_IN(
				enum.EpochStatus.Open, enum.EpochStatus.Closed, enum.EpochStatus.InputsProcessed, enum.EpochStatus.ClaimComputed,
			))))
	default:
		return false, fmt.Errorf("unsupported delivery configuration key %q", key)
	}
	stmt := postgres.SELECT(predicate)
	query, args := stmt.Sql()
	var exists bool
	err := tx.QueryRow(ctx, query, args...).Scan(&exists)
	return exists, err
}

func configChain(c config.PersistentSubmitterConfig) config.PersistentChainConfig {
	return config.PersistentChainConfig{DefaultBlock: c.DefaultBlock, ChainID: c.ChainID}
}

func insertDeliveryConfig(ctx context.Context, tx pgx.Tx, key string, raw []byte) error {
	stmt := table.NodeConfig.INSERT(table.NodeConfig.Key, table.NodeConfig.Value).
		VALUES(key, postgres.Json(raw)).ON_CONFLICT(table.NodeConfig.Key).DO_NOTHING()
	query, args := stmt.Sql()
	_, err := tx.Exec(ctx, query, args...)
	return err
}

func (r *PostgresRepository) RegisterObserver(
	ctx context.Context, key string, requested config.PersistentChainConfig,
) (config.PersistentChainConfig, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	if key != deliveryObserverKey {
		return requested, fmt.Errorf("unsupported observer %q", key)
	}
	if err := validateObserverPolicy(requested); err != nil {
		return requested, err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return requested, err
	}
	defer rollbackDeliveryTx(tx)
	if err = registrationLock(ctx, tx); err != nil {
		return requested, err
	}
	records, err := lockDeliveryConfigs(ctx, tx)
	if err != nil {
		return requested, err
	}
	raw, exists := records[key]
	if !exists {
		data, checkErr := hasUnprovenData(ctx, tx, key)
		if checkErr != nil {
			return requested, checkErr
		}
		if data {
			return requested, errors.New("observer provenance is absent for existing domain facts")
		}
		raw, err = json.Marshal(requested)
		if err != nil {
			return requested, err
		}
	}
	var saved config.PersistentChainConfig
	if err = json.Unmarshal(raw, &saved); err != nil {
		return saved, err
	}
	if err = saved.CheckRequested(requested); err != nil {
		return saved, err
	}
	if err = validateObserverPolicy(saved); err != nil {
		return saved, err
	}
	for _, role := range []string{deliveryClaimerKey, deliveryPRTKey} {
		roleRaw, exists := records[role]
		if !exists {
			continue
		}
		var roleConfig config.PersistentSubmitterConfig
		if err = json.Unmarshal(roleRaw, &roleConfig); err != nil {
			return saved, err
		}
		if err = configChain(roleConfig).CheckRequested(saved); err != nil {
			return saved, err
		}
	}
	if err = insertDeliveryConfig(ctx, tx, key, raw); err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}

func (r *PostgresRepository) RegisterSubmitter(
	ctx context.Context, key string, requested config.PersistentSubmitterConfig,
) (config.PersistentSubmitterConfig, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	if key != deliveryClaimerKey && key != deliveryPRTKey {
		return requested, fmt.Errorf("unsupported submitter %q", key)
	}
	if err := validateObserverPolicy(configChain(requested)); err != nil {
		return requested, err
	}
	if err := requested.Validate(); err != nil {
		return requested, err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return requested, err
	}
	defer rollbackDeliveryTx(tx)
	if err = registrationLock(ctx, tx); err != nil {
		return requested, err
	}
	records, err := lockDeliveryConfigs(ctx, tx)
	if err != nil {
		return requested, err
	}
	raw, exists := records[key]
	if !exists {
		data, checkErr := hasUnprovenData(ctx, tx, key)
		if checkErr != nil {
			return requested, checkErr
		}
		if data {
			return requested, errors.New("submitter provenance is absent for existing domain facts")
		}

		raw, err = json.Marshal(requested)
		if err != nil {
			return requested, err
		}
	}
	var saved config.PersistentSubmitterConfig
	err = json.Unmarshal(raw, &saved)
	if err != nil {
		return saved, err
	}
	if err = saved.CheckRequested(requested); err != nil {
		return saved, err
	}
	other := deliveryClaimerKey
	if key == deliveryClaimerKey {
		other = deliveryPRTKey
	}
	if otherRaw, exists := records[other]; exists {
		var otherConfig config.PersistentSubmitterConfig
		if err = json.Unmarshal(otherRaw, &otherConfig); err != nil {
			return saved, err
		}
		if err = configChain(otherConfig).CheckRequested(configChain(saved)); err != nil {
			return saved, fmt.Errorf("incompatible %s and %s registrations: %w", other, key, err)
		}
		bothSubmitting := saved.ClaimSubmissionEnabled && otherConfig.ClaimSubmissionEnabled
		if bothSubmitting && saved.ResolvedSignerAddress == otherConfig.ResolvedSignerAddress {
			return saved, errors.New("claimer and PRT require distinct submitting addresses")
		}
	}
	if err = checkReaderProvenance(ctx, tx, records, saved); err != nil {
		return saved, err
	}
	if err = insertDeliveryConfig(ctx, tx, key, raw); err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}

func checkReaderProvenance(
	ctx context.Context, tx pgx.Tx, records map[string][]byte, saved config.PersistentSubmitterConfig,
) error {
	raw, exists := records[deliveryObserverKey]
	if !exists {
		data, err := hasUnprovenData(ctx, tx, deliveryObserverKey)
		if err != nil {
			return err
		}
		if data {
			return errors.New("contributing observer provenance is absent for existing domain facts")
		}
		return nil
	}
	var reader config.PersistentChainConfig
	if err := json.Unmarshal(raw, &reader); err != nil {
		return err
	}
	if err := reader.CheckRequested(configChain(saved)); err != nil {
		return err
	}
	return validateObserverPolicy(reader)
}

func (r *PostgresRepository) CheckSubmitterProvenance(ctx context.Context, key string) (err error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	defer func() {
		if err != nil {
			err = errors.Join(repository.ErrSubmitterProvenance, err)
		}
	}()
	if key != deliveryClaimerKey && key != deliveryPRTKey {
		return fmt.Errorf("unsupported submitter %q", key)
	}
	// Registration records are insert-once. Read the role and contributing
	// reader in one statement snapshot without locking unrelated publication.
	records, err := readDeliveryConfigs(ctx, r.db)
	if err != nil {
		return err
	}
	raw, exists := records[key]
	if !exists {
		return repository.ErrNotFound
	}
	var saved config.PersistentSubmitterConfig
	err = json.Unmarshal(raw, &saved)
	if err != nil {
		return err
	}
	if err = validateObserverPolicy(configChain(saved)); err != nil {
		return err
	}
	readerRaw, exists := records[deliveryObserverKey]
	if !exists {
		return repository.ErrObserverNotRegistered
	}
	var reader config.PersistentChainConfig
	if err = json.Unmarshal(readerRaw, &reader); err != nil {
		return err
	}
	if err = reader.CheckRequested(configChain(saved)); err != nil {
		return err
	}
	return validateObserverPolicy(reader)
}

func readDeliveryConfigs(ctx context.Context, pool *pgxpool.Pool) (map[string][]byte, error) {
	stmt := table.NodeConfig.SELECT(table.NodeConfig.Key, table.NodeConfig.Value).
		WHERE(table.NodeConfig.Key.IN(
			postgres.String(deliveryClaimerKey), postgres.String(deliveryObserverKey), postgres.String(deliveryPRTKey),
		))
	query, args := stmt.Sql()
	rows, err := pool.Query(ctx, query, args...)
	return deliveryConfigRecords(rows, err)
}

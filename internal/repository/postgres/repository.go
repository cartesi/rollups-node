// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
)

// postgresRepository is the concrete type that implements the repository.Repository interface.
type PostgresRepository struct {
	db *pgxpool.Pool
}

func (r *PostgresRepository) Close() {
	if r.db != nil {
		r.db.Close()
	}
}

func validateSchema(pool *pgxpool.Pool) error {

	s, err := schema.NewWithPool(pool)
	if err != nil {
		return err
	}
	defer s.Close()

	_, err = s.ValidateVersion()
	return err
}

func NewPostgresRepository(ctx context.Context, conn string, maxRetries int, delay time.Duration) (repository.Repository, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	config, err := pgxpool.ParseConfig(conn)
	if err != nil {
		// pgx masks passwords, but parser errors still contain other DSN values.
		// Do not retain its raw connection string in the public error chain.
		return nil, errors.New("failed to parse Postgres connection string")
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("failed to create Postgres pool")
	}

	// Wait for database to be available
	for i := range maxRetries {
		if err := pool.Ping(ctx); err == nil {
			break
		}
		if err := ctx.Err(); err != nil {
			pool.Close()
			return nil, err
		}
		if i == maxRetries-1 {
			pool.Close()
			return nil, fmt.Errorf("failed to ping Postgres after %d retries", maxRetries)
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		}
	}

	// Wait for schema validation (migrations) to complete. Workaround to facilitate container startup order.
	for i := range maxRetries {
		err = validateSchema(pool)
		if err == nil {
			return &PostgresRepository{db: pool}, nil
		}
		if err := ctx.Err(); err != nil {
			pool.Close()
			return nil, err
		}
		if i == maxRetries-1 {
			pool.Close()
			return nil, safeSchemaValidationError(err)
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		}
	}

	// This should never be reached due to the returns in the loops above
	pool.Close()
	return nil, fmt.Errorf("unexpected error initializing Postgres repository")
}

// Only local schema diagnostics and context sentinels may escape construction.
// Driver errors can contain connection settings even after parsing succeeds.
func safeSchemaValidationError(err error) error {
	var safe error
	for _, known := range []error{
		context.Canceled, context.DeadlineExceeded,
		schema.ErrMigrationNotCompleted, schema.ErrNoValidDatabaseSchema,
	} {
		if errors.Is(err, known) {
			safe = known
			break
		}
	}
	var mismatch *schema.VersionMismatchError
	if safe == nil && errors.As(err, &mismatch) {
		safe = mismatch
	}
	if safe != nil {
		return fmt.Errorf("failed to validate Postgres schema version: %w", safe)
	}
	return errors.New("failed to validate Postgres schema version")
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
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

	config, err := pgxpool.ParseConfig(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Postgres connection string: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Postgres pool: %w", err)
	}

	// Wait for database to be available
	for i := range maxRetries {
		if err := pool.Ping(ctx); err == nil {
			break
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
		if i == maxRetries-1 {
			pool.Close()
			return nil, fmt.Errorf("failed to validate Postgres schema version: %w", err)
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

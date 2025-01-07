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
type postgresRepository struct {
	db *pgxpool.Pool
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

	for i := 0; i < maxRetries; i++ {
		if err := pool.Ping(ctx); err == nil {
			err = validateSchema(pool)
			if err != nil {
				return nil, fmt.Errorf("failed to validate Postgres schema version: %w", err)
			}

			return &postgresRepository{db: pool}, nil
		}
		time.Sleep(delay)
	}

	pool.Close()
	return nil, fmt.Errorf("failed to ping Postgres after %d retries", maxRetries)

}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package db

import (
	"context"
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/jackc/pgx/v5"
)

const testDatabaseLockID int64 = 0x4352545349544553 // "CRTSITES"

func GetTestDatabaseEndpoint() (string, error) {
	endpoint, ok := os.LookupEnv("CARTESI_TEST_DATABASE_CONNECTION")
	if !ok {
		return "", fmt.Errorf("environment variable CARTESI_TEST_DATABASE_CONNECTION not set")
	}
	return endpoint, nil
}

// LockTestPostgres serializes package test processes that reset the shared test
// schema. The session-level advisory lock is held until the returned connection
// closer is called.
func LockTestPostgres(ctx context.Context, endpoint string) (func(), error) {
	conn, err := pgx.Connect(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to connect for test database lock: %w", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", testDatabaseLockID); err != nil {
		_ = conn.Close(context.Background())
		return nil, fmt.Errorf("failed to lock test database: %w", err)
	}
	return func() { _ = conn.Close(context.Background()) }, nil
}

func SetupTestPostgres(endpoint string) error {

	schema, err := schema.New(endpoint)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	defer schema.Close()

	err = schema.Downgrade()
	if err != nil {
		return fmt.Errorf("failed to downgrade schema: %w", err)
	}

	err = schema.Upgrade()
	if err != nil {
		return fmt.Errorf("failed to upgrade schema: %w", err)
	}

	return nil
}

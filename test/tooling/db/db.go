// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package db

import (
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
)

func GetTestDatabaseEndpoint() (string, error) {
	endpoint, ok := os.LookupEnv("CARTESI_TEST_DATABASE_CONNECTION")
	if !ok {
		return "", fmt.Errorf("environment variable CARTESI_TEST_DATABASE_CONNECTION not set")
	}
	return endpoint, nil
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

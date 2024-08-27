// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package db

import (
	"context"
	"os"
	"time"

	"github.com/cartesi/rollups-node/internal/repository/schema"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	postgresImage    = "postgres:16-alpine"
	postgresDatabase = "cartesinode"
	postgresUsername = "admin"
	postgresPassword = "password"
)

func Setup(ctx context.Context) (string, error) {
	endpoint, ok := os.LookupEnv("TEST_POSTGRES_ENDPOINT")
	if !ok {
		container, err := SetupContainer(ctx)
		if err != nil {
			return "", err
		}

		endpoint, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			return "", err
		}
	}

	err := SetupSchema(endpoint)
	if err != nil {
		return "", err
	}

	return endpoint, nil
}

func SetupContainer(ctx context.Context) (*postgres.PostgresContainer, error) {
	log := "database system is ready to accept connections"
	occurrences := 2            //nolint: mnd
	timeout := 10 * time.Second //nolint: mnd
	strategy := wait.ForLog(log).WithOccurrence(occurrences).WithStartupTimeout(timeout)
	return postgres.Run(ctx,
		postgresImage,
		postgres.WithDatabase(postgresDatabase),
		postgres.WithUsername(postgresUsername),
		postgres.WithPassword(postgresPassword),
		testcontainers.WithWaitStrategy(strategy))
}

func SetupSchema(endpoint string) error {
	schema, err := schema.New(endpoint)
	if err != nil {
		return err
	}
	defer schema.Close()

	err = schema.Downgrade()
	if err != nil {
		return err
	}

	err = schema.Upgrade()
	if err != nil {
		return err
	}

	return nil
}

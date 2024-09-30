// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package db

import (
	"context"
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/repository/schema"
)

func Setup(ctx context.Context) (string, error) {
	endpoint, ok := os.LookupEnv("CARTESI_TESTS_POSTGRES_ENDPOINT")
	if !ok {
		return "", fmt.Errorf("environment variable CARTESI_TESTS_POSTGRES_ENDPOINT not set")
	}

	err := SetupSchema(endpoint)
	if err != nil {
		return "", err
	}

	return endpoint, nil
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

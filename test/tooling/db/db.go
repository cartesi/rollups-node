// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package db

import (
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/repository/schema"
)

func GetPostgresTestEndpoint() (string, error) {
	endpoint, ok := os.LookupEnv("CARTESI_TEST_POSTGRES_ENDPOINT")
	if !ok {
		return "", fmt.Errorf("environment variable CARTESI_TEST_POSTGRES_ENDPOINT not set")
	}
	return endpoint, nil
}

func SetupTestPostgres(endpoint string) error {

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

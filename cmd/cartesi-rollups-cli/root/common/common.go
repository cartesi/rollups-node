// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package common

import (
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

var (
	PostgresEndpoint string
	Repository       repository.Repository
)

func PersistentPreRun(cmd *cobra.Command, args []string) error {

	ctx := cmd.Context()

	var err error
	Repository, err = factory.NewRepositoryFromConnectionString(ctx, PostgresEndpoint)
	if err != nil {
		return err
	}

	return nil
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package common

import (
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/spf13/cobra"
)

var (
	PostgresEndpoint string
	Database         *repository.Database
)

func Setup(cmd *cobra.Command, args []string) {

	ctx := cmd.Context()

	var err error
	Database, err = repository.Connect(ctx, PostgresEndpoint)
	cobra.CheckErr(err)
}

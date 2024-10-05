// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package app

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/add"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/deploy"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/list"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:              "app",
	Short:            "Application management related commands",
	PersistentPreRun: common.Setup,
}

func init() {

	Cmd.PersistentFlags().StringVarP(
		&common.PostgresEndpoint,
		"postgres-endpoint",
		"p",
		"postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable",
		"Postgres endpoint",
	)

	Cmd.AddCommand(add.Cmd)
	Cmd.AddCommand(deploy.Cmd)
	Cmd.AddCommand(list.Cmd)
}

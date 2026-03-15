// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package app

import (
	"github.com/spf13/cobra"

	execution "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/execution-parameters"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/list"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/purge"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/register"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/remove"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/status"
)

var Cmd = &cobra.Command{
	Use:   "app",
	Short: "Application management related commands",
}

func init() {
	Cmd.AddCommand(register.Cmd)
	Cmd.AddCommand(list.Cmd)
	Cmd.AddCommand(status.Cmd)
	Cmd.AddCommand(remove.Cmd)
	Cmd.AddCommand(purge.Cmd)
	Cmd.AddCommand(execution.Cmd)
}

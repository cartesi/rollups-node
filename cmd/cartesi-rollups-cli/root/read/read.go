// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package read

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/input"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/inputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/notice"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/notices"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/report"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/reports"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/voucher"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/vouchers"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "read",
	Short: "Read the node state from the GraphQL API",
}

func init() {
	Cmd.AddCommand(input.Cmd)
	Cmd.AddCommand(inputs.Cmd)
	Cmd.AddCommand(notice.Cmd)
	Cmd.AddCommand(notices.Cmd)
	Cmd.AddCommand(voucher.Cmd)
	Cmd.AddCommand(vouchers.Cmd)
	Cmd.AddCommand(report.Cmd)
	Cmd.AddCommand(reports.Cmd)
}

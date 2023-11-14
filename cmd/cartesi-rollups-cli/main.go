// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"os"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/send"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cartesi-rollups-cli",
	Short: "Command line interface for Cartesi Rollups",
	Long: `Command line interface for Cartesi Rollups

This command line interface provides functionality to help develop and debug 
the Cartesi Rollups node.`,
}

func init() {
	rootCmd.AddCommand(send.Cmd)
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

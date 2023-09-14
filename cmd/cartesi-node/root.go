// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use: "cartesi-node [reader|validator|full|no-backend]",
	Run: func(cmd *cobra.Command, args []string) { cmd.Usage() },
}

func init() {
	rootCmd.AddCommand(reader)
	rootCmd.AddCommand(validator)
	rootCmd.AddCommand(full)
	rootCmd.AddCommand(noBackend)
}

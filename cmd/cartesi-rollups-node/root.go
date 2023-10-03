// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:                   "cartesi-rollups-node",
	CompletionOptions:     cobra.CompletionOptions{HiddenDefaultCmd: true},
	DisableFlagsInUseLine: true,
}

func init() {
	rootCmd.AddCommand(reader)
	rootCmd.AddCommand(validator)
	rootCmd.AddCommand(noBackend)
}

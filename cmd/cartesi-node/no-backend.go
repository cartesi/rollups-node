// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import "github.com/spf13/cobra"

var noBackend = &cobra.Command{
	Use:                   "no-backend",
	Short:                 "Starts the node in no-backend mode",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		println("TODO")
	},
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import "github.com/spf13/cobra"

var reader = &cobra.Command{
	Use:                   "reader",
	Short:                 "Starts the node in reader mode",
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		println("TODO")
	},
}

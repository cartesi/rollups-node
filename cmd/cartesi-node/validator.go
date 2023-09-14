// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import "github.com/spf13/cobra"

var validator = &cobra.Command{
	Use:   "validator",
	Short: "Starts the node in validator mode",
	Run: func(cmd *cobra.Command, args []string) {
		println("TODO")
	},
}

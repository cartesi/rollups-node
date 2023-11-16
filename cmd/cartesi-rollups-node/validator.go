// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"github.com/cartesi/rollups-node/internal/services"
	"github.com/spf13/cobra"
)

var validator = &cobra.Command{
	Use:                   "validator",
	Short:                 "Starts the node in validator mode",
	DisableFlagsInUseLine: true,
	Run:                   runValidatorNode,
}

func runValidatorNode(cmd *cobra.Command, args []string) {
	validatorServices := []services.Service{
		services.StateServer, // must be initialized before Dispatcher
		services.AdvanceRunner,
		services.AuthorityClaimer,
		services.GraphQLServer,
		services.Indexer,
		services.InspectServer,
		services.Dispatcher,
	}

	services.Run(cmd.Context(), validatorServices)
}

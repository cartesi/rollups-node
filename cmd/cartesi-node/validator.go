// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"github.com/cartesi/rollups-node/internal/pkg/services"
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
		services.GraphQLService{},
	}

	services.Run(validatorServices)
}

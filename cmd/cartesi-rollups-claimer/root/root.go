// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	service "github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/config"

	"github.com/spf13/cobra"
)

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion = "devel"
	createInfo   = service.CreateInfo{
		SignalTrapsEnabled: true,
		TelemetryEnabled:   true,
		TelemetryAddress:  ":8081",
	}
)

var Cmd = &cobra.Command{
	Use:   service.Name,
	Short: "Run " + service.Name,
	Long:  "Run " + service.Name + " in standalone mode",
	Run:   run,
}

func init() {
	c := config.FromEnv()
	createInfo.Auth = c.Auth
	createInfo.BlockchainHttpEndpoint = c.BlockchainHttpEndpoint
	createInfo.PostgresEndpoint = c.PostgresEndpoint

	Cmd.Flags().StringVar(&createInfo.TelemetryAddress,
		"telemetry-address", createInfo.TelemetryAddress,
		"telemetry address")
	Cmd.Flags().StringVar(&createInfo.BlockchainHttpEndpoint.Value,
		"blockchain-http-endpoint", createInfo.BlockchainHttpEndpoint.Value,
		"blockchain http endpoint")
	Cmd.Flags().DurationVar(&createInfo.PollInterval,
		"poll-interval", createInfo.PollInterval,
		"poll interval")
	Cmd.Flags().StringVar(&createInfo.LogLevel,
		"log-level", createInfo.LogLevel,
		"log level: debug, info, warn, error.")
}

func run(cmd *cobra.Command, args []string) {
	service, err := service.Create(createInfo)
	cobra.CheckErr(err)
	service.Start(true)
}

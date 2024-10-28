// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"log/slog"

	"github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/spf13/cobra"
)

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion   = "devel"
	claimerService = claimer.Service{}
	createInfo     = claimer.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 "claimer",
			ProcOwner:            true,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     ":8081",
			Impl:                 &claimerService,
		},
	}
)

var Cmd = &cobra.Command{
	Use:   createInfo.Name,
	Short: "Runs " + createInfo.Name,
	Long:  "Runs " + createInfo.Name + " in standalone mode",
	Run:   run,
}

func init() {
	c := config.FromEnv()
	createInfo.Auth = c.Auth
	createInfo.BlockchainHttpEndpoint = c.BlockchainHttpEndpoint
	createInfo.PostgresEndpoint = c.PostgresEndpoint
	createInfo.PollInterval = c.ClaimerPollingInterval
	createInfo.LogLevel = map[slog.Level]string{
		slog.LevelDebug: "debug",
		slog.LevelInfo:  "info",
		slog.LevelWarn:  "warn",
		slog.LevelError: "error",
	}[c.LogLevel]

	Cmd.Flags().StringVar(&createInfo.TelemetryAddress,
		"telemetry-address", createInfo.TelemetryAddress,
		"health check and metrics address and port")
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
	cobra.CheckErr(claimer.Create(createInfo, &claimerService))
	claimerService.CreateDefaultHandlers("/" + claimerService.Name)
	cobra.CheckErr(claimerService.Serve())
}

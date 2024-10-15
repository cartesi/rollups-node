// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"github.com/cartesi/rollups-node/internal/template/service"
	"github.com/spf13/cobra"
)

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion = "devel"
	createInfo   = service.CreateInfo{
		SignalTrapsEnabled:  true,
		TelemetrysEnabled:   true,
		TelemetryAddress:   ":10005",
	}
)

var Cmd = &cobra.Command{
	Use:   service.Name,
	Short: "Runs " + service.Name,
	Long:  "Runs " + service.Name + " in standalone mode",
	Run:   run,
}

func init() {
	Cmd.Flags().StringVar(&createInfo.TelemetryAddress,
		"telemetry-address", createInfo.TelemetryAddress,
		"health check and metrics address and port")
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

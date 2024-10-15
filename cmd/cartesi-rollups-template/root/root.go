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
		HealthChecksEnabled: true,
		HealthCheckAddress: "localhost:8080",
	}
)

const (
	CMD_NAME = service.ServiceName
)

var Cmd = &cobra.Command{
	Use:   CMD_NAME,
	Short: "Runs " + service.ServiceName,
	Long:  "Runs " + service.ServiceName + " in standalone mode",
	Run:   run,
}

func init() {
	Cmd.Flags().StringVar(&createInfo.HealthCheckAddress,
		"health-check-address", createInfo.HealthCheckAddress,
		"health check address")
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

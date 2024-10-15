// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/cartesi/rollups-node/internal/template"
	"github.com/spf13/cobra"
)

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion = "devel"
	createInfo   = template.CreateInfo{
		CreateInfo: service.CreateInfo {
			Name: "template",
			LogLevel: "debug",
			SignalTrapsCreate: true,
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
	//Cmd.Flags().StringVar(&createInfo.TelemetryAddress,
	//	"telemetry-address", createInfo.TelemetryAddress,
	//	"health check and metrics address and port")
	Cmd.Flags().DurationVar(&createInfo.PollInterval,
		"poll-interval", createInfo.PollInterval,
		"poll interval")
	Cmd.Flags().StringVar(&createInfo.LogLevel,
		"log-level", createInfo.LogLevel,
		"log level: debug, info, warn, error.")
}

func run(cmd *cobra.Command, args []string) {
	s := template.Service{}
	err := template.Create(createInfo, &s)
	cobra.CheckErr(err)
	s.Start(true)
}

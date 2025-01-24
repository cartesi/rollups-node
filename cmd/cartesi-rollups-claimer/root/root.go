// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"time"

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
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     ":10004",
			Impl:                 &claimerService,
		},
		EnableSubmission: true,
		MaxStartupTime:   10 * time.Second,
	}
	DefaultBlockString = "finalized"
)

var Cmd = &cobra.Command{
	Use:   createInfo.Name,
	Short: "Runs " + createInfo.Name,
	Long:  "Runs " + createInfo.Name + " in standalone mode",
	Run:   run,
}

func init() {
	createInfo.LoadEnv()
	Cmd.Flags().StringVar(&createInfo.TelemetryAddress,
		"telemetry-address", createInfo.TelemetryAddress,
		"health check and metrics address and port")
	Cmd.Flags().Var(&createInfo.LogLevel,
		"log-level",
		"log level: debug, info, warn or error")
	Cmd.Flags().BoolVar(&createInfo.LogPretty,
		"log-color", createInfo.LogPretty,
		"tint the logs (colored output)")
	Cmd.Flags().StringVar(&createInfo.BlockchainHttpEndpoint.Value,
		"blockchain-http-endpoint", createInfo.BlockchainHttpEndpoint.Value,
		"blockchain http endpoint")
	Cmd.Flags().DurationVar(&createInfo.PollInterval,
		"poll-interval", createInfo.PollInterval,
		"poll interval")
	Cmd.Flags().DurationVar(&createInfo.MaxStartupTime,
		"max-startup-time", createInfo.MaxStartupTime,
		"maximum startup time in seconds")
	Cmd.Flags().BoolVar(&createInfo.EnableSubmission,
		"claim-submission", createInfo.EnableSubmission,
		"enable or disable claim submission (reader mode)")
	Cmd.Flags().StringVarP(&DefaultBlockString,
		"default-block", "d", DefaultBlockString,
		`Default block to be used when fetching new blocks.
		One of 'latest', 'safe', 'pending', 'finalized'`)
}

func run(cmd *cobra.Command, args []string) {
	if cmd.Flags().Changed("default-block") {
		var err error
		createInfo.DefaultBlock, err = config.ToDefaultBlockFromString(DefaultBlockString)
		cobra.CheckErr(err)
	}
	cobra.CheckErr(claimer.Create(&createInfo, &claimerService))
	claimerService.CreateDefaultHandlers("")
	cobra.CheckErr(claimerService.Serve())
}

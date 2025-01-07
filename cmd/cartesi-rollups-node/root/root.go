// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"strings"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/node"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/spf13/cobra"
)

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion = "devel"
	nodeService  = node.Service{}
	createInfo   = node.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 "cartesi-rollups-node",
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     ":10000",
			Impl:                 &nodeService,
		},
		NodeConfig: model.NodeConfig[model.NodeConfigValue]{
			Value: model.NodeConfigValue{
				DefaultBlock: model.DefaultBlock_Finalized,
			},
		},
		MaxStartupTime: 10 * time.Second,
	}
	DefaultBlockString = "finalized"
	inputBoxAddress    service.EthAddress
)

var Cmd = &cobra.Command{
	Use:   createInfo.Name,
	Short: "Runs " + createInfo.Name,
	Long:  "Runs " + createInfo.Name + " as a single process",
	Run:   run,
}

func init() {
	createInfo.LoadEnv()
	Cmd.Flags().StringVar(&createInfo.TelemetryAddress,
		"telemetry-address", createInfo.TelemetryAddress,
		"telemetry address")
	Cmd.Flags().StringVarP(&DefaultBlockString,
		"default-block", "d", DefaultBlockString,
		`Default block to be used when fetching new blocks.
		One of 'latest', 'safe', 'pending', 'finalized'`)
	Cmd.Flags().Var(&inputBoxAddress,
		"inputbox-address",
		"Input Box contract address")
	Cmd.Flags().Uint64VarP(&createInfo.Value.InputBoxDeploymentBlock,
		"inputbox-block-number", "n", 0,
		"Input Box deployment block number")
	Cmd.Flags().Var(&createInfo.LogLevel,
		"log-level",
		"log level: debug, info, warn or error")
	Cmd.Flags().BoolVar(&createInfo.LogPretty,
		"log-color", createInfo.LogPretty,
		"tint the logs (colored output)")
	Cmd.Flags().BoolVar(&createInfo.EnableClaimSubmission,
		"claim-submission", createInfo.EnableClaimSubmission,
		"enable or disable claim submission (reader mode)")
	Cmd.Flags().DurationVar(&createInfo.MaxStartupTime,
		"max-startup-time", createInfo.MaxStartupTime,
		"maximum startup time in seconds")
}

func run(cmd *cobra.Command, args []string) {
	if cmd.Flags().Changed("default-block") {
		var err error
		createInfo.Value.DefaultBlock, err = config.ToDefaultBlockFromString(DefaultBlockString)
		cobra.CheckErr(err)
	}
	if cmd.Flags().Changed("inputbox-address") {
		createInfo.Value.InputBoxAddress = strings.ToLower(inputBoxAddress.String())
	}

	cobra.CheckErr(node.Create(&createInfo, &nodeService))
	nodeService.CreateDefaultHandlers("")
	cobra.CheckErr(nodeService.Serve())
}

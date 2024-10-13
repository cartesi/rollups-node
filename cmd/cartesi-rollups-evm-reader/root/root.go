// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader/service"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/services/startup"

	"github.com/spf13/cobra"
)

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion = "devel"
)

const (
	CMD_NAME = "evm-reader"
)

var Cmd = &cobra.Command{
	Use:   CMD_NAME,
	Short: "Runs EVM Reader",
	Long:  `Runs EVM Reader in standalone mode`,
	Run:   run,
}
var (
	defaultBlock                  string
	postgresEndpoint              string
	blockchainHttpEndpoint        string
	blockchainWsEndpoint          string
	inputBoxAddress               string
	inputBoxDeploymentBlockNumber uint64
	verbose                       bool
)

func init() {

	Cmd.Flags().StringVarP(&defaultBlock,
		"default-block",
		"d",
		"",
		`Default block to be used when fetching new blocks.
		One of 'latest', 'safe', 'pending', 'finalized'`)

	Cmd.Flags().StringVarP(&postgresEndpoint,
		"postgres-endpoint",
		"p",
		"",
		"Postgres endpoint")

	Cmd.Flags().StringVarP(&blockchainHttpEndpoint,
		"blockchain-http-endpoint",
		"b",
		"",
		"Blockchain HTTP Endpoint")

	Cmd.Flags().StringVarP(&blockchainWsEndpoint,
		"blockchain-ws-endpoint",
		"w",
		"",
		"Blockchain WS Endpoint")

	Cmd.Flags().StringVarP(&inputBoxAddress,
		"inputbox-address",
		"i",
		"",
		"Input Box contract address")

	Cmd.Flags().Uint64VarP(&inputBoxDeploymentBlockNumber,
		"inputbox-block-number",
		"n",
		0,
		"Input Box deployment block number")

	Cmd.Flags().BoolVarP(&verbose,
		"verbose",
		"v",
		false,
		"enable verbose logging")
}

func run(cmd *cobra.Command, args []string) {
	startTime := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := config.FromEnv()

	// Override configs
	if verbose {
		c.LogLevel = slog.LevelDebug
	}
	if postgresEndpoint != "" {
		c.PostgresEndpoint = config.Redacted[string]{Value: postgresEndpoint}
	}
	if blockchainHttpEndpoint != "" {
		c.BlockchainHttpEndpoint = config.Redacted[string]{Value: blockchainHttpEndpoint}
	}
	if blockchainWsEndpoint != "" {
		c.BlockchainWsEndpoint = config.Redacted[string]{Value: blockchainWsEndpoint}
	}
	if defaultBlock != "" {
		evmReaderDefaultBlock, err := config.ToDefaultBlockFromString(defaultBlock)
		cobra.CheckErr(err)
		c.EvmReaderDefaultBlock = evmReaderDefaultBlock
	}

	// setup log
	startup.ConfigLogs(c.LogLevel, c.LogPrettyEnabled)

	slog.Info("Starting the Cartesi Rollups Node EVM Reader", "version", buildVersion, "config", c)

	database, err := repository.Connect(ctx, c.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("EVM Reader couldn't connect to the database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	_, err = startup.SetupNodePersistentConfig(ctx, database, c)
	if err != nil {
		slog.Error("EVM Reader couldn't connect to the database", "error", err)
		os.Exit(1)
	}

	// create EVM Reader Service
	service := service.NewEvmReaderService(
		c.BlockchainHttpEndpoint.Value,
		c.BlockchainWsEndpoint.Value,
		database,
		c.EvmReaderRetryPolicyMaxRetries,
		c.EvmReaderRetryPolicyMaxDelay,
	)

	// logs startup time
	ready := make(chan struct{}, 1)
	go func() {
		select {
		case <-ready:
			duration := time.Since(startTime)
			slog.Info("EVM Reader is ready", "after", duration)
		case <-ctx.Done():
		}
	}()

	// start service
	if err := service.Start(ctx, ready); err != nil {
		slog.Error("EVM Reader exited with an error", "error", err)
		os.Exit(1)
	}
}

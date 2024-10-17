// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-espresso-reader/root/nonce"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/espressoreader"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/evmreader/retrypolicy"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/services/startup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion = "devel"
)

const (
	CMD_NAME = "espresso-sequencer"
)

var Cmd = &cobra.Command{
	Use:   CMD_NAME,
	Short: "Runs Espresso Reader",
	Long:  `Runs Espresso Reader`,
	Run:   run,
}

func init() {
	Cmd.AddCommand(nonce.Cmd)
}

func run(cmd *cobra.Command, args []string) {
	c := config.FromEnv()

	// setup log
	startup.ConfigLogs(c.LogLevel, c.LogPrettyEnabled)

	// Validate Schema
	err := startup.ValidateSchema(c.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("Espresso Reader exited with an error", "error", err)
		os.Exit(1)
	}

	ctx := cmd.Context()
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

	evmReader := setupEvmReader(ctx, c, database)

	espressoReader := espressoreader.NewEspressoReader(c.EspressoBaseUrl, c.EspressoStartingBlock, c.EspressoNamespace, database, evmReader)

	go setupNonceHttpServer()

	if err := espressoReader.Run(ctx); err != nil {
		slog.Error("EVM Reader exited with an error", "error", err)
		os.Exit(1)
	}
}

func setupEvmReader(ctx context.Context, c config.NodeConfig, database *repository.Database) *evmreader.EvmReader {
	client, err := ethclient.DialContext(ctx, c.BlockchainHttpEndpoint.Value)
	if err != nil {
		slog.Error("eth client http", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	wsClient, err := ethclient.DialContext(ctx, c.BlockchainWsEndpoint.Value)
	if err != nil {
		slog.Error("eth client ws", "error", err)
		os.Exit(1)
	}
	defer wsClient.Close()

	config, err := database.GetNodeConfig(ctx)
	if err != nil {
		slog.Error("db config", "error", err)
		os.Exit(1)
	}

	inputSource, err := evmreader.NewInputSourceAdapter(config.InputBoxAddress, client)
	if err != nil {
		slog.Error("input source", "error", err)
		os.Exit(1)
	}

	contractFactory := retrypolicy.NewEvmReaderContractFactory(client, c.EvmReaderRetryPolicyMaxRetries, c.EvmReaderRetryPolicyMaxDelay)

	evmReader := evmreader.NewEvmReader(
		retrypolicy.NewEhtClientWithRetryPolicy(client, c.EvmReaderRetryPolicyMaxRetries, c.EvmReaderRetryPolicyMaxDelay),
		retrypolicy.NewEthWsClientWithRetryPolicy(wsClient, c.EvmReaderRetryPolicyMaxRetries, c.EvmReaderRetryPolicyMaxDelay),
		retrypolicy.NewInputSourceWithRetryPolicy(inputSource, c.EvmReaderRetryPolicyMaxRetries, c.EvmReaderRetryPolicyMaxDelay),
		database,
		config.InputBoxDeploymentBlock,
		config.DefaultBlock,
		contractFactory,
	)

	return &evmReader
}

func setupNonceHttpServer() {
	http.HandleFunc("/nonce/{sender}/{dapp}", getNonce)

	http.ListenAndServe(":3333", nil)
}

func getNonce(w http.ResponseWriter, r *http.Request) {
	senderAddress := common.HexToAddress(r.PathValue("sender"))
	applicationAddress := common.HexToAddress(r.PathValue("dapp"))
	ctx := context.Background()

	nonce := process(ctx, senderAddress, applicationAddress)

	fmt.Printf("got nonce request\n")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	err := json.NewEncoder(w).Encode(nonce)
	if err != nil {
		slog.Info("Internal server error",
			"service", "espresso nonce querier",
			"err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func process(
	ctx context.Context,
	senderAddress common.Address,
	applicationAddress common.Address) uint64 {
	c := config.FromEnv()

	database, err := repository.Connect(ctx, c.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("EVM Reader couldn't connect to the database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if database == nil {
		panic("Database was not initialized")
	}

	nonce, err := database.GetEspressoNonce(ctx, senderAddress, applicationAddress)
	if err != nil {
		slog.Error("failed to get espresso nonce", "error", err)
		os.Exit(1)
	}

	return nonce
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/EspressoSystems/espresso-sequencer-go/client"
	"github.com/EspressoSystems/espresso-sequencer-go/types"
	"github.com/cartesi/rollups-node/internal/espressoreader"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/evmreader/retrypolicy"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Service to manage InputReader lifecycle
type EspressoReaderService struct {
	blockchainHttpEndpoint string
	blockchainWsEndpoint   string
	database               *repository.Database
	EspressoBaseUrl        string
	EspressoStartingBlock  uint64
	EspressoNamespace      uint64
	maxRetries             uint64
	maxDelay               time.Duration
}

func NewEspressoReaderService(
	blockchainHttpEndpoint string,
	blockchainWsEndpoint string,
	database *repository.Database,
	EspressoBaseUrl string,
	EspressoStartingBlock uint64,
	EspressoNamespace uint64,
	maxRetries uint64,
	maxDelay time.Duration,
) *EspressoReaderService {
	return &EspressoReaderService{
		blockchainHttpEndpoint: blockchainHttpEndpoint,
		blockchainWsEndpoint:   blockchainWsEndpoint,
		database:               database,
		EspressoBaseUrl:        EspressoBaseUrl,
		EspressoStartingBlock:  EspressoStartingBlock,
		EspressoNamespace:      EspressoNamespace,
		maxRetries:             maxRetries,
		maxDelay:               maxDelay,
	}
}

func (s *EspressoReaderService) Start(
	ctx context.Context,
	ready chan<- struct{},
) error {

	evmReader := s.setupEvmReader(ctx, s.database)

	espressoReader := espressoreader.NewEspressoReader(s.EspressoBaseUrl, s.EspressoStartingBlock, s.EspressoNamespace, s.database, evmReader)

	go s.setupNonceHttpServer()

	return espressoReader.Run(ctx, ready)
}

func (s *EspressoReaderService) String() string {
	return "espressoreader"
}

func (s *EspressoReaderService) setupEvmReader(ctx context.Context, database *repository.Database) *evmreader.EvmReader {
	client, err := ethclient.DialContext(ctx, s.blockchainHttpEndpoint)
	if err != nil {
		slog.Error("eth client http", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	wsClient, err := ethclient.DialContext(ctx, s.blockchainWsEndpoint)
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

	contractFactory := retrypolicy.NewEvmReaderContractFactory(client, s.maxRetries, s.maxDelay)

	evmReader := evmreader.NewEvmReader(
		retrypolicy.NewEhtClientWithRetryPolicy(client, s.maxRetries, s.maxDelay),
		retrypolicy.NewEthWsClientWithRetryPolicy(wsClient, s.maxRetries, s.maxDelay),
		retrypolicy.NewInputSourceWithRetryPolicy(inputSource, s.maxRetries, s.maxDelay),
		database,
		config.InputBoxDeploymentBlock,
		config.DefaultBlock,
		contractFactory,
	)

	return &evmReader
}

func (s *EspressoReaderService) setupNonceHttpServer() {
	http.HandleFunc("/nonce/{sender}/{dapp}", s.getNonce)
	http.HandleFunc("/submit", s.submit)

	http.ListenAndServe(":3333", nil)
}

func (s *EspressoReaderService) getNonce(w http.ResponseWriter, r *http.Request) {
	senderAddress := common.HexToAddress(r.PathValue("sender"))
	applicationAddress := common.HexToAddress(r.PathValue("dapp"))
	ctx := context.Background()

	nonce := s.process(ctx, senderAddress, applicationAddress)

	slog.Info("got nonce request")

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

func (s *EspressoReaderService) process(
	ctx context.Context,
	senderAddress common.Address,
	applicationAddress common.Address) uint64 {
	nonce, err := s.database.GetEspressoNonce(ctx, senderAddress, applicationAddress)
	if err != nil {
		slog.Error("failed to get espresso nonce", "error", err)
		os.Exit(1)
	}

	return nonce
}

func (s *EspressoReaderService) submit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("could not read body: %s\n", err)
	}
	slog.Info("got submit request", "request body", string(body))

	client := client.NewClient(s.EspressoBaseUrl)
	ctx := context.Background()
	var tx types.Transaction
	tx.UnmarshalJSON(body)
	client.SubmitTransaction(ctx, tx)
}

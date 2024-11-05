// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	blockchainHttpEndpoint  string
	blockchainWsEndpoint    string
	database                *repository.Database
	EspressoBaseUrl         string
	EspressoStartingBlock   uint64
	EspressoNamespace       uint64
	maxRetries              uint64
	maxDelay                time.Duration
	chainId                 uint64
	inputBoxDeploymentBlock uint64
	espressoServiceEndpoint string
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
	chainId uint64,
	inputBoxDeploymentBlock uint64,
	espressoServiceEndpoint string,
) *EspressoReaderService {
	return &EspressoReaderService{
		blockchainHttpEndpoint:  blockchainHttpEndpoint,
		blockchainWsEndpoint:    blockchainWsEndpoint,
		database:                database,
		EspressoBaseUrl:         EspressoBaseUrl,
		EspressoStartingBlock:   EspressoStartingBlock,
		EspressoNamespace:       EspressoNamespace,
		maxRetries:              maxRetries,
		maxDelay:                maxDelay,
		chainId:                 chainId,
		inputBoxDeploymentBlock: inputBoxDeploymentBlock,
		espressoServiceEndpoint: espressoServiceEndpoint,
	}
}

func (s *EspressoReaderService) Start(
	ctx context.Context,
	ready chan<- struct{},
) error {

	evmReader := s.setupEvmReader(ctx, s.database)

	espressoReader := espressoreader.NewEspressoReader(s.EspressoBaseUrl, s.EspressoStartingBlock, s.EspressoNamespace, s.database, evmReader, s.chainId, s.inputBoxDeploymentBlock)

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
	}
	defer client.Close()

	wsClient, err := ethclient.DialContext(ctx, s.blockchainWsEndpoint)
	if err != nil {
		slog.Error("eth client ws", "error", err)
	}
	defer wsClient.Close()

	config, err := database.GetNodeConfig(ctx)
	if err != nil {
		slog.Error("db config", "error", err)
	}

	inputSource, err := evmreader.NewInputSourceAdapter(config.InputBoxAddress, client)
	if err != nil {
		slog.Error("input source", "error", err)
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
		true,
	)

	return &evmReader
}

func (s *EspressoReaderService) setupNonceHttpServer() {
	http.HandleFunc("/nonce", s.requestNonce)
	http.HandleFunc("/submit", s.submit)

	http.ListenAndServe(s.espressoServiceEndpoint, nil)
}

type NonceRequest struct {
	// AppContract App contract address
	AppContract string `json:"app_contract"`

	// MsgSender Message sender address
	MsgSender string `json:"msg_sender"`
}

type NonceResponse struct {
	Nonce uint64 `json:"nonce"`
}

func (s *EspressoReaderService) requestNonce(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("could not read body", "err", err)
	}
	nonceRequest := &NonceRequest{}
	if err := json.Unmarshal(body, nonceRequest); err != nil {
		fmt.Println(err)
	}

	senderAddress := common.HexToAddress(nonceRequest.MsgSender)
	applicationAddress := common.HexToAddress(nonceRequest.AppContract)

	ctx := r.Context()
	nonce := s.process(ctx, senderAddress, applicationAddress)

	slog.Debug("got nonce request", "senderAddress", senderAddress, "applicationAddress", applicationAddress)

	nonceResponse := NonceResponse{Nonce: nonce}
	if err != nil {
		slog.Error("error json marshal nonce response", "err", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers")
	err = json.NewEncoder(w).Encode(nonceResponse)
	if err != nil {
		slog.Info("Internal server error",
			"service", "espresso nonce querier",
			"err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *EspressoReaderService) process(
	ctx context.Context,
	senderAddress common.Address,
	applicationAddress common.Address) uint64 {
	nonce, err := s.database.GetEspressoNonce(ctx, senderAddress, applicationAddress)
	if err != nil {
		slog.Error("failed to get espresso nonce", "error", err)
	}

	return nonce
}

type SubmitResponse struct {
	Id string `json:"id,omitempty"`
}

func (s *EspressoReaderService) submit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Headers, Origin,Accept, X-Requested-With, Content-Type, Access-Control-Request-Method, Access-Control-Request-Headers")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("could not read body", "err", err)
	}
	slog.Debug("got submit request", "request body", string(body))

	client := client.NewClient(s.EspressoBaseUrl)
	ctx := r.Context()
	var tx types.Transaction
	tx.Namespace = s.EspressoNamespace
	tx.Payload = []byte(base64.StdEncoding.EncodeToString(body))
	_, err = client.SubmitTransaction(ctx, tx)
	if err != nil {
		slog.Error("espresso tx submit error", "err", err)
		return
	}

	_, _, sigHash, err := espressoreader.ExtractSigAndData(string(tx.Payload))
	submitResponse := SubmitResponse{Id: sigHash}

	err = json.NewEncoder(w).Encode(submitResponse)
	if err != nil {
		slog.Info("Internal server error",
			"service", "espresso submit endpoint",
			"err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

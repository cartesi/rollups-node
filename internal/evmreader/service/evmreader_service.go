// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/evmreader/retrypolicy"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type (
	Address = common.Address
	Context = context.Context
)

// Service to manage InputReader lifecycle
type EvmReaderService struct {
	blockchainHttpEndpoint string
	blockchainWsEndpoint   string
	database               *repository.Database
	maxRetries             uint64
	maxDelay               time.Duration
}

func NewEvmReaderService(
	blockchainHttpEndpoint string,
	blockchainWsEndpoint string,
	database *repository.Database,
	maxRetries uint64,
	maxDelay time.Duration,
) EvmReaderService {
	return EvmReaderService{
		blockchainHttpEndpoint: blockchainHttpEndpoint,
		blockchainWsEndpoint:   blockchainWsEndpoint,
		database:               database,
		maxRetries:             maxRetries,
		maxDelay:               maxDelay,
	}
}

func (s EvmReaderService) Start(
	ctx Context,
	ready chan<- struct{},
) error {

	client, err := ethclient.DialContext(ctx, s.blockchainHttpEndpoint)
	if err != nil {
		return err
	}
	defer client.Close()

	wsClient, err := ethclient.DialContext(ctx, s.blockchainWsEndpoint)
	if err != nil {
		return err
	}
	defer wsClient.Close()

	config, err := s.database.GetNodeConfig(ctx)
	if err != nil {
		return err
	}

	inputSource, err := evmreader.NewInputSourceAdapter(config.InputBoxAddress, client)
	if err != nil {
		return err
	}

	reader := evmreader.NewEvmReader(
		retrypolicy.NewEhtClientWithRetryPolicy(client, s.maxRetries, s.maxDelay),
		retrypolicy.NewEthWsClientWithRetryPolicy(wsClient, s.maxRetries, s.maxDelay),
		retrypolicy.NewInputSourceWithRetryPolicy(inputSource, s.maxRetries, s.maxDelay),
		s.database,
		*config,
	)

	return reader.Run(ctx, ready)
}

func (s EvmReaderService) String() string {
	return "evm-reader"
}

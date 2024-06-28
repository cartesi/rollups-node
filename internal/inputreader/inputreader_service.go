// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inputreader

import (
	"context"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Service to manage InputReader lifecycle
type InputReaderService struct {
	blockchainHttpEndpoint string
	postgresEndpoint       string
	inputBoxAddress        common.Address
	inputBoxBlockNumber    uint64
	applicationAddress     common.Address
}

func NewInputReaderService(
	blockchainHttpEndpoint string,
	postgresEndpoint string,
	inputBoxAddress common.Address,
	inputBoxBlockNumber uint64,
	applicationAddress common.Address,
) InputReaderService {
	return InputReaderService{
		blockchainHttpEndpoint: blockchainHttpEndpoint,
		postgresEndpoint:       postgresEndpoint,
		inputBoxAddress:        inputBoxAddress,
		inputBoxBlockNumber:    inputBoxBlockNumber,
		applicationAddress:     applicationAddress,
	}
}

func (s InputReaderService) Start(
	ctx context.Context,
	ready chan<- struct{},
) error {

	db, err := repository.Connect(ctx, s.postgresEndpoint)

	if err != nil {
		return err
	}

	client, err := ethclient.DialContext(ctx, s.blockchainHttpEndpoint)

	if err != nil {
		return err
	}

	inputBoxWrapper, err := NewInputBoxInputSource(s.inputBoxAddress, client)

	if err != nil {
		return err
	}

	reader := newInputReader(
		client,
		inputBoxWrapper,
		db,
		s.inputBoxAddress,
		s.inputBoxBlockNumber,
		s.applicationAddress,
	)

	return reader.Start(ctx, ready)
}

func (s InputReaderService) String() string {
	return "input-reader"
}

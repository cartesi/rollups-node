// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inputreader

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/contracts/inputbox"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// InputReader reads inputs from the blockchain
type InputReader struct {
	client              EthClient
	inputSource         InputSource
	repository          InputReaderRepository
	inputBoxAddress     common.Address
	inputBoxBlockNumber uint64
	applicationAddress  common.Address
}

// Interface for Input reading
type InputSource interface {
	// Wrapper for FilterInputAdded()
	// TODO Should we check for valid inputs?
	RetrieveInputs(
		opts *bind.FilterOpts,
		appContract []common.Address,
		index []*big.Int,
	) ([]*inputbox.InputBoxInputAdded, error)
}

// Interface for the node repository
type InputReaderRepository interface {
	InsertInputsAndUpdateMostRecentFinalizedBlockNumber(
		ctx context.Context,
		inputs []*model.Input,
		blockNumber uint64,
	) error
	GetMostRecentFinalizedBlockNumber(
		ctx context.Context,
	) (uint64, error)
}

// EthClient mimics part of ethclient.Client functions to narrow down the
// interface needed by the InputReader
type EthClient interface {
	HeaderByNumber(
		ctx context.Context,
		number *big.Int,
	) (*types.Header, error)
	SubscribeNewHead(
		ctx context.Context,
		ch chan<- *types.Header,
	) (ethereum.Subscription, error)
}

func (r InputReader) String() string {
	return "input-reader"
}

// Creates a new InputReader.
func NewInputReader(
	client EthClient,
	inputSource InputSource,
	repository InputReaderRepository,
	inputBoxAddress common.Address,
	inputBoxBlockNumber uint64,
	applicationAddress common.Address,
) InputReader {
	return InputReader{
		client:              client,
		inputSource:         inputSource,
		repository:          repository,
		inputBoxAddress:     inputBoxAddress,
		inputBoxBlockNumber: inputBoxBlockNumber,
		applicationAddress:  applicationAddress,
	}
}

func (r InputReader) Start(
	ctx context.Context,
	ready chan<- struct{},
) error {
	// Check the last block processed by the the Input Reader
	storedMostRecentFinalizedBlockNumber, err := r.repository.GetMostRecentFinalizedBlockNumber(ctx)
	if err != nil {
		return err
	}

	// Safeguard: Only check blocks after InputBox was deployed
	if storedMostRecentFinalizedBlockNumber < r.inputBoxBlockNumber {
		storedMostRecentFinalizedBlockNumber = r.inputBoxBlockNumber
	}

	currentMostRecentFinalizedHeader, err := r.fetchMostRecentFinalizedHeader(ctx)
	if err != nil {
		return err
	}
	currentMostRecentFinalizedBlockNumber := currentMostRecentFinalizedHeader.Number.Uint64()

	if currentMostRecentFinalizedBlockNumber > storedMostRecentFinalizedBlockNumber {
		opts := bind.FilterOpts{
			Context: ctx,
			Start:   storedMostRecentFinalizedBlockNumber + 1,
			End:     &currentMostRecentFinalizedBlockNumber,
		}
		err = r.readInputs(ctx, &opts)
		if err != nil {
			return err
		}
	}

	return r.watchForNewInputs(ctx, ready)
}

// Fetch the most recent `finalized` header, up to what all inputs should be
// considered finalized in L1
func (r InputReader) fetchMostRecentFinalizedHeader(
	ctx context.Context,
) (*types.Header, error) {
	header, err :=
		r.client.HeaderByNumber(
			ctx,
			new(big.Int).SetInt64(rpc.FinalizedBlockNumber.Int64()))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve most recent finalized header. %v", err)
	}

	if header == nil {
		return nil, fmt.Errorf("returned header is nil")
	}
	return header, nil
}

// Read inputs from the InputSource given specific filter options.
func (r InputReader) readInputs(
	ctx context.Context,
	opts *bind.FilterOpts,
) error {
	filter := []common.Address{r.applicationAddress}

	inputsEvents, err := r.inputSource.RetrieveInputs(opts, filter, nil)
	if err != nil {
		return fmt.Errorf("failed to read inputs from block %v to block %v. %v",
			opts.Start,
			opts.End,
			err)
	}

	var inputs = []*model.Input{}
	for _, event := range inputsEvents {
		input := model.Input{
			Index:  event.Index.Uint64(),
			Status: "Enqueued",
			Blob:   event.Input,
		}
		inputs = append(inputs, &input)
	}

	err = r.repository.InsertInputsAndUpdateMostRecentFinalizedBlockNumber(
		ctx,
		inputs,
		*opts.End)
	if err != nil {
		return err
	}

	return nil
}

// Watch for new blocks and reads new inputs from finalized blocks which have not
// been processed yet.
func (r InputReader) watchForNewInputs(
	ctx context.Context,
	ready chan<- struct{},
) error {
	headers := make(chan *types.Header)
	sub, err := r.client.SubscribeNewHead(ctx, headers)
	if err != nil {
		return fmt.Errorf("could not start subscription: %v", err)
	}
	ready <- struct{}{}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sub.Err():
			return fmt.Errorf("subscription failed: %v", err)
		case <-headers:

			storedMostRecentFinalizedBlockNumber, err := r.repository.GetMostRecentFinalizedBlockNumber(ctx)
			if err != nil {
				return fmt.Errorf("failed to retrieve known most recent finalized block from repo. %v", err)
			}

			mostRecentFinalizedHeader, err := r.fetchMostRecentFinalizedHeader(ctx)

			switch {
			case err != nil:
				return fmt.Errorf("failed to retrieve most recent finalized block. %v", err)

			case storedMostRecentFinalizedBlockNumber == mostRecentFinalizedHeader.Number.Uint64():
				continue

			default:
				// TODO handle a very big gap, and probably the need to read
				// blocks in smaller chunks
				mostRecentFinalizedHeaderNumber := mostRecentFinalizedHeader.Number.Uint64()
				opts := bind.FilterOpts{
					Context: ctx,
					Start:   storedMostRecentFinalizedBlockNumber + 1,
					End:     &mostRecentFinalizedHeaderNumber,
				}
				err = r.readInputs(ctx, &opts)
				if err != nil {
					return err
				}
			}
		}
	}
}

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
	client                         EthClient
	inputSource                    InputSource
	repository                     InputReaderRepository
	inputBoxAddress                common.Address
	inputBoxBlockNumber            uint64
	applicationAddress             common.Address
	mostRecentFinalizedBlockNumber uint64
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
	InsertInput(
		ctx context.Context,
		input model.Input,
	) error
	GetMostRecentFinalizedBlockNumber(
		ctx context.Context,
	) (uint64, error)
	UpdateMostRecentFinalizedBlockNumber(
		ctx context.Context,
		number uint64,
	) error
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
	previouslyKnownMostRecentFinalizedBlockNumber, err := r.repository.GetMostRecentFinalizedBlockNumber(ctx)
	if err != nil {
		return err
	}

	// Safegaurd: Only check blocks after InputBox was deployed
	if previouslyKnownMostRecentFinalizedBlockNumber < r.inputBoxBlockNumber {
		previouslyKnownMostRecentFinalizedBlockNumber = r.inputBoxBlockNumber
	}

	currentMostRecentFinalizedHeader, err := r.fetchMostRecentFinalizedHeader(ctx)
	if err != nil {
		return err
	}
	currentMostRecentFinalizedBlockNumber := currentMostRecentFinalizedHeader.Number.Uint64()

	opts := bind.FilterOpts{
		Context: ctx,
		Start:   previouslyKnownMostRecentFinalizedBlockNumber,
		End:     &currentMostRecentFinalizedBlockNumber,
	}
	err = r.readInputs(ctx, &opts)
	if err != nil {
		return err
	}

	// TODO : Should be ready if watchForNewInputs is sucessfull
	//ready <- struct{}{}

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

	inputs, err := r.inputSource.RetrieveInputs(opts, filter, nil)
	if err != nil {
		return fmt.Errorf("failed to read inputs from block %v to block %v. %v",
			opts.Start,
			opts.End,
			err)
	}

	// TODO store most recent finalized block number along with inputs in a single
	// database transaction
	// "addInputsAndUpdateMostRecentFinalizedBlockNumber"
	for _, i := range inputs {
		if err := r.addInput(ctx, i); err != nil {
			return err
		}
	}

	err = r.repository.UpdateMostRecentFinalizedBlockNumber(
		ctx,
		*opts.End)
	if err != nil {
		return err
	}

	// If the inputs were added successfully and the DB is updated
	r.mostRecentFinalizedBlockNumber = *opts.End

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

			mostRecentFinalizedHeader, err := r.fetchMostRecentFinalizedHeader(ctx)
			switch {
			case err != nil:
				return fmt.Errorf("failed to retrieve most recent finalized block. %v", err)

			case r.mostRecentFinalizedBlockNumber == mostRecentFinalizedHeader.Number.Uint64():
				continue

			default:
				// TODO account for race condition when there might be inputs available right
				// before the subscription was created, which were not previously read on
				// start-up :
				// Most Recent FinalizedBlockNumber is only updated after All inputs are read
				// This way all the new inputs will be read after startup
				mostRecentHeaderNumber := mostRecentFinalizedHeader.Number.Uint64()
				opts := bind.FilterOpts{
					Context: ctx,
					Start:   r.mostRecentFinalizedBlockNumber + 1,
					End:     &mostRecentHeaderNumber,
				}
				err = r.readInputs(ctx, &opts)
				if err != nil {
					return err
				}
			}
		}
	}
}

// Add input to repository
func (r InputReader) addInput(
	ctx context.Context,
	event *inputbox.InputBoxInputAdded,
) error {
	input := model.Input{
		Index:  event.Index.Uint64(),
		Status: "UNPROCESSED",
		Blob:   event.Input,
	}
	return r.repository.InsertInput(ctx, input)
}

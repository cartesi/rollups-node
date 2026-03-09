// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type iclaimerBlockchain interface {
	findClaimSubmittedEventAndSucc(
		ctx context.Context,
		application *model.Application,
		epoch *model.Epoch,
		endBlock *big.Int,
	) (
		*iconsensus.IConsensus,
		*iconsensus.IConsensusClaimSubmitted,
		*iconsensus.IConsensusClaimSubmitted,
		error,
	)

	submitClaimToBlockchain(
		ic *iconsensus.IConsensus,
		application *model.Application,
		epoch *model.Epoch,
	) (common.Hash, error)

	pollTransaction(
		ctx context.Context,
		txHash common.Hash,
		endBlock *big.Int,
	) (bool, *types.Receipt, error)

	findClaimAcceptedEventAndSucc(
		ctx context.Context,
		application *model.Application,
		epoch *model.Epoch,
		endBlock *big.Int,
	) (
		*iconsensus.IConsensus,
		*iconsensus.IConsensusClaimAccepted,
		*iconsensus.IConsensusClaimAccepted,
		error,
	)

	getDefaultBlockNumber(ctx context.Context) (*big.Int, error)

	getConsensusAddress(
		ctx context.Context,
		app *model.Application,
	) (common.Address, error)
}

type claimerBlockchain struct {
	client       *ethclient.Client
	txOpts       *bind.TransactOpts
	logger       *slog.Logger
	filter       ethutil.Filter
	defaultBlock config.DefaultBlock
}

func (cb *claimerBlockchain) submitClaimToBlockchain(
	ic *iconsensus.IConsensus,
	application *model.Application,
	epoch *model.Epoch,
) (common.Hash, error) {
	txHash := common.Hash{}
	if cb.txOpts == nil {
		return txHash, fmt.Errorf("txOpts is required for claim submission")
	}
	lastBlockNumber := new(big.Int).SetUint64(epoch.LastBlock)
	tx, err := ic.SubmitClaim(cb.txOpts, application.IApplicationAddress,
		lastBlockNumber, *epoch.OutputsMerkleRoot)
	if err != nil {
		cb.logger.Error("submitClaimToBlockchain:failed",
			"appContractAddress", application.IApplicationAddress,
			"claimHash", *epoch.OutputsMerkleRoot,
			"last_block", epoch.LastBlock,
			"error", err)
	} else {
		txHash = tx.Hash()
		cb.logger.Debug("submitClaimToBlockchain:success",
			"appContractAddress", application.IApplicationAddress,
			"claimHash", *epoch.OutputsMerkleRoot,
			"last_block", epoch.LastBlock,
			"TxHash", txHash)
	}
	return txHash, err
}

type EventIterator interface {
	Next() bool
	Close() error
	Error() error
}

func newOracle(
	nr func(*bind.CallOpts) (*big.Int, error),
) (
	func(ctx context.Context, block uint64) (*big.Int, error),
) {
	return func(ctx context.Context, block uint64) (*big.Int, error) {
		return nr(&bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		})
	}
}

func newOnHit[IT EventIterator](
	ctx context.Context,
	address common.Address,
	filter func (*bind.FilterOpts, []common.Address, []common.Address) (IT, error),
	onEvent func(IT),
) (
	func(block uint64) error,
) {
	return func(block uint64) error {
		filterOpts := &bind.FilterOpts{
			Context: ctx,
			Start:   block,
			End:     &block,
		}
		it, err := filter(filterOpts, nil, []common.Address{address})
		if err != nil {
			return err
		}
		defer it.Close()
		for it.Next() {
			onEvent(it)
		}
		return it.Error()
	}
}

// scan the event stream for a claimSubmitted event that matches claim.
// return this event and its successor
func (cb *claimerBlockchain) findClaimSubmittedEventAndSucc(
	ctx context.Context,
	application *model.Application,
	epoch *model.Epoch,
	endBlock *big.Int,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimSubmitted,
	*iconsensus.IConsensusClaimSubmitted,
	error,
) {
	ic, err := iconsensus.NewIConsensus(application.IConsensusAddress, cb.client)
	if err != nil {
		return nil, nil, nil, err
	}
	oracle := newOracle(ic.GetNumberOfSubmittedClaims)
	events := []*iconsensus.IConsensusClaimSubmitted{}
	onHit := newOnHit(ctx, application.IApplicationAddress, ic.FilterClaimSubmitted,
		func(it *iconsensus.IConsensusClaimSubmittedIterator) {
			event := it.Event
			if (len(events) == 0) || claimSubmittedEventMatches(application, epoch, event) {
				events = append(events, event)
			}
		},
	)

	numSubmittedClaims, err := oracle(ctx, epoch.LastBlock)
	if err != nil {
		return nil, nil, nil, err
	}
	_, err = ethutil.FindTransitions(ctx, epoch.LastBlock, endBlock.Uint64(), numSubmittedClaims, oracle, onHit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to walk ClaimSubmitted transitions: %w", err)
	}

	if len(events) == 0 {
		return ic, nil, nil, nil
	} else if len(events) == 1 {
		return ic, events[0], nil, nil
	} else {
		return ic, events[0], events[1], nil
	}
}

// scan the event stream for a claimAccepted event that matches claim.
// return this event and its successor
func (cb *claimerBlockchain) findClaimAcceptedEventAndSucc(
	ctx context.Context,
	application *model.Application,
	epoch *model.Epoch,
	endBlock *big.Int,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimAccepted,
	*iconsensus.IConsensusClaimAccepted,
	error,
) {
	ic, err := iconsensus.NewIConsensus(application.IConsensusAddress, cb.client)
	if err != nil {
		return nil, nil, nil, err
	}

	oracle := newOracle(ic.GetNumberOfAcceptedClaims)
	events := []*iconsensus.IConsensusClaimAccepted{}
	filter := func(
		opts *bind.FilterOpts,
		_ []common.Address,
		appContract []common.Address,
	) (*iconsensus.IConsensusClaimAcceptedIterator, error) {
		return ic.FilterClaimAccepted(opts, appContract)
	}
	onHit := newOnHit(ctx, application.IApplicationAddress, filter,
		func(it *iconsensus.IConsensusClaimAcceptedIterator) {
			event := it.Event
			if (len(events) == 0) || claimAcceptedEventMatches(application, epoch, event) {
				events = append(events, event)
			}
		},
	)

	numAcceptedClaims, err := oracle(ctx, epoch.LastBlock)
	if err != nil {
		return nil, nil, nil, err
	}
	_, err = ethutil.FindTransitions(ctx, epoch.LastBlock, endBlock.Uint64(), numAcceptedClaims, oracle, onHit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to walk ClaimAccepted transitions: %w", err)
	}

	if len(events) == 0 {
		return ic, nil, nil, nil
	} else if len(events) == 1 {
		return ic, events[0], nil, nil
	} else {
		return ic, events[0], events[1], nil
	}
}

func (cb *claimerBlockchain) getConsensusAddress(
	ctx context.Context,
	app *model.Application,
) (common.Address, error) {
	return ethutil.GetConsensus(ctx, cb.client, app.IApplicationAddress)
}

// poll a transaction for its receipt
func (cb *claimerBlockchain) pollTransaction(
	ctx context.Context,
	txHash common.Hash,
	endBlockNumber *big.Int,
) (bool, *types.Receipt, error) {
	receipt, err := cb.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return false, nil, err
	}

	// receipt must be committed before use. Return false until it is.
	if receipt.BlockNumber.Cmp(endBlockNumber) >= 0 {
		return false, nil, nil
	}

	return receipt.Status == 1, receipt, nil
}

/* Retrieve the block number for the configured commitment level in ethereum terms,
 * that is: `latest`, `safe`, `finalized`, etc. Which may be many blocks behind. */
func (cb *claimerBlockchain) getDefaultBlockNumber(ctx context.Context) (*big.Int, error) {
	var nr int64
	switch cb.defaultBlock {
	case model.DefaultBlock_Pending:
		nr = rpc.PendingBlockNumber.Int64()
	case model.DefaultBlock_Latest:
		nr = rpc.LatestBlockNumber.Int64()
	case model.DefaultBlock_Finalized:
		nr = rpc.FinalizedBlockNumber.Int64()
	case model.DefaultBlock_Safe:
		nr = rpc.SafeBlockNumber.Int64()
	default:
		return nil, fmt.Errorf("default block '%v' not supported", cb.defaultBlock)
	}

	hdr, err := cb.client.HeaderByNumber(ctx, big.NewInt(nr))
	if err != nil {
		return nil, err
	}
	return hdr.Number, nil
}

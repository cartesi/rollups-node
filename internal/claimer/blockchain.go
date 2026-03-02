// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"math/big"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
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

	getBlockNumber(ctx context.Context) (*big.Int, error)

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

func takeEventAndParse[T any](
	pull func() (log *types.Log, err error, ok bool),
	parse func(log types.Log) (*T, error),
) (*T, bool, error) {
	log, err, ok := pull()
	if !ok || err != nil {
		return nil, false, err
	}
	ev, err := parse(*log)
	return ev, err == nil, err
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

	// filter must match:
	// - `ClaimSubmitted` events
	// - submitter == nil (any)
	// - appContract == claim.IApplicationAddress
	c, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		return nil, nil, nil, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[model.MonitoredEvent_ClaimSubmitted.String()].ID},
		nil,
		[]any{application.IApplicationAddress},
	)
	if err != nil {
		return nil, nil, nil, err
	}

	it, err := cb.filter.ChunkedFilterLogs(ctx, cb.client, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(epoch.LastBlock),
		ToBlock:   endBlock,
		Addresses: []common.Address{application.IConsensusAddress},
		Topics:    topics,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// pull events instead of iterating
	next, stop := iter.Pull2(it)
	defer stop()
	for {
		event, ok, err := takeEventAndParse(next, ic.ParseClaimSubmitted)
		if !ok || err != nil {
			return ic, event, nil, err
		}
		lastBlock := event.LastProcessedBlockNumber.Uint64()

		if claimSubmittedEventMatches(application, epoch, event) {
			// found the event, does it has a successor? try to fetch it
			succ, ok, err := takeEventAndParse(next, ic.ParseClaimSubmitted)
			if !ok || err != nil {
				return ic, event, nil, err
			}
			return ic, event, succ, err
		} else if lastBlock > epoch.LastBlock {
			err = fmt.Errorf("No matching claim, searched up to %v", event)
			return nil, nil, nil, err
		}
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

	// filter must match:
	// - `ClaimAccepted` events
	// - appContract == claim.IApplicationAddress
	c, err := iconsensus.IConsensusMetaData.GetAbi()
	topics, err := abi.MakeTopics(
		[]any{c.Events[model.MonitoredEvent_ClaimAccepted.String()].ID},
		[]any{application.IApplicationAddress},
	)
	if err != nil {
		return nil, nil, nil, err
	}

	it, err := cb.filter.ChunkedFilterLogs(ctx, cb.client, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(epoch.LastBlock),
		ToBlock:   endBlock,
		Addresses: []common.Address{application.IConsensusAddress},
		Topics:    topics,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// pull events instead of iterating
	next, stop := iter.Pull2(it)
	defer stop()
	for {
		event, ok, err := takeEventAndParse(next, ic.ParseClaimAccepted)
		if !ok || err != nil {
			return ic, event, nil, err
		}
		lastBlock := event.LastProcessedBlockNumber.Uint64()

		if claimAcceptedEventMatches(application, epoch, event) {
			// found the event, does it has a successor? try to fetch it
			succ, ok, err := takeEventAndParse(next, ic.ParseClaimAccepted)
			if !ok || err != nil {
				return ic, event, nil, err
			}
			return ic, event, succ, err
		} else if lastBlock > epoch.LastBlock {
			err = fmt.Errorf("No matching claim, searched up to %v", event)
			return nil, nil, nil, err
		}
	}
}

func (cb *claimerBlockchain) getConsensusAddress(
	ctx context.Context,
	app *model.Application,
) (common.Address, error) {
	return ethutil.GetConsensus(ctx, cb.client, app.IApplicationAddress)
}

// poll a transaction hash for its submission status and receipt. Wait until
// the receipt is older than commitment to avoid speculative execution, we don't
// want to be subject to reorgs.
// receipt returned value is only valid when (ready == true && error == nil)
func (cb *claimerBlockchain) pollTransaction(
	ctx context.Context,
	txHash common.Hash,
	commitmentBlockNumber *big.Int,
) (bool, *types.Receipt, error) {
	_, isPending, err := cb.client.TransactionByHash(ctx, txHash)
	if err != nil || isPending {
		return false, nil, err
	}

	receipt, err := cb.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return false, nil, err
	}

	// wait until receipt is older than commitment.
	if receipt.BlockNumber.Cmp(commitmentBlockNumber) >= 0 {
		return false, nil, err
	}

	return receipt.Status == 1, receipt, err
}

// Retrieve the block number of a commitment level
func (cb *claimerBlockchain) getBlockNumber(ctx context.Context) (*big.Int, error) {
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

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

func (self *claimerBlockchain) submitClaimToBlockchain(
	ic *iconsensus.IConsensus,
	application *model.Application,
	epoch *model.Epoch,
) (common.Hash, error) {
	txHash := common.Hash{}
	lastBlockNumber := new(big.Int).SetUint64(epoch.LastBlock)
	tx, err := ic.SubmitClaim(self.txOpts, application.IApplicationAddress,
		lastBlockNumber, *epoch.OutputsMerkleRoot)
	if err != nil {
		self.logger.Error("submitClaimToBlockchain:failed",
			"appContractAddress", application.IApplicationAddress,
			"claimHash", *epoch.OutputsMerkleRoot,
			"last_block", epoch.LastBlock,
			"error", err)
	} else {
		txHash = tx.Hash()
		self.logger.Debug("submitClaimToBlockchain:success",
			"appContractAddress", application.IApplicationAddress,
			"claimHash", *epoch.OutputsMerkleRoot,
			"last_block", epoch.LastBlock,
			"TxHash", txHash)
	}
	return txHash, err
}

func unwrapClaimSubmitted(
	ic *iconsensus.IConsensus,
	pull func() (log *types.Log, err error, ok bool),
) (
	*iconsensus.IConsensusClaimSubmitted,
	bool,
	error,
) {
	log, err, ok := pull()
	if !ok || err != nil {
		return nil, false, err
	}
	ev, err := ic.ParseClaimSubmitted(*log)
	return ev, true, err
}

// scan the event stream for a claimSubmitted event that matches claim.
// return this event and its successor
func (self *claimerBlockchain) findClaimSubmittedEventAndSucc(
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
	ic, err := iconsensus.NewIConsensus(application.IConsensusAddress, self.client)
	if err != nil {
		return nil, nil, nil, err
	}

	// filter must match:
	// - `ClaimSubmitted` events
	// - submitter == nil (any)
	// - appContract == claim.IApplicationAddress
	c, err := iconsensus.IConsensusMetaData.GetAbi()
	topics, err := abi.MakeTopics(
		[]any{c.Events[model.MonitoredEvent_ClaimSubmitted.String()].ID},
		nil,
		[]any{application.IApplicationAddress},
	)
	if err != nil {
		return nil, nil, nil, err
	}

	it, err := self.filter.ChunkedFilterLogs(ctx, self.client, ethereum.FilterQuery{
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
		event, ok, err := unwrapClaimSubmitted(ic, next)
		if !ok || err != nil {
			return ic, event, nil, err
		}
		lastBlock := event.LastProcessedBlockNumber.Uint64()

		if claimSubmittedEventMatches(application, epoch, event) {
			// found the event, does it has a successor? try to fetch it
			succ, ok, err := unwrapClaimSubmitted(ic, next)
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

func unwrapClaimAccepted(
	ic *iconsensus.IConsensus,
	pull func() (log *types.Log, err error, ok bool),
) (
	*iconsensus.IConsensusClaimAccepted,
	bool,
	error,
) {
	log, err, ok := pull()
	if !ok || err != nil {
		return nil, false, err
	}
	ev, err := ic.ParseClaimAccepted(*log)
	return ev, true, err
}

// scan the event stream for a claimAccepted event that matches claim.
// return this event and its successor
func (self *claimerBlockchain) findClaimAcceptedEventAndSucc(
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
	ic, err := iconsensus.NewIConsensus(application.IConsensusAddress, self.client)
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

	it, err := self.filter.ChunkedFilterLogs(ctx, self.client, ethereum.FilterQuery{
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
		event, ok, err := unwrapClaimAccepted(ic, next)
		if !ok || err != nil {
			return ic, event, nil, err
		}
		lastBlock := event.LastProcessedBlockNumber.Uint64()

		if claimAcceptedEventMatches(application, epoch, event) {
			// found the event, does it has a successor? try to fetch it
			succ, ok, err := unwrapClaimAccepted(ic, next)
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

func (self *claimerBlockchain) getConsensusAddress(
	ctx context.Context,
	app *model.Application,
) (common.Address, error) {
	return ethutil.GetConsensus(ctx, self.client, app.IApplicationAddress)
}

/* poll a transaction hash for its submission status and receipt */
func (self *claimerBlockchain) pollTransaction(
	ctx context.Context,
	txHash common.Hash,
	endBlock *big.Int,
) (bool, *types.Receipt, error) {
	_, isPending, err := self.client.TransactionByHash(ctx, txHash)
	if err != nil || isPending {
		return false, nil, err
	}

	receipt, err := self.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return false, nil, err
	}

	if receipt.BlockNumber.Cmp(endBlock) >= 0 {
		return false, receipt, err
	}

	return receipt.Status == 1, receipt, err
}

/* Retrieve the block number of "DefaultBlock" */
func (self *claimerBlockchain) getBlockNumber(ctx context.Context) (*big.Int, error) {
	var nr int64
	switch self.defaultBlock {
	case model.DefaultBlock_Pending:
		nr = rpc.PendingBlockNumber.Int64()
	case model.DefaultBlock_Latest:
		nr = rpc.LatestBlockNumber.Int64()
	case model.DefaultBlock_Finalized:
		nr = rpc.FinalizedBlockNumber.Int64()
	case model.DefaultBlock_Safe:
		nr = rpc.SafeBlockNumber.Int64()
	default:
		return nil, fmt.Errorf("default block '%v' not supported", self.defaultBlock)
	}

	hdr, err := self.client.HeaderByNumber(ctx, big.NewInt(nr))
	if err != nil {
		return nil, err
	}
	return hdr.Number, nil
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type iclaimerBlockchain interface {
	claimTransactionCanonical(ctx context.Context, hash common.Hash, block uint64) (bool, error)
	findClaimSubmittedEventAndSucc(
		ctx context.Context,
		application *model.Application,
		epoch *model.Epoch,
		fromBlock uint64,
		toBlock uint64,
	) (
		*iconsensus.IConsensus,
		[]*iconsensus.IConsensusClaimSubmitted,
		error,
	)

	findClaimStagedEventAndSucc(
		ctx context.Context,
		application *model.Application,
		epoch *model.Epoch,
		fromBlock uint64,
		toBlock uint64,
	) (
		*iconsensus.IConsensus,
		*iconsensus.IConsensusClaimStaged,
		*iconsensus.IConsensusClaimStaged,
		error,
	)

	getClaimStatus(
		ctx context.Context,
		application *model.Application,
		epoch *model.Epoch,
		blockNumber *big.Int,
	) (iconsensus.IConsensusClaim, error)

	findClaimAcceptedEventAndSucc(
		ctx context.Context,
		application *model.Application,
		epoch *model.Epoch,
		fromBlock uint64,
		toBlock uint64,
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
		blockNumber *big.Int,
	) (common.Address, error)

	claimSubmitterAddress() (common.Address, bool)
}

func (cb *claimerBlockchain) claimTransactionCanonical(ctx context.Context, hash common.Hash, block uint64) (bool, error) {
	receipt, err := cb.client.TransactionReceipt(ctx, hash)
	if errors.Is(err, ethereum.NotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if receipt == nil || receipt.BlockNumber == nil || !receipt.BlockNumber.IsUint64() || receipt.BlockNumber.Uint64() > block {
		return false, txsender.ErrInconclusive
	}
	if receipt.TxHash != hash || receipt.BlockHash == (common.Hash{}) || receipt.Status > types.ReceiptStatusSuccessful {
		return false, txsender.ErrInconclusive
	}
	if receipt.Status == types.ReceiptStatusFailed {
		return false, nil
	}
	return cb.backend.Canonical(ctx, txsender.BlockRef{Number: receipt.BlockNumber.Uint64(), Hash: receipt.BlockHash})
}

type claimerBlockchain struct {
	client        *ethclient.Client
	txOptsFactory ethutil.TransactOptsFactory
	logger        *slog.Logger
	defaultBlock  config.DefaultBlock
	observed      txsender.BlockRef
	backend       txsender.Backend
	fault         error
}

func (cb *claimerBlockchain) claimSubmitterAddress() (common.Address, bool) {
	if cb.txOptsFactory == nil {
		return common.Address{}, false
	}
	return cb.txOptsFactory.From(), true
}

type eventIterator interface {
	Next() bool
	Close() error
	Error() error
}

// newOracle adapts a contract counter getter to the function shape expected by
// ethutil.FindTransitions. The getter is called for one app at one block.
func (cb *claimerBlockchain) newOracle(
	addr common.Address,
	nr func(*bind.CallOpts, common.Address) (*big.Int, error),
) func(ctx context.Context, block uint64) (*big.Int, error) {
	return func(ctx context.Context, block uint64) (*big.Int, error) {
		opts := cb.callOpts(ctx, new(big.Int).SetUint64(block))
		if opts.BlockHash == (common.Hash{}) {
			header, err := cb.client.HeaderByNumber(ctx, new(big.Int).SetUint64(block))
			if err != nil {
				return nil, err
			}
			if header == nil || header.Number == nil || header.Number.Uint64() != block {
				return nil, txsender.ErrInconclusive
			}
			opts = &bind.CallOpts{Context: ctx, BlockHash: header.Hash()}
		}
		return nr(opts, addr)
	}
}

// priorCounter reads the counter at the block before fromBlock. That value is
// used as the previous value for ethutil.FindTransitions.
//
// When fromBlock is zero, there is no earlier block to read. In that case this
// returns nil, and FindTransitions starts without a previous value.
//
// FindTransitions needs the counter value from just before the scan starts.
// That is why this uses oracle(fromBlock-1).
//
// Do not use epoch.LastBlock here. For scans that start after a previous
// epoch, epoch.LastBlock can be after events that are inside the scan window.
// Reading the counter there would make the previous value too high.
func priorCounter(
	ctx context.Context,
	oracle func(ctx context.Context, block uint64) (*big.Int, error),
	fromBlock uint64,
) (*big.Int, error) {
	if fromBlock == 0 {
		return nil, nil
	}
	return oracle(ctx, fromBlock-1)
}

func newOnHit[IT eventIterator](
	ctx context.Context,
	address common.Address,
	filter func(*bind.FilterOpts, []common.Address, []common.Address) (IT, error),
	oracle func(context.Context, uint64) (*big.Int, error),
	onEvent func(IT) error,
) func(block uint64) error {
	return func(block uint64) (err error) {
		before := new(big.Int)
		if block > 0 {
			before, err = oracle(ctx, block-1)
			if err != nil {
				return err
			}
		}
		after, err := oracle(ctx, block)
		if err != nil {
			return err
		}
		if before == nil || after == nil || after.Cmp(before) < 0 {
			return txsender.ErrInconclusive
		}
		expected := new(big.Int).Sub(after, before)
		filterOpts := &bind.FilterOpts{
			Context: ctx,
			Start:   block,
			End:     &block,
		}
		it, err := filter(filterOpts, nil, []common.Address{address})
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, it.Close()) }()
		var count uint64
		for it.Next() {
			count++
			if err := onEvent(it); err != nil {
				return err
			}
		}
		if err := it.Error(); err != nil {
			return err
		}
		if expected.Cmp(new(big.Int).SetUint64(count)) != 0 {
			return fmt.Errorf("%w: event window at block %d returned %d logs, counter advanced %s",
				txsender.ErrInconclusive, block, count, expected)
		}
		return nil
	}
}

// findClaimSubmittedEventAndSucc scans for ClaimSubmitted events for this
// epoch. It returns the matching event and later events in the same stream.
// The caller then decides whether each event is our claim or a different one.
func (cb *claimerBlockchain) findClaimSubmittedEventAndSucc(
	ctx context.Context,
	application *model.Application,
	epoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	[]*iconsensus.IConsensusClaimSubmitted,
	error,
) {
	ic, err := iconsensus.NewIConsensus(application.IConsensusAddress, cb.client)
	if err != nil {
		return nil, nil, fmt.Errorf("creating IConsensus binding for submitted events of application: %v, epoch: %v (%v): %w",
			application.IApplicationAddress, epoch.Index, epoch.VirtualIndex, err)
	}
	caller, err := iconsensus.NewIConsensusCaller(application.IConsensusAddress, txsender.ProtectedClient{Client: cb.client})
	if err != nil {
		return nil, nil, err
	}
	oracle := cb.newOracle(application.IApplicationAddress, caller.GetNumberOfSubmittedClaims)
	events := []*iconsensus.IConsensusClaimSubmitted{}
	onHit := newOnHit(ctx, application.IApplicationAddress, ic.FilterClaimSubmitted, oracle,
		func(it *iconsensus.IConsensusClaimSubmittedIterator) error {
			event := it.Event
			if err := cb.validateObservedLog(ctx, event.Raw, toBlock); err != nil {
				return err
			}
			if (len(events) > 0) || claimSubmittedEventMatchesEpoch(application, epoch, event) {
				events = append(events, event)
			}
			return nil
		},
	)

	prevValue, err := priorCounter(ctx, oracle, fromBlock)
	if err != nil {
		return nil, nil, fmt.Errorf("querying number of submitted claims for epoch %v (%v) before block %d: %w",
			epoch.Index, epoch.VirtualIndex, fromBlock, err)
	}
	_, err = ethutil.FindTransitions(ctx, fromBlock, toBlock, prevValue, oracle, onHit)
	if err != nil {
		return nil, nil, fmt.Errorf("walking ClaimSubmitted transitions for application: %v, epoch %v (%v): %w",
			application.IApplicationAddress, epoch.Index, epoch.VirtualIndex, err)
	}

	return ic, events, nil
}

// findClaimStagedEventAndSucc scans for a ClaimStaged event for this epoch.
// It returns the first matching event and the next event after it, if any.
//
// The scan matches only app and lastProcessedBlockNumber. It does not require
// the Merkle roots to match. This is intentional: the caller must still see a
// staged event for this epoch even when the event contains different roots.
func (cb *claimerBlockchain) findClaimStagedEventAndSucc(
	ctx context.Context,
	application *model.Application,
	epoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimStaged,
	*iconsensus.IConsensusClaimStaged,
	error,
) {
	ic, err := iconsensus.NewIConsensus(application.IConsensusAddress, cb.client)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating IConsensus binding for staged events: %w", err)
	}

	caller, err := iconsensus.NewIConsensusCaller(application.IConsensusAddress, txsender.ProtectedClient{Client: cb.client})
	if err != nil {
		return nil, nil, nil, err
	}
	oracle := cb.newOracle(application.IApplicationAddress, caller.GetNumberOfStagedClaims)
	events := []*iconsensus.IConsensusClaimStaged{}
	filter := func(
		opts *bind.FilterOpts,
		_ []common.Address,
		appContract []common.Address,
	) (*iconsensus.IConsensusClaimStagedIterator, error) {
		return ic.FilterClaimStaged(opts, appContract)
	}
	onHit := newOnHit(ctx, application.IApplicationAddress, filter, oracle,
		func(it *iconsensus.IConsensusClaimStagedIterator) error {
			event := it.Event
			if err := cb.validateObservedLog(ctx, event.Raw, toBlock); err != nil {
				return err
			}
			if (len(events) > 0) || claimStagedEventMatchesEpoch(application, epoch, event) {
				events = append(events, event)
			}
			return nil
		},
	)

	prevValue, err := priorCounter(ctx, oracle, fromBlock)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("querying number of staged claims before block %d: %w", fromBlock, err)
	}
	_, err = ethutil.FindTransitions(ctx, fromBlock, toBlock, prevValue, oracle, onHit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("walking ClaimStaged transitions for application: %v, epoch %v (%v): %w",
			application.IApplicationAddress, epoch.Index, epoch.VirtualIndex, err)
	}

	switch len(events) {
	case 0:
		return ic, nil, nil, nil
	case 1:
		return ic, events[0], nil, nil
	default:
		return ic, events[0], events[1], nil
	}
}

// findClaimAcceptedEventAndSucc scans for a ClaimAccepted event for this
// epoch. It returns the first event with matching app and lastBlock, plus its
// successor, if any. The caller compares roots to distinguish our claim from a
// different claim for the same epoch.
func (cb *claimerBlockchain) findClaimAcceptedEventAndSucc(
	ctx context.Context,
	application *model.Application,
	epoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimAccepted,
	*iconsensus.IConsensusClaimAccepted,
	error,
) {
	ic, err := iconsensus.NewIConsensus(application.IConsensusAddress, cb.client)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating IConsensus binding for accepted events: %w", err)
	}

	caller, err := iconsensus.NewIConsensusCaller(application.IConsensusAddress, txsender.ProtectedClient{Client: cb.client})
	if err != nil {
		return nil, nil, nil, err
	}
	oracle := cb.newOracle(application.IApplicationAddress, caller.GetNumberOfAcceptedClaims)
	events := []*iconsensus.IConsensusClaimAccepted{}
	filter := func(
		opts *bind.FilterOpts,
		_ []common.Address,
		appContract []common.Address,
	) (*iconsensus.IConsensusClaimAcceptedIterator, error) {
		return ic.FilterClaimAccepted(opts, appContract)
	}
	onHit := newOnHit(ctx, application.IApplicationAddress, filter, oracle,
		func(it *iconsensus.IConsensusClaimAcceptedIterator) error {
			event := it.Event
			if err := cb.validateObservedLog(ctx, event.Raw, toBlock); err != nil {
				return err
			}
			// Match only app and lastBlock. Do not require the Merkle root to
			// match here. The caller still needs to see a ClaimAccepted event
			// from a different claim, especially in Quorum.
			if (len(events) > 0) || claimAcceptedEventMatchesEpoch(application, epoch, event) {
				events = append(events, event)
			}
			return nil
		},
	)

	prevValue, err := priorCounter(ctx, oracle, fromBlock)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("querying number of accepted claims before block %d: %w", fromBlock, err)
	}
	_, err = ethutil.FindTransitions(ctx, fromBlock, toBlock, prevValue, oracle, onHit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("walking ClaimAccepted transitions for application: %v, epoch %v (%v): %w",
			application.IApplicationAddress, epoch.Index, epoch.VirtualIndex, err)
	}

	switch len(events) {
	case 0:
		return ic, nil, nil, nil
	case 1:
		return ic, events[0], nil, nil
	default:
		return ic, events[0], events[1], nil
	}
}

func (cb *claimerBlockchain) getConsensusAddress(
	ctx context.Context,
	app *model.Application,
	blockNumber *big.Int,
) (common.Address, error) {
	binding, err := iapplication.NewIApplicationCaller(app.IApplicationAddress, txsender.ProtectedClient{Client: cb.client})
	if err != nil {
		return common.Address{}, err
	}
	return binding.GetOutputsMerkleRootValidator(cb.callOpts(ctx, blockNumber))
}

// getClaimStatus reads the on-chain Claim record for this app, last processed
// block, and machine root.
//
// The caller passes the tick's configured observation block. That makes all chain
// reads in the same tick use the same block.
func (cb *claimerBlockchain) getClaimStatus(
	ctx context.Context,
	application *model.Application,
	epoch *model.Epoch,
	blockNumber *big.Int,
) (iconsensus.IConsensusClaim, error) {
	var zero iconsensus.IConsensusClaim
	if epoch.MachineHash == nil {
		return zero, fmt.Errorf(
			"epoch %d (%d) has no machine_hash; cannot query getClaim",
			epoch.Index, epoch.VirtualIndex)
	}
	ic, err := iconsensus.NewIConsensusCaller(application.IConsensusAddress, txsender.ProtectedClient{Client: cb.client})
	if err != nil {
		return zero, fmt.Errorf("creating IConsensus caller for getClaim: %w", err)
	}
	opts := cb.callOpts(ctx, blockNumber)
	return ic.GetClaim(opts, application.IApplicationAddress,
		new(big.Int).SetUint64(epoch.LastBlock), *epoch.MachineHash)
}

/* Retrieve the block number for the configured commitment level in ethereum terms,
 * that is: `latest`, `safe`, `finalized`, etc. Which may be many blocks behind. */
func (cb *claimerBlockchain) getDefaultBlockNumber(ctx context.Context) (*big.Int, error) {
	if cb.fault != nil {
		return nil, cb.fault
	}
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
		return nil, fmt.Errorf("fetching header for block %v: %w", nr, err)
	}
	if hdr == nil || hdr.Number == nil || !hdr.Number.IsUint64() {
		return nil, fmt.Errorf("returned header for block %v is invalid", nr)
	}
	candidate := txsender.BlockRef{Number: hdr.Number.Uint64(), Hash: hdr.Hash()}
	canonical, err := cb.backend.Canonical(ctx, candidate)
	if err != nil {
		return nil, err
	}
	if !canonical {
		return nil, txsender.ErrInconclusive
	}
	if cb.observed.Hash != (common.Hash{}) && cb.defaultBlock == model.DefaultBlock_Finalized {
		if hdr.Number.Uint64() < cb.observed.Number {
			return nil, txsender.ErrInconclusive
		}
		savedHeader, err := cb.client.HeaderByNumber(ctx, new(big.Int).SetUint64(cb.observed.Number))
		if err != nil {
			return nil, err
		}
		if savedHeader == nil || savedHeader.Number == nil || savedHeader.Number.Uint64() != cb.observed.Number {
			return nil, txsender.ErrInconclusive
		}
		if savedHeader.Hash() != cb.observed.Hash {
			cb.fault = fmt.Errorf("%w: finalized block %d saved %s now %s",
				txsender.ErrFinality, cb.observed.Number, cb.observed.Hash, savedHeader.Hash())
			return nil, cb.fault
		}
	}
	cb.observed = candidate
	return hdr.Number, nil
}

// Current observations use the exact header selected at the start of Observe.
// The protected adapter requires that hash to remain canonical on this backend.
func (cb *claimerBlockchain) callOpts(ctx context.Context, block *big.Int) *bind.CallOpts {
	if block != nil && block.IsUint64() && block.Uint64() == cb.observed.Number && cb.observed.Hash != (common.Hash{}) {
		return &bind.CallOpts{Context: ctx, BlockHash: cb.observed.Hash}
	}
	return &bind.CallOpts{Context: ctx, BlockNumber: block}
}

func (cb *claimerBlockchain) validateObservedLog(ctx context.Context, log types.Log, end uint64) error {
	if log.Removed || log.BlockNumber > end || log.BlockHash == (common.Hash{}) {
		return txsender.ErrInconclusive
	}
	canonical, err := cb.backend.Canonical(ctx, txsender.BlockRef{Number: log.BlockNumber, Hash: log.BlockHash})
	if err != nil {
		return err
	}
	if !canonical {
		return txsender.ErrNonCanonical
	}
	return nil
}

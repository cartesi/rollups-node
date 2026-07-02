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
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum"
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

	submitClaimToBlockchain(
		ctx context.Context,
		ic *iconsensus.IConsensus,
		application *model.Application,
		epoch *model.Epoch,
	) (common.Hash, error)

	acceptClaimOnBlockchain(
		ctx context.Context,
		application *model.Application,
		epoch *model.Epoch,
	) (common.Hash, error)

	getClaimStatus(
		ctx context.Context,
		application *model.Application,
		epoch *model.Epoch,
		blockNumber *big.Int,
	) (iconsensus.IConsensusClaim, error)

	pollTransaction(
		ctx context.Context,
		txHash common.Hash,
		endBlock *big.Int,
	) (bool, *types.Receipt, error)

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

type claimerBlockchain struct {
	client        *ethclient.Client
	txOptsFactory ethutil.TransactOptsFactory
	logger        *slog.Logger
	defaultBlock  config.DefaultBlock
}

func (cb *claimerBlockchain) claimSubmitterAddress() (common.Address, bool) {
	if cb.txOptsFactory == nil {
		return common.Address{}, false
	}
	return cb.txOptsFactory.From(), true
}

func (cb *claimerBlockchain) submitClaimToBlockchain(
	ctx context.Context,
	ic *iconsensus.IConsensus,
	application *model.Application,
	epoch *model.Epoch,
) (common.Hash, error) {
	txHash := common.Hash{}
	if cb.txOptsFactory == nil {
		return txHash, fmt.Errorf("txOptsFactory is required for claim submission")
	}
	if epoch.OutputsMerkleRoot == nil {
		return txHash, fmt.Errorf(
			"epoch %d (%d) has no outputs_merkle_root; refusing to submit claim",
			epoch.Index, epoch.VirtualIndex)
	}
	// The DB trigger checks outputs_merkle_proof when an epoch moves to
	// CLAIM_COMPUTED. It does not stop a later UPDATE from clearing the proof.
	// Submitting without a proof would revert on chain, so fail here with a
	// clear local error.
	if epoch.OutputsMerkleProof == nil {
		return txHash, fmt.Errorf(
			"epoch %d (%d) has no outputs_merkle_proof; refusing to submit claim",
			epoch.Index, epoch.VirtualIndex)
	}
	proof := make([][32]byte, len(epoch.OutputsMerkleProof))
	for i, h := range epoch.OutputsMerkleProof {
		proof[i] = h
	}
	txOpts, err := cb.txOptsFactory.NewTransactOpts(ctx)
	if err != nil {
		return txHash, fmt.Errorf("creating transaction options for claim submission: %w", err)
	}
	lastBlockNumber := new(big.Int).SetUint64(epoch.LastBlock)
	tx, err := ic.SubmitClaim(txOpts, application.IApplicationAddress,
		lastBlockNumber, *epoch.OutputsMerkleRoot, proof)
	if err != nil {
		cb.logger.Warn("submitClaimToBlockchain:failed",
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

type eventIterator interface {
	Next() bool
	Close() error
	Error() error
}

// newOracle adapts a contract counter getter to the function shape expected by
// ethutil.FindTransitions. The getter is called for one app at one block.
func newOracle(
	addr common.Address,
	nr func(*bind.CallOpts, common.Address) (*big.Int, error),
) func(ctx context.Context, block uint64) (*big.Int, error) {
	return func(ctx context.Context, block uint64) (*big.Int, error) {
		return nr(&bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		}, addr)
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
	onEvent func(IT),
) func(block uint64) error {
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
	oracle := newOracle(application.IApplicationAddress, ic.GetNumberOfSubmittedClaims)
	events := []*iconsensus.IConsensusClaimSubmitted{}
	onHit := newOnHit(ctx, application.IApplicationAddress, ic.FilterClaimSubmitted,
		func(it *iconsensus.IConsensusClaimSubmittedIterator) {
			event := it.Event
			if (len(events) > 0) || claimSubmittedEventMatchesEpoch(application, epoch, event) {
				events = append(events, event)
			}
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

	oracle := newOracle(application.IApplicationAddress, ic.GetNumberOfStagedClaims)
	events := []*iconsensus.IConsensusClaimStaged{}
	filter := func(
		opts *bind.FilterOpts,
		_ []common.Address,
		appContract []common.Address,
	) (*iconsensus.IConsensusClaimStagedIterator, error) {
		return ic.FilterClaimStaged(opts, appContract)
	}
	onHit := newOnHit(ctx, application.IApplicationAddress, filter,
		func(it *iconsensus.IConsensusClaimStagedIterator) {
			event := it.Event
			if (len(events) > 0) || claimStagedEventMatchesEpoch(application, epoch, event) {
				events = append(events, event)
			}
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

	if len(events) == 0 {
		return ic, nil, nil, nil
	} else if len(events) == 1 {
		return ic, events[0], nil, nil
	} else {
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

	oracle := newOracle(application.IApplicationAddress, ic.GetNumberOfAcceptedClaims)
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
			// Match only app and lastBlock. Do not require the Merkle root to
			// match here. The caller still needs to see a ClaimAccepted event
			// from a different claim, especially in Quorum.
			if (len(events) > 0) || claimAcceptedEventMatchesEpoch(application, epoch, event) {
				events = append(events, event)
			}
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
	blockNumber *big.Int,
) (common.Address, error) {
	return ethutil.GetConsensusAt(ctx, cb.client, app.IApplicationAddress, blockNumber)
}

// acceptClaimOnBlockchain calls IConsensus.acceptClaim for an epoch whose
// claim is already STAGED on chain and whose staging period has elapsed.
// The contract validates the period server-side and reverts with
// ClaimStagingPeriodNotOverYet if the math is off; the caller handles that
// revert via handleAcceptClaimRevert.
func (cb *claimerBlockchain) acceptClaimOnBlockchain(
	ctx context.Context,
	application *model.Application,
	epoch *model.Epoch,
) (common.Hash, error) {
	txHash := common.Hash{}
	if cb.txOptsFactory == nil {
		return txHash, fmt.Errorf("txOpts is required for claim acceptance")
	}
	if epoch.MachineHash == nil {
		return txHash, fmt.Errorf(
			"epoch %d (%d) has no machine_hash; refusing to accept claim",
			epoch.Index, epoch.VirtualIndex)
	}
	ic, err := iconsensus.NewIConsensus(application.IConsensusAddress, cb.client)
	if err != nil {
		return txHash, fmt.Errorf("creating IConsensus binding for acceptClaim: %w", err)
	}
	txOpts, err := cb.txOptsFactory.NewTransactOpts(ctx)
	if err != nil {
		return txHash, fmt.Errorf("creating transaction options for claim acceptance: %w", err)
	}
	lastBlockNumber := new(big.Int).SetUint64(epoch.LastBlock)
	tx, err := ic.AcceptClaim(txOpts, application.IApplicationAddress,
		lastBlockNumber, *epoch.MachineHash)
	if err != nil {
		cb.logger.Warn("acceptClaimOnBlockchain:failed",
			"appContractAddress", application.IApplicationAddress,
			"machineHash", *epoch.MachineHash,
			"last_block", epoch.LastBlock,
			"error", err)
	} else {
		txHash = tx.Hash()
		cb.logger.Debug("acceptClaimOnBlockchain:success",
			"appContractAddress", application.IApplicationAddress,
			"machineHash", *epoch.MachineHash,
			"last_block", epoch.LastBlock,
			"TxHash", txHash)
	}
	return txHash, err
}

// getClaimStatus reads the on-chain Claim record for this app, last processed
// block, and machine root.
//
// The caller passes the tick's finalized block number. That makes all chain
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
	ic, err := iconsensus.NewIConsensusCaller(application.IConsensusAddress, cb.client)
	if err != nil {
		return zero, fmt.Errorf("creating IConsensus caller for getClaim: %w", err)
	}
	opts := &bind.CallOpts{Context: ctx, BlockNumber: blockNumber}
	return ic.GetClaim(opts, application.IApplicationAddress,
		new(big.Int).SetUint64(epoch.LastBlock), *epoch.MachineHash)
}

// isCustomConsensusError matches a typed Solidity error against an RPC revert.
// Checks IConsensus first; falls back to IQuorum so Quorum-only errors such
// as CallerIsNotValidator are also recognised. Selectors are name+type-based,
// so an error declared in both interfaces has the same selector either way.
func isCustomConsensusError(err error, name string) bool {
	return ethutil.IsCustomError(err, iconsensus.IConsensusMetaData, name) ||
		ethutil.IsCustomError(err, iquorum.IQuorumMetaData, name)
}

// isCustomApplicationError matches a typed Solidity error declared in the
// IApplication ABI. The on-chain merkle library errors (e.g. InvalidNodeIndex)
// are raised by consensus calls but are not declared in the IConsensus ABI;
// the selector is derived from the error signature alone, so any ABI that
// declares the error works for matching.
func isCustomApplicationError(err error, name string) bool {
	return ethutil.IsCustomError(err, iapplication.IApplicationMetaData, name)
}

// poll a transaction for its receipt
func (cb *claimerBlockchain) pollTransaction(
	ctx context.Context,
	txHash common.Hash,
	endBlockNumber *big.Int,
) (bool, *types.Receipt, error) {
	receipt, err := cb.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		// TransactionReceipt returns ethereum.NotFound while the transaction is still pending.
		// Treat this as "not ready yet" rather than a hard failure so that callers keep
		// the claim in-flight and avoid duplicate submissions.
		if errors.Is(err, ethereum.NotFound) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("fetching transaction receipt for %v: %w", txHash, err)
	}

	// receipt must be committed before use. Return false until it is.
	return receipt.BlockNumber.Cmp(endBlockNumber) <= 0, receipt, nil
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
		return nil, fmt.Errorf("fetching header for block %v: %w", nr, err)
	}
	if hdr == nil {
		return nil, fmt.Errorf("returned header for block %v is nil", nr)
	}
	return hdr.Number, nil
}

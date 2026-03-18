// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Algorithm for the state transition of computed claims. Possible actions are:
// - update epoch in the database
// - submit claim to blockchain
// - transition application to an invalid state
//
// 1. On startup of a clean blockchain there are no previous claims nor events.
//
//   - This configuration must submit a new computed claim.
//
//     2. Some time after the submission, the computed claim shows up as a claimSubmitted
//     event in the blockchain. The claim and event must match.
//
//   - This configuration must update the epoch in the database: computed -> submitted
//
// 3. After the first epoch, additional checks must be done. Same as (1) otherwise.
// 3.1. No epoch was skipped:
//   - previous_claim.last_block < current_claim.first_block
//
// 4. After the first epoch, additional checks must be done. Same as (2) otherwise.
// 4.1. epochs are in order:
//   - previous_claim.last_block < current_claim.first_block
//
// 4.2. There are no events between the epochs
//   - next(previous_event) == current_event
//
// Other cases are errors.
//
// | n |      prev     |      curr     | action |
// |   | claim | event | claim | event |        |
// |---+-------+-------+-------+-------+--------+
// | 1 |   .   |   .   |  cc   |   .   | submit |
// | 2 |   .   |   .   |  cc   |  ce   | update |
// | 3 |  pc   |  pe   |  cc   |   .   | submit |
// | 4 |  pc   |  pe   |  cc   |  ce   | update |
package claimer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"

	"github.com/cartesi/rollups-node/internal/appstatus"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	//"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

type iRepository interface {
	repository.ApplicationRepository
	repository.EpochRepository
	repository.NodeConfigRepository
}

// iBlockchain is the minimal client interface the claimer needs.
// It embeds bind.ContractBackend (required by the consensus contract binding)
// and adds TransactionReceipt for monitoring in-flight claim transactions.
// HeaderByNumber is already part of bind.ContractBackend via ContractTransactor.
type iBlockchain interface {
	bind.ContractBackend
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// iConsensus is a testable façade over the generated *iconsensus.IConsensus,
// using slice-based filter methods to avoid exposing iterator internals.
type iConsensus interface {
	GetNumberOfSubmittedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error)
	GetNumberOfAcceptedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error)
	FilterClaimSubmitted(opts *bind.FilterOpts, submitter []common.Address, appContract []common.Address) ([]*iconsensus.IConsensusClaimSubmitted, error)
	FilterClaimAccepted(opts *bind.FilterOpts, appContract []common.Address) ([]*iconsensus.IConsensusClaimAccepted, error)
	SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error)
}

// consensusAdapter wraps the generated *iconsensus.IConsensus and implements
// iConsensus by draining iterators into slices.
type consensusAdapter struct {
	ic *iconsensus.IConsensus
}

func newConsensus(addr common.Address, client iBlockchain) (iConsensus, error) {
	ic, err := iconsensus.NewIConsensus(addr, client)
	if err != nil {
		return nil, err
	}
	return &consensusAdapter{ic: ic}, nil
}

func (a *consensusAdapter) GetNumberOfSubmittedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	return a.ic.GetNumberOfSubmittedClaims(opts, appContract)
}

func (a *consensusAdapter) GetNumberOfAcceptedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	return a.ic.GetNumberOfAcceptedClaims(opts, appContract)
}

func (a *consensusAdapter) FilterClaimSubmitted(opts *bind.FilterOpts, submitter []common.Address, appContract []common.Address) ([]*iconsensus.IConsensusClaimSubmitted, error) {
	it, err := a.ic.FilterClaimSubmitted(opts, submitter, appContract)
	if err != nil {
		return nil, err
	}
	defer it.Close()
	var events []*iconsensus.IConsensusClaimSubmitted
	for it.Next() {
		events = append(events, it.Event)
	}
	return events, it.Error()
}

func (a *consensusAdapter) FilterClaimAccepted(opts *bind.FilterOpts, appContract []common.Address) ([]*iconsensus.IConsensusClaimAccepted, error) {
	it, err := a.ic.FilterClaimAccepted(opts, appContract)
	if err != nil {
		return nil, err
	}
	defer it.Close()
	var events []*iconsensus.IConsensusClaimAccepted
	for it.Next() {
		events = append(events, it.Event)
	}
	return events, it.Error()
}

func (a *consensusAdapter) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return a.ic.SubmitClaim(opts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

func update(
	ctx context.Context,
	logger *slog.Logger,
	txOpts *bind.TransactOpts,
	repo iRepository,
	client iBlockchain,
	endBlock uint64,
) []error {
	var errs []error
	apps, _, err := repo.ListApplications(
		ctx,
		repository.ApplicationFilter{
			State: Pointer(ApplicationState_Enabled),
		},
		repository.Pagination{},
		true,
	)
	if err != nil {
		return []error{err}
	}

	for _, app := range apps {
		if app.IsDaveConsensus() {
			logger.Debug("incompatible consensus", "application", app.Name)
			continue
		}

		logger.Debug("processing claims", "application", app.Name)
		if err := updateApplication(ctx, logger, txOpts, repo, client, app, endBlock); err != nil {
			errs = append(errs, err)
			continue
		}
	}
	return errs
}

func updateApplication(
	ctx context.Context,
	logger *slog.Logger,
	txOpts *bind.TransactOpts,
	ir iRepository,
	client iBlockchain,
	app *Application,
	endBlock uint64,
) error {
	epochs, _, err := ir.ListEpochs(
		ctx,
		app.Name,
		repository.EpochFilter{
			Status: []EpochStatus{
				EpochStatus_ClaimComputed,
				EpochStatus_ClaimSubmitted,
			},
		},
		repository.Pagination{},
		false,
	)
	if err != nil {
		return err
	}

	// nothing to do
	if len(epochs) == 0 {
		return nil
	}

	ic, err := newConsensus(app.IConsensusAddress, client)
	if err != nil {
		return err
	}

	if err := updateClaimSubmitted(ctx, logger, app, epochs, ir, ic, endBlock); err != nil {
		return err
	}

	if err := updateClaimAccepted(ctx, logger, app, epochs, ir, ic, endBlock); err != nil {
		return err
	}

	if epochs[0].Status == EpochStatus_ClaimComputed {
		return trySubmitClaim(ctx, logger, app, epochs[0], ir, ic, client, txOpts, endBlock)
	}

	return nil
}

// update epochs from claim accepted events.
// ClaimAccepted events are exactly 1 - 1 with epochs
func updateClaimAccepted(
	ctx context.Context,
	logger *slog.Logger,
	app *Application,
	epochs []*Epoch,
	ir iRepository,
	ic iConsensus,
	endBlock uint64,
) error {
	startBlock := max(app.LastAcceptedClaimCheckBlock, epochs[0].LastBlock) + 1
	base, events, err := collectClaimAcceptedEvents(ctx, logger, app, ic, startBlock, endBlock)
	if err != nil {
		return err
	}

	for i, ev := range events {
		// there are more accepted events than computed + submitted
		// epochs, that means we are probably catching up an
		// application with some history. We'll try again later after
		// the validator produces more computed epochs.
		if i >= len(epochs) {
			endBlock = ev.Raw.BlockNumber - 1
			break
		}

		expectedVirtualIndex := base + uint64(i)
		if epochs[i].VirtualIndex != expectedVirtualIndex {
			return appstatus.SetInoperablef(
				ctx,
				logger,
				ir,
				app,
				"claim accepted event sequence mismatch: expected virtual index %d, got %d (base=%d, offset=%d)",
				expectedVirtualIndex,
				epochs[i].VirtualIndex,
				base,
				i,
			)
		}
		if err := checkClaimAcceptedEvent(app, epochs[i], ev); err != nil {
			return appstatus.SetInoperablef(
				ctx,
				logger,
				ir,
				app,
				"claim accepted event validation failed: %v",
				err,
			)
		}

		txHash := ev.Raw.TxHash
		epochs[i].ClaimAcceptedTransactionHash = &txHash
		if err := ir.UpdateEpochClaimAcceptedTransactionHash(ctx, app.Name, epochs[i]); err != nil {
			endBlock = ev.Raw.BlockNumber - 1
			break
		}

		epochs[i].Status = EpochStatus_ClaimAccepted
		if err := ir.UpdateEpochStatus(ctx, app.Name, epochs[i]); err != nil {
			endBlock = ev.Raw.BlockNumber - 1
			break
		}
		logger.Info("Claim accepted (confirmed)",
			"application", app.Name,
			"epoch_index", epochs[i].Index,
			"outputs_merkle_root", *epochs[i].OutputsMerkleRoot,
			"tx_hash", ev.Raw.TxHash,
			"block_number", ev.Raw.BlockNumber,
		)
	}

	err = ir.UpdateEventLastCheckBlock(
		ctx,
		[]int64{app.ID},
		MonitoredEvent_ClaimAccepted,
		endBlock,
	)
	if err != nil {
		return err
	}
	return nil
}

// update epochs from claim submitted events.
// ClaimSubmitted events are exactly 1 - 1 with epochs (for authority)
func updateClaimSubmitted(
	ctx context.Context,
	logger *slog.Logger,
	app *Application,
	epochs []*Epoch,
	ir iRepository,
	ic iConsensus,
	endBlock uint64,
) error {
	startBlock := max(app.LastSubmittedClaimCheckBlock, epochs[0].LastBlock) + 1
	base, events, err := collectClaimSubmittedEvents(ctx, logger, app, ic, startBlock, endBlock)
	if err != nil {
		return err
	}
	for i, ev := range events {
		// there are more submitted events than computed epochs, that
		// means we are probably catching up an application with some
		// history. We'll try again later after the validator produces
		// more computed epochs.
		if i >= len(epochs) {
			endBlock = ev.Raw.BlockNumber - 1
			break
		}

		expectedVirtualIndex := base + uint64(i)
		if epochs[i].VirtualIndex != expectedVirtualIndex {
			return appstatus.SetInoperablef(ctx, logger, ir, app,
				"claim submitted event sequence mismatch: expected virtual index %d, got %d (base=%d, offset=%d)",
				expectedVirtualIndex,
				epochs[i].VirtualIndex,
				base,
				i,
			)
		}
		if err := checkClaimSubmittedEvent(app, epochs[i], ev); err != nil {
			return appstatus.SetInoperablef(ctx, logger, ir, app,
				"claim submitted event validation failed: %v",
				err,
			)
		}

		txHash := ev.Raw.TxHash
		epochs[i].ClaimSubmittedTransactionHash = &txHash
		if err := ir.UpdateEpochClaimSubmittedTransactionHash(ctx, app.Name, epochs[i]); err != nil {
			endBlock = ev.Raw.BlockNumber - 1
			break
		}

		// epochs may have advanced its state during this tick.
		// make sure it makes sense to update this one
		if epochs[i].Status == EpochStatus_ClaimComputed {
			epochs[i].Status = EpochStatus_ClaimSubmitted
			err = ir.UpdateEpochStatus(ctx, app.Name, epochs[i])
			if err != nil {
				endBlock = ev.Raw.BlockNumber - 1
				break
			}
		}
	}

	err = ir.UpdateEventLastCheckBlock(
		ctx,
		[]int64{app.ID},
		MonitoredEvent_ClaimSubmitted,
		endBlock,
	)
	if err != nil {
		return err
	}
	return nil
}

func collectClaimSubmittedEvents(
	ctx context.Context,
	logger *slog.Logger,
	app *Application,
	ic iConsensus,
	startBlock uint64,
	endBlock uint64,
) (uint64, []*iconsensus.IConsensusClaimSubmitted, error) {
	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		return ic.GetNumberOfSubmittedClaims(&bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		}, app.IApplicationAddress)
	}
	prevValue, err := oracle(ctx, startBlock-1)
	if err != nil {
		return 0, nil, err
	}

	var events []*iconsensus.IConsensusClaimSubmitted
	_, err = ethutil.FindTransitions(ctx, startBlock,
		endBlock, prevValue, oracle, func(block uint64) error {
			evs, err := ic.FilterClaimSubmitted(&bind.FilterOpts{
				Context: ctx,
				Start:   block,
				End:     &block,
			}, nil, []common.Address{app.IApplicationAddress})
			if err != nil {
				return err
			}
			events = append(events, evs...)
			return nil
		},
	)
	if err != nil {
		return 0, nil, err
	}
	return prevValue.Uint64(), events, nil
}

func collectClaimAcceptedEvents(
	ctx context.Context,
	logger *slog.Logger,
	app *Application,
	ic iConsensus,
	startBlock uint64,
	endBlock uint64,
) (uint64, []*iconsensus.IConsensusClaimAccepted, error) {
	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		return ic.GetNumberOfAcceptedClaims(&bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		}, app.IApplicationAddress)
	}
	prevValue, err := oracle(ctx, startBlock-1)
	if err != nil {
		return 0, nil, err
	}

	var events []*iconsensus.IConsensusClaimAccepted
	_, err = ethutil.FindTransitions(ctx, startBlock,
		endBlock, prevValue, oracle, func(block uint64) error {
			evs, err := ic.FilterClaimAccepted(&bind.FilterOpts{
				Context: ctx,
				Start:   block,
				End:     &block,
			}, []common.Address{app.IApplicationAddress})
			if err != nil {
				return err
			}
			events = append(events, evs...)
			return nil
		},
	)
	if err != nil {
		return 0, nil, err
	}
	return prevValue.Uint64(), events, nil
}

func trySubmitClaim(
	ctx context.Context,
	logger *slog.Logger,
	app *Application,
	epoch *Epoch,
	ir iRepository,
	ic iConsensus,
	client iBlockchain,
	txOpts *bind.TransactOpts,
	endBlock uint64,
) error {
	// claim submitted but not confirmed, try to confirm it
	if epoch.ClaimSubmittedTransactionHash != nil {
		receipt, err := client.TransactionReceipt(ctx, *epoch.ClaimSubmittedTransactionHash)
		if errors.Is(err, ethereum.NotFound) {
			goto submit // yes: something went wrong with the one we had, resubmit
		}
		if err != nil {
			return err
		}

		if receipt.BlockNumber.Cmp(new(big.Int).SetUint64(endBlock)) > 0 {
			return nil // no: its too early, wait for receipt to be committed
		}

		if receipt.Status == 1 {
			epoch.Status = EpochStatus_ClaimSubmitted
			logger.Info("Claim submitted (confirmed)",
				"application", app.Name,
				"epoch_index", epoch.Index,
				"outputs_merkle_root", *epoch.OutputsMerkleRoot,
				"tx_hash", receipt.TxHash,
				"block_number", receipt.BlockNumber,
			)
			return ir.UpdateEpochStatus(ctx, app.Name, epoch)
		} else {
			// transaction reverted
			// what do we do?
		}
	}

submit:
	if txOpts == nil {
		logger.Debug("Claim NOT submitted (disabled)",
			"application", app.Name,
			"epoch_index", epoch.Index,
			"outputs_merkle_root", *epoch.OutputsMerkleRoot,
		)
		return nil
	}
	tx, err := ic.SubmitClaim(
		txOpts,
		app.IApplicationAddress,
		new(big.Int).SetUint64(epoch.LastBlock),
		*epoch.OutputsMerkleRoot,
	)
	if err != nil {
		return err
	}

	txHash := tx.Hash()
	logger.Info("Claim submitted (unconfirmed)",
		"application", app.Name,
		"epoch_index", epoch.Index,
		"outputs_merkle_root", *epoch.OutputsMerkleRoot,
		"tx_hash", txHash,
	)

	epoch.ClaimSubmittedTransactionHash = &txHash
	return ir.UpdateEpochClaimSubmittedTransactionHash(ctx, app.Name, epoch)
}

/* Retrieve the block number for the configured commitment level in ethereum terms,
 * that is: `latest`, `safe`, `finalized`, etc. Which may be many blocks behind. */
func getDefaultBlockNumber(
	ctx context.Context,
	client iBlockchain,
	defaultBlock DefaultBlock,
) (uint64, error) {
	var nr int64
	switch defaultBlock {
	case DefaultBlock_Pending:
		nr = rpc.PendingBlockNumber.Int64()
	case DefaultBlock_Latest:
		nr = rpc.LatestBlockNumber.Int64()
	case DefaultBlock_Finalized:
		nr = rpc.FinalizedBlockNumber.Int64()
	case DefaultBlock_Safe:
		nr = rpc.SafeBlockNumber.Int64()
	default:
		return 0, fmt.Errorf("default block '%v' not supported", defaultBlock)
	}

	hdr, err := client.HeaderByNumber(ctx, big.NewInt(nr))
	if err != nil {
		return 0, err
	}
	return hdr.Number.Uint64(), nil
}

func checkClaimAcceptedEvent(application *Application, epoch *Epoch, event *iconsensus.IConsensusClaimAccepted) error {
	if application == nil {
		return fmt.Errorf("missing the application (nil)")
	}
	if epoch == nil {
		return fmt.Errorf("missing the epoch (nil)")
	}
	if event == nil {
		return fmt.Errorf("missing the event (nil)")
	}

	if application.IApplicationAddress != event.AppContract {
		return fmt.Errorf("application contract mismatch: %v != %v",
			application.IApplicationAddress, event.AppContract)
	}
	if epoch.OutputsMerkleRoot == nil {
		return fmt.Errorf("epoch is missing outputs merkle root (nil)")
	}
	if *epoch.OutputsMerkleRoot != event.OutputsMerkleRoot {
		return fmt.Errorf("outputs merkle root mismatch: %v != %v",
			*epoch.OutputsMerkleRoot, common.Hash(event.OutputsMerkleRoot))
	}
	if nr := event.LastProcessedBlockNumber.Uint64(); epoch.LastBlock != nr {
		return fmt.Errorf("outputs merkle root mismatch: %v != %v",
			epoch.LastBlock, nr)
	}
	return nil
}

func checkClaimSubmittedEvent(application *Application, epoch *Epoch, event *iconsensus.IConsensusClaimSubmitted) error {
	if application == nil {
		return fmt.Errorf("missing the application (nil)")
	}
	if epoch == nil {
		return fmt.Errorf("missing the epoch (nil)")
	}
	if event == nil {
		return fmt.Errorf("missing the event (nil)")
	}

	if application.IApplicationAddress != event.AppContract {
		return fmt.Errorf("application contract mismatch: %v != %v",
			application.IApplicationAddress, event.AppContract)
	}
	if epoch.OutputsMerkleRoot == nil {
		return fmt.Errorf("epoch is missing outputs merkle root (nil)")
	}
	if *epoch.OutputsMerkleRoot != event.OutputsMerkleRoot {
		return fmt.Errorf("outputs merkle root mismatch: %v != %v",
			*epoch.OutputsMerkleRoot, common.Hash(event.OutputsMerkleRoot))
	}
	if nr := event.LastProcessedBlockNumber.Uint64(); epoch.LastBlock != nr {
		return fmt.Errorf("outputs merkle root mismatch: %v != %v",
			epoch.LastBlock, nr)
	}
	return nil
}

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
	"fmt"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	ErrClaimMismatch = fmt.Errorf("constraints failed for epoch claim and its successor.")
	ErrEventMismatch = fmt.Errorf("epoch claim does not match its corresponding event.")
	ErrMissingEvent  = fmt.Errorf("epoch claim does not have a corresponding event.")
)

type iclaimerRepository interface {
	// key is model.Application.ID
	SelectSubmittedClaimPairsPerApp(ctx context.Context) (
		map[int64]*model.Epoch,
		map[int64]*model.Epoch,
		map[int64]*model.Application,
		error,
	)

	// key is model.Application.ID
	SelectAcceptedClaimPairsPerApp(ctx context.Context) (
		map[int64]*model.Epoch,
		map[int64]*model.Epoch,
		map[int64]*model.Application,
		error,
	)

	UpdateEpochWithSubmittedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
		transactionHash common.Hash,
	) error

	UpdateEpochWithAcceptedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
	) error

	UpdateApplicationState(
		ctx context.Context,
		appID int64,
		state model.ApplicationState,
		reason *string,
	) error

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}

func hashToHex(h *common.Hash) string {
	if h == nil {
		return ""
	}
	return h.Hex()
}

// claims in flight are those that have been submitted but are waiting for a
// transaction confirmation. When confirmed, we update their status on the
// database. The epoch is now "submitted" and no longer "computed".
// Returns the number of confirmed transitions and any error.
func (s *Service) checkClaimsInFlight(
	computedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	endBlock *big.Int,
) (int, error) {
	confirmed := 0
	// check claims in flight. NOTE: map mutation + iteration is safe in Go
	for key, txHash := range s.claimsInFlight {
		ready, receipt, err := s.blockchain.pollTransaction(s.Context, txHash, endBlock)
		if err != nil {
			s.Logger.Warn("Claim submission failed, retrying.",
				"txHash", txHash,
				"err", err,
			)
			delete(s.claimsInFlight, key)
			continue
		}
		if !ready {
			continue
		}
		if receipt.Status == 0 {
			s.Logger.Warn("Claim submission reverted, retrying.",
				"txHash", txHash,
				"err", err,
			)
			delete(s.claimsInFlight, key)
			continue
		}
		if computedEpoch, ok := computedEpochs[key]; ok {
			err = s.repository.UpdateEpochWithSubmittedClaim(
				s.Context,
				computedEpoch.ApplicationID,
				computedEpoch.Index,
				receipt.TxHash,
			)

			// NOTE: there is no point in trying the other applications on a database error
			//       so we just return and try again later (next tick)
			if err != nil {
				return confirmed, fmt.Errorf("updating epoch %d (%d) with submitted claim: %w", computedEpoch.Index, computedEpoch.VirtualIndex, err)
			}
			confirmed++

			app := apps[key]
			appAddress := common.Address{}
			if app != nil {
				appAddress = app.IApplicationAddress
			}
			s.Logger.Info("Claim submitted",
				"app", appAddress,
				"receipt_block_number", receipt.BlockNumber,
				"claim_hash", hashToHex(computedEpoch.OutputsMerkleRoot),
				"last_block", computedEpoch.LastBlock,
				"tx", txHash)

			// Authority emits ClaimAccepted in the same tx as ClaimSubmitted.
			// Parse the receipt to transition directly to accepted, saving a
			// full tick round-trip. Quorum waits for a separate acceptance scan.
			if app != nil && app.ConsensusType == model.Consensus_Authority {
				if accepted := s.tryAcceptFromReceipt(receipt, app, computedEpoch); accepted {
					confirmed++
				}
			}

			// epoch is no longer "computed" and is now "submitted" (or accepted).
			delete(computedEpochs, key)
		} else {
			s.Logger.Warn("unexpected, claim in flight is not a computed epoch.",
				"id", key,
				"tx", receipt.TxHash)
		}
		delete(s.claimsInFlight, key)
	}
	return confirmed, nil
}

// tryAcceptFromReceipt parses a transaction receipt for a ClaimAccepted event
// matching the given epoch. If found and valid, it transitions the epoch
// directly to accepted in the database, returning true. This is an optimization
// for Authority consensus, which emits both ClaimSubmitted and ClaimAccepted
// atomically in the same transaction.
//
// Errors are logged but not propagated — the normal acceptance scan on the
// next tick will handle the transition if this fast path fails.
func (s *Service) tryAcceptFromReceipt(
	receipt *types.Receipt,
	app *model.Application,
	epoch *model.Epoch,
) bool {
	ic, err := iconsensus.NewIConsensus(app.IConsensusAddress, nil)
	if err != nil {
		s.Logger.Warn("Authority fast-accept: failed to create ABI binding",
			"app", app.IApplicationAddress, "error", err)
		return false
	}
	for _, log := range receipt.Logs {
		event, err := ic.ParseClaimAccepted(*log)
		if err != nil {
			continue // not a ClaimAccepted event
		}
		if !claimAcceptedEventMatches(app, epoch, event) {
			continue
		}
		err = s.repository.UpdateEpochWithAcceptedClaim(
			s.Context, epoch.ApplicationID, epoch.Index)
		if err != nil {
			s.Logger.Warn("Authority fast-accept: DB update failed, "+
				"will retry via normal acceptance scan",
				"app", app.IApplicationAddress,
				"epoch", epoch.Index, "error", err)
			return false
		}
		s.Logger.Info("Claim accepted (Authority fast path)",
			"app", app.IApplicationAddress,
			"epoch_index", epoch.Index,
			"claim_hash", hashToHex(epoch.OutputsMerkleRoot),
			"last_block", epoch.LastBlock,
			"tx", receipt.TxHash)
		return true
	}
	// No matching ClaimAccepted event found. This is unexpected for Authority
	// but not fatal — the normal acceptance scan will handle it.
	s.Logger.Warn("Authority fast-accept: ClaimAccepted event not found in receipt",
		"app", app.IApplicationAddress, "tx", receipt.TxHash)
	return false
}

func (s *Service) findClaimSubmittedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	prevEpoch *model.Epoch,
	currEpoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimSubmitted,
	*iconsensus.IConsensusClaimSubmitted,
	error,
) {
	err := checkEpochSequenceConstraint(prevEpoch, currEpoch)
	if err != nil {
		err = s.setApplicationInoperable(
			s.Context,
			app,
			"%v. epoch: %v (%v).",
			err,
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
		)
		return nil, nil, nil, err
	}

	ic, prevClaimSubmissionEvent, currClaimSubmissionEvent, err :=
		s.blockchain.findClaimSubmittedEventAndSucc(ctx, app, prevEpoch, fromBlock, toBlock)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("finding claim submitted event for epoch %d (%d): %w", prevEpoch.Index, prevEpoch.VirtualIndex, err)
	}

	if prevClaimSubmissionEvent == nil {
		err = s.setApplicationInoperable(
			s.Context,
			app,
			"application has an invalid epoch: %v (%v). No claim submission event to match.",
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
		)
		return nil, nil, nil, err
	}

	if !claimSubmittedEventMatches(app, prevEpoch, prevClaimSubmissionEvent) {
		err = s.setApplicationInoperable(
			s.Context,
			app,
			"application has an invalid epoch: %v (%v), missing claim submitted event (%v).",
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
			prevClaimSubmissionEvent.Raw.TxHash,
		)
		return nil, nil, nil, err
	}
	return ic, prevClaimSubmissionEvent, currClaimSubmissionEvent, nil
}

// transition epoch claims from computed to submitted.
// Returns the number of successful transitions and any errors.
func (s *Service) submitClaimsAndUpdateDatabase(
	acceptedOrSubmittedEpochs map[int64]*model.Epoch,
	computedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, []error) {
	confirmed, err := s.checkClaimsInFlight(computedEpochs, apps, defaultBlockNumber)
	if err != nil {
		return confirmed, []error{err}
	}

	transitions := confirmed
	errs := []error{}
	// check computed epochs. NOTE: map mutation + iteration is safe in Go
	for key, currEpoch := range computedEpochs {
		var ic *iconsensus.IConsensus
		var currEvent *iconsensus.IConsensusClaimSubmitted

		if _, isClaimInFlight := s.claimsInFlight[key]; isClaimInFlight {
			continue
		}

		app := apps[key] // guaranteed to exist because of the query and database constraints
		prevEpoch, prevEpochExists := acceptedOrSubmittedEpochs[key]

		// check address for changes
		if err := s.checkConsensusForAddressChange(app); err != nil {
			delete(computedEpochs, key)
			errs = append(errs, err)
			continue
		}
		if prevEpochExists {
			ic, _, currEvent, err = s.findClaimSubmittedEventAndSucc(
				s.Context, app, prevEpoch, currEpoch, prevEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
			)
		} else {
			ic, currEvent, _, err = s.blockchain.findClaimSubmittedEventAndSucc(
				s.Context, app, currEpoch, currEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
			)
		}
		if err != nil {
			delete(computedEpochs, key)
			errs = append(errs, err)
			continue
		}

		if currEvent != nil {
			s.Logger.Debug("Found ClaimSubmitted Event",
				"app", currEvent.AppContract,
				"claim_hash", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
				"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
			)
			if !claimSubmittedEventMatches(app, currEpoch, currEvent) {
				err = s.setApplicationInoperable(
					s.Context,
					app,
					"computed claim does not match event. computed_claim=%v, current_event=%v",
					currEpoch, currEvent,
				)
				delete(computedEpochs, key)
				errs = append(errs, err)
				continue
			}
			s.Logger.Debug("Updating claim status to submitted",
				"app", app.IApplicationAddress,
				"claim_hash", hashToHex(currEpoch.OutputsMerkleRoot),
				"last_block", currEpoch.LastBlock,
			)
			txHash := currEvent.Raw.TxHash
			err = s.repository.UpdateEpochWithSubmittedClaim(
				s.Context,
				currEpoch.ApplicationID,
				currEpoch.Index,
				txHash,
			)
			if err != nil {
				delete(computedEpochs, key)
				errs = append(errs, err)
				continue
			}
			delete(s.claimsInFlight, key)
			transitions++
			s.Logger.Info("Claim previously submitted",
				"app", app.IApplicationAddress,
				"event_block_number", currEvent.Raw.BlockNumber,
				"claim_hash", hashToHex(currEpoch.OutputsMerkleRoot),
				"last_block", currEpoch.LastBlock,
			)
		} else {
			if s.submissionEnabled {
				if prevEpoch != nil && prevEpoch.Status != model.EpochStatus_ClaimAccepted {
					s.Logger.Debug("Waiting previous claim to be accepted before submitting new one. Previous:",
						"app", app.IApplicationAddress,
						"claim_hash", hashToHex(prevEpoch.OutputsMerkleRoot),
						"last_block", prevEpoch.LastBlock,
					)
					continue
				}
				s.Logger.Debug("Submitting claim to blockchain",
					"app", app.IApplicationAddress,
					"claim_hash", hashToHex(currEpoch.OutputsMerkleRoot),
					"last_block", currEpoch.LastBlock,
				)
				txHash, err := s.blockchain.submitClaimToBlockchain(ic, app, currEpoch)
				if err != nil {
					// NotFirstClaim handling after restart.
					//
					// Gas estimation (eth_estimateGas) simulates
					// the call before broadcasting, so the revert
					// is caught without spending gas. This relies
					// on txOpts.GasLimit == 0 (the default); if
					// GasLimit were pre-set, the tx would skip
					// estimation and revert on-chain.
					//
					// Authority: submitClaim checks a per-epoch
					// bitmap. Any duplicate (same epoch, regardless
					// of merkle root) reverts with NotFirstClaim.
					// After restart this is benign — the node
					// recomputed the same claim that was already
					// on-chain. Both ClaimSubmitted and
					// ClaimAccepted events were already emitted
					// (Authority emits both atomically).
					//
					// Quorum: submitClaim first checks if this
					// validator already voted for the SAME claim
					// (same app + lastBlock + merkleRoot). If so,
					// it silently returns — no revert, no event.
					// It only reverts with NotFirstClaim when the
					// validator voted for a DIFFERENT merkleRoot
					// in the same epoch (checked via allVotes
					// bitmap). After restart, this means the node
					// recomputed a different claim hash than what
					// it submitted pre-restart — a determinism
					// violation. ClaimSubmitted was emitted for
					// the original vote; ClaimAccepted is emitted
					// only once a majority of validators agree.
					if isNotFirstClaimError(err) {
						if app.ConsensusType == model.Consensus_Quorum {
							// Quorum only reverts with NotFirstClaim
							// when the merkle root differs. This is
							// unrecoverable: computation is expected
							// to be deterministic, so recomputing
							// will produce the same divergent hash.
							err = s.setApplicationInoperable(
								s.Context,
								app,
								"NotFirstClaim from Quorum consensus: "+
									"computed claim hash %s differs from "+
									"previously submitted claim for "+
									"epoch with last_block %d. "+
									"Possible determinism violation or "+
									"machine state corruption.",
								hashToHex(currEpoch.OutputsMerkleRoot),
								currEpoch.LastBlock,
							)
							delete(computedEpochs, key)
							errs = append(errs, err)
							continue
						}
						s.Logger.Info(
							"Claim already on-chain, "+
								"waiting for event sync",
							"app", app.IApplicationAddress,
							"claim_hash",
							hashToHex(currEpoch.OutputsMerkleRoot),
							"last_block", currEpoch.LastBlock,
						)
						continue
					}
					delete(computedEpochs, key)
					errs = append(errs, err)
					continue
				}
				s.claimsInFlight[key] = txHash
				transitions++
			}
		}
	}
	return transitions, errs
}

func (s *Service) findClaimAcceptedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	prevEpoch *model.Epoch,
	currEpoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimAccepted,
	*iconsensus.IConsensusClaimAccepted,
	error,
) {
	err := checkEpochSequenceConstraint(prevEpoch, currEpoch)
	if err != nil {
		err = s.setApplicationInoperable(
			ctx,
			app,
			"%v. epoch: %v (%v).",
			err,
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
		)
		return nil, nil, nil, err
	}

	ic, prevClaimAcceptanceEvent, currClaimAcceptanceEvent, err :=
		s.blockchain.findClaimAcceptedEventAndSucc(ctx, app, prevEpoch, fromBlock, toBlock)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("finding claim accepted event for epoch %d (%d): %w", prevEpoch.Index, prevEpoch.VirtualIndex, err)
	}

	if prevClaimAcceptanceEvent == nil {
		err = s.setApplicationInoperable(
			ctx,
			app,
			"application has an invalid epoch: %v (%v), missing claim acceptance event.",
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
		)
		return nil, nil, nil, err
	}
	if !claimAcceptedEventMatches(app, prevEpoch, prevClaimAcceptanceEvent) {
		err = s.setApplicationInoperable(
			ctx,
			app,
			"application has an invalid epoch: %v (%v). event does not match: %v",
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
			prevClaimAcceptanceEvent.Raw.TxHash,
		)
		return nil, nil, nil, err
	}
	return ic, prevClaimAcceptanceEvent, currClaimAcceptanceEvent, nil
}

// transition claims from submitted to accepted.
// Returns the number of successful transitions and any errors.
func (s *Service) acceptClaimsAndUpdateDatabase(
	acceptedEpochs map[int64]*model.Epoch,
	submittedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, []error) {
	transitions := 0
	errs := []error{}
	var err error

	// check submitted  epochs. NOTE: map mutation + iteration is safe in Go
	for key, currEpoch := range submittedEpochs {
		var currEvent *iconsensus.IConsensusClaimAccepted

		app := apps[key]
		prevEpoch, prevEpochExists := acceptedEpochs[key]
		// check address for changes
		if err := s.checkConsensusForAddressChange(app); err != nil {
			delete(submittedEpochs, key)
			errs = append(errs, err)
			continue
		}

		if prevEpochExists {
			_, _, currEvent, err = s.findClaimAcceptedEventAndSucc(
				s.Context, app, prevEpoch, currEpoch, prevEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
			)
		} else {
			_, currEvent, _, err = s.blockchain.findClaimAcceptedEventAndSucc(
				s.Context, app, currEpoch, currEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
			)
		}
		if err != nil {
			delete(submittedEpochs, key)
			errs = append(errs, err)
			continue
		}

		if currEvent != nil {
			s.Logger.Debug("Found ClaimAccepted Event",
				"app", currEvent.AppContract,
				"claim_hash", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
				"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
			)
			if !claimAcceptedEventMatches(app, currEpoch, currEvent) {
				s.Logger.Error("event mismatch",
					"claim", currEpoch,
					"event", currEvent,
					"err", ErrEventMismatch,
				)
				err := s.setApplicationInoperable(
					s.Context,
					app,
					"event mismatch for epoch %v, event tx_hash: %v",
					currEpoch.Index,
					currEvent.Raw.TxHash,
				)
				delete(submittedEpochs, key)
				errs = append(errs, err)
				continue
			}
			s.Logger.Debug("Updating claim status to accepted",
				"app", app.IApplicationAddress,
				"claim_hash", hashToHex(currEpoch.OutputsMerkleRoot),
				"last_block", currEpoch.LastBlock,
			)
			txHash := currEvent.Raw.TxHash
			err = s.repository.UpdateEpochWithAcceptedClaim(s.Context, currEpoch.ApplicationID, currEpoch.Index)
			if err != nil {
				delete(submittedEpochs, key)
				errs = append(errs, err)
				continue
			}
			transitions++
			s.Logger.Info("Claim accepted",
				"app", currEvent.AppContract,
				"event_block_number", currEvent.Raw.BlockNumber,
				"claim_hash", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
				"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
				"tx", txHash,
			)
		}
	}
	return transitions, errs
}

func (s *Service) setApplicationInoperable(
	ctx context.Context,
	app *model.Application,
	reasonFmt string,
	args ...any,
) error {
	return appstatus.SetInoperablef(ctx, s.Logger, s.repository, app, reasonFmt, args...)
}

func (s *Service) checkConsensusForAddressChange(
	app *model.Application,
) error {
	newConsensusAddress, err := s.blockchain.getConsensusAddress(s.Context, app)
	if err != nil {
		return fmt.Errorf("getting consensus address for app %v: %w", app.IApplicationAddress, err)
	}
	if app.IConsensusAddress != newConsensusAddress {
		err = s.setApplicationInoperable(
			s.Context,
			app,
			"consensus change detected. application: %v.",
			app.IApplicationAddress,
		)
		return err
	}
	return nil
}

func checkEpochConstraint(epoch *model.Epoch) error {
	if epoch.FirstBlock > epoch.LastBlock {
		return fmt.Errorf("unexpected epoch state. first_block: %v > last_block: %v",
			epoch.FirstBlock, epoch.LastBlock)
	}

	mustHaveOutputsMerkleRoot := epoch.Status == model.EpochStatus_ClaimSubmitted ||
		epoch.Status == model.EpochStatus_ClaimAccepted ||
		epoch.Status == model.EpochStatus_ClaimComputed
	if mustHaveOutputsMerkleRoot {
		if epoch.OutputsMerkleRoot == nil {
			return fmt.Errorf("unexpected epoch state. missing outputs_merkle_root.")
		}
	}

	mustHaveClaimTransactionHash := epoch.Status == model.EpochStatus_ClaimSubmitted ||
		epoch.Status == model.EpochStatus_ClaimAccepted
	if mustHaveClaimTransactionHash {
		if epoch.ClaimTransactionHash == nil {
			return fmt.Errorf("unexpected epoch state. missing claim_transaction_hash.")
		}
	}
	return nil
}

func checkEpochSequenceConstraint(prevEpoch *model.Epoch, currEpoch *model.Epoch) error {
	var err error

	err = checkEpochConstraint(currEpoch)
	if err != nil {
		return fmt.Errorf("%w on current epoch.", err)
	}
	err = checkEpochConstraint(prevEpoch)
	if err != nil {
		return fmt.Errorf("%w on previous epoch.", err)
	}

	if prevEpoch.LastBlock > currEpoch.LastBlock {
		return fmt.Errorf("unexpected epochs sequence on field last_block: previous(%v) > current(%v)", prevEpoch.LastBlock, currEpoch.LastBlock)
	}
	if prevEpoch.FirstBlock > currEpoch.FirstBlock {
		return fmt.Errorf("unexpected epochs sequence on field first_block: previous(%v) > current(%v)", prevEpoch.FirstBlock, currEpoch.FirstBlock)
	}
	if prevEpoch.Index > currEpoch.Index {
		return fmt.Errorf("unexpected epochs sequence on field index: previous(%v) > current(%v)", prevEpoch.Index, currEpoch.Index)
	}
	return nil
}

func claimSubmittedEventMatches(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimSubmitted) bool {
	if application == nil || epoch == nil || event == nil {
		return false
	}
	return application.IApplicationAddress == event.AppContract &&
		epoch.OutputsMerkleRoot != nil &&
		*epoch.OutputsMerkleRoot == event.OutputsMerkleRoot &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64()
}

func claimAcceptedEventMatches(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimAccepted) bool {
	if application == nil || epoch == nil || event == nil {
		return false
	}
	return application.IApplicationAddress == event.AppContract &&
		epoch.OutputsMerkleRoot != nil &&
		*epoch.OutputsMerkleRoot == event.OutputsMerkleRoot &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64()
}

// claimAcceptedEventMatchesEpoch checks if a ClaimAccepted event belongs to
// the same epoch (app + lastBlock) regardless of the merkle root. This is
// used to detect outvoting in Quorum: a ClaimAccepted event exists for the
// epoch but with a different merkle root than what this node submitted.
func claimAcceptedEventMatchesEpoch(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimAccepted) bool {
	if application == nil || epoch == nil || event == nil {
		return false
	}
	return application.IApplicationAddress == event.AppContract &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64()
}

func (s *Service) String() string {
	return s.Name
}

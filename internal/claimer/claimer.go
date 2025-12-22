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
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrClaimMismatch = fmt.Errorf("Claim and antecessor mismatch")
	ErrEventMismatch = fmt.Errorf("Computed Claim mismatches ClaimSubmitted event")
	ErrMissingEvent  = fmt.Errorf("Accepted claim has no matching blockchain event")
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
		application_id int64,
		index uint64,
		transaction_hash common.Hash,
	) error

	UpdateEpochWithAcceptedClaim(
		ctx context.Context,
		application_id int64,
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

/* transition claims from computed to submitted */
func (s *Service) submitClaimsAndUpdateDatabase(
	acceptedOrSubmittedEpochs map[int64]*model.Epoch,
	computedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	endBlock *big.Int,
) []error {
	errs := []error{}
	var err error

	// check claims in flight
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
		if computedEpoch, ok := computedEpochs[key]; ok {
			err = s.repository.UpdateEpochWithSubmittedClaim(
				s.Context,
				computedEpoch.ApplicationID,
				computedEpoch.Index,
				receipt.TxHash,
			)
			if err != nil {
				errs = append(errs, err)
				return errs
			}
			s.Logger.Info("Claim submitted",
				"app", apps[key].IApplicationAddress,
				"receipt_block_number", receipt.BlockNumber,
				"claim_hash", fmt.Sprintf("%x", computedEpoch.OutputsMerkleRoot),
				"last_block", computedEpoch.LastBlock,
				"tx", txHash)
			delete(computedEpochs, key)
		} else {
			s.Logger.Warn("expected claim in flight to be in currClaims.",
				"tx", receipt.TxHash)
		}
		delete(s.claimsInFlight, key)
	}

	// check computed epochs
	for key, currEpoch := range computedEpochs {
		var ic *iconsensus.IConsensus
		var prevClaimSubmissionEvent *iconsensus.IConsensusClaimSubmitted
		var currClaimSubmissionEvent *iconsensus.IConsensusClaimSubmitted

		if _, isClaimInFlight := s.claimsInFlight[key]; isClaimInFlight {
			continue
		}

		app := apps[key] // guaranteed to exist because of the query and database constraints
		prevEpoch, previousEpochExists := acceptedOrSubmittedEpochs[key]

		// check address for changes
		if err := s.checkConsensusForAddressChange(app); err != nil {
			delete(computedEpochs, key)
			errs = append(errs, err)
			goto nextApp
		}
		if previousEpochExists {
			err := checkEpochSequenceConstraint(prevEpoch, currEpoch)
			if err != nil {
				err = s.setApplicationInoperable(
					s.Context,
					app.IApplicationAddress,
					prevEpoch.ApplicationID,
					"database mismatch on epochs. application: %v, epochs: %v (%v), %v (%v).",
					app.IApplicationAddress,
					prevEpoch.Index,
					prevEpoch.VirtualIndex,
					currEpoch.Index,
					currEpoch.VirtualIndex,
				)
				delete(computedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}

			// the previous epoch must have a matching claim submission event.
			// current epoch may or may not be present
			ic, prevClaimSubmissionEvent, currClaimSubmissionEvent, err =
				s.blockchain.findClaimSubmittedEventAndSucc(s.Context, app, prevEpoch, endBlock)
			if err != nil {
				delete(computedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
			if prevClaimSubmissionEvent == nil {
				err = s.setApplicationInoperable(
					s.Context,
					app.IApplicationAddress,
					app.ID,
					"epoch has no matching event. application: %v, epoch: %v (%v).",
					app.IApplicationAddress,
					prevEpoch.Index,
					prevEpoch.VirtualIndex,
				)
				delete(computedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
			if !claimSubmittedEventMatches(app, prevEpoch, prevClaimSubmissionEvent) {
				s.Logger.Error("event mismatch",
					"claim", prevEpoch,
					"event", prevClaimSubmissionEvent,
					"err", ErrEventMismatch,
				)
				err = s.setApplicationInoperable(
					s.Context,
					app.IApplicationAddress,
					app.ID,
					"epoch has an invalid event: %v, epoch: %v (%v). event: %v",
					currEpoch.Index,
					prevEpoch.Index,
					prevEpoch.VirtualIndex,
					prevClaimSubmissionEvent,
				)
				delete(computedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
		} else {
			// first claim
			ic, currClaimSubmissionEvent, _, err =
				s.blockchain.findClaimSubmittedEventAndSucc(s.Context, app, currEpoch, endBlock)
			if err != nil {
				delete(computedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
		}

		if currClaimSubmissionEvent != nil {
			s.Logger.Debug("Found ClaimSubmitted Event",
				"app", currClaimSubmissionEvent.AppContract,
				"claim_hash", fmt.Sprintf("%x", currClaimSubmissionEvent.OutputsMerkleRoot),
				"last_block", currClaimSubmissionEvent.LastProcessedBlockNumber.Uint64(),
			)
			if !claimSubmittedEventMatches(app, currEpoch, currClaimSubmissionEvent) {
				err = s.setApplicationInoperable(
					s.Context,
					app.IApplicationAddress,
					app.ID,
					"computed claim does not match event. computed_claim=%v, current_event=%v",
					currEpoch, currClaimSubmissionEvent,
				)
				delete(computedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
			s.Logger.Debug("Updating claim status to submitted",
				"app", app.IApplicationAddress,
				"claim_hash", fmt.Sprintf("%x", currEpoch.OutputsMerkleRoot),
				"last_block", currEpoch.LastBlock,
			)
			txHash := currClaimSubmissionEvent.Raw.TxHash
			err = s.repository.UpdateEpochWithSubmittedClaim(
				s.Context,
				currEpoch.ApplicationID,
				currEpoch.Index,
				txHash,
			)
			if err != nil {
				delete(computedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
			delete(s.claimsInFlight, key)
			s.Logger.Info("Claim previously submitted",
				"app", app.IApplicationAddress,
				"event_block_number", currClaimSubmissionEvent.Raw.BlockNumber,
				"claim_hash", fmt.Sprintf("%x", currEpoch.OutputsMerkleRoot),
				"last_block", currEpoch.LastBlock,
			)
		} else if s.submissionEnabled {
			if prevEpoch != nil && prevEpoch.Status != model.EpochStatus_ClaimAccepted {
				s.Logger.Debug("Waiting previous claim to be accepted before submitting new one. Previous:",
					"app", app.IApplicationAddress,
					"claim_hash", fmt.Sprintf("%x", prevEpoch.OutputsMerkleRoot),
					"last_block", prevEpoch.LastBlock,
				)
				goto nextApp
			}
			s.Logger.Debug("Submitting claim to blockchain",
				"app", app.IApplicationAddress,
				"claim_hash", fmt.Sprintf("%x", currEpoch.OutputsMerkleRoot),
				"last_block", currEpoch.LastBlock,
			)
			txHash, err := s.blockchain.submitClaimToBlockchain(ic, app, currEpoch)
			if err != nil {
				delete(computedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
			s.claimsInFlight[key] = txHash
		} else {
			s.Logger.Debug("Claim submission disabled. Doing nothing",
				"app", app.IApplicationAddress,
				"claim_hash", fmt.Sprintf("%x", currEpoch.OutputsMerkleRoot),
				"last_block", currEpoch.LastBlock,
			)

		}
	nextApp:
	}
	return errs
}

/* transition claims from submitted to accepted */
func (s *Service) acceptClaimsAndUpdateDatabase(
	acceptedEpochs map[int64]*model.Epoch,
	submittedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	endBlock *big.Int,
) []error {
	errs := []error{}
	var err error

	// check submitted claims
	for key, submittedEpoch := range submittedEpochs {
		var prevEvent *iconsensus.IConsensusClaimAccepted
		var currEvent *iconsensus.IConsensusClaimAccepted

		app := apps[key]
		acceptedEpoch, prevExists := acceptedEpochs[key]
		// check address for changes
		if err := s.checkConsensusForAddressChange(app); err != nil {
			delete(submittedEpochs, key)
			errs = append(errs, err)
			goto nextApp
		}
		if prevExists {
			err := checkEpochSequenceConstraint(acceptedEpoch, submittedEpoch)
			if err != nil {
				s.Logger.Error("Database mismatch on epochs.",
					"app", app.IApplicationAddress,
					"previous_epoch_index", acceptedEpoch.Index,
					"current_epoch_index", submittedEpoch.Index,
					"err", err,
				)
				delete(submittedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}

			// if prevClaimRow exists, there must be a matching event
			_, prevEvent, currEvent, err =
				s.blockchain.findClaimAcceptedEventAndSucc(s.Context, app, acceptedEpoch, endBlock)
			if err != nil {
				delete(submittedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
			if prevEvent == nil {
				s.Logger.Error("Missing event",
					"app", app.IApplicationAddress,
					"claim", acceptedEpoch,
					"err", ErrMissingEvent,
				)
				delete(submittedEpochs, key)
				errs = append(errs, ErrMissingEvent)
				goto nextApp
			}
			if !claimAcceptedEventMatches(app, acceptedEpoch, prevEvent) {
				s.Logger.Error("Event mismatch",
					"app", app.IApplicationAddress,
					"claim", acceptedEpoch,
					"event", prevEvent,
					"err", ErrEventMismatch,
				)
				delete(submittedEpochs, key)
				errs = append(errs, ErrEventMismatch)
				goto nextApp
			}
		} else {
			// first claim
			_, currEvent, _, err =
				s.blockchain.findClaimAcceptedEventAndSucc(s.Context, app, submittedEpoch, endBlock)
			if err != nil {
				delete(submittedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
		}

		if currEvent != nil {
			s.Logger.Debug("Found ClaimAccepted Event",
				"app", currEvent.AppContract,
				"claim_hash", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
				"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
			)
			if !claimAcceptedEventMatches(app, submittedEpoch, currEvent) {
				s.Logger.Error("event mismatch",
					"claim", submittedEpoch,
					"event", currEvent,
					"err", ErrEventMismatch,
				)
				delete(submittedEpochs, key)
				errs = append(errs, ErrEventMismatch)
				goto nextApp
			}
			s.Logger.Debug("Updating claim status to accepted",
				"app", app.IApplicationAddress,
				"claim_hash", fmt.Sprintf("%x", submittedEpoch.OutputsMerkleRoot),
				"last_block", submittedEpoch.LastBlock,
			)
			txHash := currEvent.Raw.TxHash
			err = s.repository.UpdateEpochWithAcceptedClaim(s.Context, submittedEpoch.ApplicationID, submittedEpoch.Index)
			if err != nil {
				delete(submittedEpochs, key)
				errs = append(errs, err)
				goto nextApp
			}
			s.Logger.Info("Claim accepted",
				"app", currEvent.AppContract,
				"event_block_number", currEvent.Raw.BlockNumber,
				"claim_hash", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
				"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
				"tx", txHash,
			)
		}
	nextApp:
	}
	return errs
}

// setApplicationInoperable marks an application as inoperable with the given reason,
// logs any error that occurs during the update, and returns an error with the reason.
func (s *Service) setApplicationInoperable(
	ctx context.Context,
	iApplicationAddress common.Address,
	id int64,
	reasonFmt string,
	args ...any,
) error {
	reason := fmt.Sprintf(reasonFmt, args...)
	appAddress := iApplicationAddress.String()

	// Log the reason first
	s.Logger.Error(reason, "application", appAddress)

	// Update application state
	err := s.repository.UpdateApplicationState(ctx, id, model.ApplicationState_Inoperable, &reason)
	if err != nil {
		s.Logger.Error("failed to update application state to inoperable", "app", appAddress, "err", err)
	}

	// Return the error with the reason
	return errors.New(reason)
}

func (s *Service) checkConsensusForAddressChange(
	app *model.Application,
) error {
	newConsensusAddress, err := s.blockchain.getConsensusAddress(s.Context, app)
	if err != nil {
		return err
	}
	if app.IConsensusAddress != newConsensusAddress {
		err = s.setApplicationInoperable(
			s.Context,
			app.IApplicationAddress,
			app.ID,
			"consensus change detected. application: %v.",
			app.IApplicationAddress,
		)
		return err
	}
	return nil
}

func checkEpochConstraint(c *model.Epoch) error {
	if c.FirstBlock > c.LastBlock {
		return fmt.Errorf("unexpected epoch state. first_block: %v > last_block: %v", c.FirstBlock, c.LastBlock)
	}
	if c.Status == model.EpochStatus_ClaimSubmitted {
		if c.OutputsMerkleRoot == nil {
			return fmt.Errorf("unexpected epoch state. missing claim_hash.")
		}
	}
	if c.Status == model.EpochStatus_ClaimAccepted || c.Status == model.EpochStatus_ClaimSubmitted {
		if c.ClaimTransactionHash == nil {
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
	return application.IApplicationAddress == event.AppContract &&
		*epoch.OutputsMerkleRoot == event.OutputsMerkleRoot &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64()
}

func claimAcceptedEventMatches(application *model.Application, epoch *model.Epoch, event *iconsensus.IConsensusClaimAccepted) bool {
	return application.IApplicationAddress == event.AppContract &&
		*epoch.OutputsMerkleRoot == event.OutputsMerkleRoot &&
		epoch.LastBlock == event.LastProcessedBlockNumber.Uint64()
}

func (s *Service) String() string {
	return s.Name
}

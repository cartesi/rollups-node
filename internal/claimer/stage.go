// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
)

// stageClaimsAndUpdateDatabase looks for ClaimStaged events on chain and moves
// local epochs from CLAIM_SUBMITTED to CLAIM_STAGED.
//
// If the event is for our epoch but has a different machine root, the app is
// set to DIVERGED. The reason tells the guardian to call foreclose() before
// the staging period ends.
func (s *Service) stageClaimsAndUpdateDatabase(
	ctx context.Context,
	submittedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, error) {
	return s.observeClaims(ctx, submittedEpochs, apps, defaultBlockNumber, s.processSubmittedClaim)
}

func (s *Service) processSubmittedClaim(
	ctx context.Context,
	work claimWork,
	defaultBlockNumber *big.Int,
) claimStepResult {
	app := work.app
	currEpoch := work.epoch
	if err := requireObservedStaging(currEpoch, defaultBlockNumber.Uint64()); err != nil {
		return claimRetryLater(err)
	}

	if err := s.checkConsensusForAddressChange(ctx, app, defaultBlockNumber); err != nil {
		return claimDropped(err)
	}

	fromBlock := currEpoch.LastBlock + 1

	_, currEvent, _, err := s.blockchain.findClaimStagedEventAndSucc(
		ctx, app, currEpoch, fromBlock, defaultBlockNumber.Uint64(),
	)
	if err != nil {
		return claimDropped(err)
	}

	if currEvent != nil && !claimStagedEventMatchesEpoch(app, currEpoch, currEvent) {
		currEvent = nil
	}
	if currEvent != nil {
		s.Logger.Debug("Found ClaimStaged Event",
			"app", currEvent.AppContract,
			"outputs_merkle_root", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
			"machine_hash", fmt.Sprintf("%x", currEvent.MachineMerkleRoot),
			"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
		)
		matches, ok := claimStagedEventMatches(app, currEpoch, currEvent)
		if !ok {
			return claimDropped(s.markMatcherPrecondFailure(ctx, app, currEpoch, "stageClaimsAndUpdateDatabase"))
		}
		if !matches {
			return claimDropped(s.markStagingDivergence(ctx, app, currEpoch, currEvent, "stageClaimsAndUpdateDatabase"))
		}
		if s.defaultBlock.Provisional() {
			hash, err := s.stagedClaimAuditHash(ctx, app, currEpoch, currEvent.Raw.TxHash, defaultBlockNumber.Uint64())
			if err != nil {
				return claimRetryLater(err)
			}
			if err := s.refreshClaimMetadata(ctx, currEpoch, hash, nil); err != nil {
				return claimRetryLater(err)
			}
		}
		err = s.repository.UpdateEpochToStaged(
			ctx, currEpoch.ApplicationID, currEpoch.Index,
			currEvent.Raw.BlockNumber)
		if err != nil {
			return claimDropped(err)
		}
		s.Logger.Info("Claim staged",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(currEpoch.TxBufferDataBlock),
			"last_block", currEpoch.LastBlock,
			"staged_at_block", currEvent.Raw.BlockNumber,
		)
		return claimWorkCompleted(1)
	}

	// Foreclosed apps cannot stage new claims. If no matching ClaimStaged event
	// exists up to this block, this submitted claim has no remaining on-chain
	// path.
	if app.ForecloseBlock != 0 {
		if ferr := s.forecloseClaim(ctx, app, currEpoch, defaultBlockNumber.Uint64(), "stageClaimsAndUpdateDatabase"); ferr != nil {
			return claimDropped(ferr)
		}
		return claimWorkCompleted(1)
	}
	if s.defaultBlock.Provisional() {
		if err := s.refreshSubmittedClaim(ctx, app, currEpoch, defaultBlockNumber.Uint64()); err != nil {
			return claimRetryLater(err)
		}
	}
	return claimNoProgress()
}

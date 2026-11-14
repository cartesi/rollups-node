// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"math/big"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
)

func (s *Service) validateIORPointReference(ctx context.Context, work claimWork) error {
	err := s.blockchain.checkClaimReference(ctx, work.ior.at)
	return errors.Join(err, ctx.Err())
}

func (s *Service) observedStagedClaimStatus(
	ctx context.Context, work claimWork, block *big.Int,
) (iconsensus.IConsensusClaim, error) {
	if work.ior != nil {
		return work.ior.claim, work.ior.readErr
	}
	return s.blockchain.getClaimStatus(ctx, work.app, work.epoch, block)
}

// Accepted-event evidence has precedence. This fallback/substep consumes the
// same point read, including reader mode, without another publication path.
func (s *Service) reconcileRetainedStagedIOR(
	ctx context.Context, work claimWork, block *big.Int,
) claimStepResult {
	if work.ior.readErr != nil || work.ior.claim.Status != claimStatusAccepted {
		return s.processStagedClaim(ctx, work, block)
	}
	if err := s.verifyClaimOutputsMatch(ctx, work.app, work.epoch, work.ior.claim, "observeRetainedIOR"); err != nil {
		return claimRetryLater(err)
	}
	if s.defaultBlock.Provisional() {
		if err := s.refreshStagedClaim(ctx, work.app, work.epoch, work.ior.claim, block.Uint64()); err != nil {
			return claimRetryLater(err)
		}
	}
	if err := s.validateIORPointReference(ctx, work); err != nil {
		return claimRetryLater(err)
	}
	if err := s.updateEpochAcceptedFromClaimStatus(ctx, work.app, work.epoch,
		work.ior.claim, "observeRetainedIOR", block.Uint64()); err != nil {
		return claimRetryLater(err)
	}
	return claimWorkCompleted(1)
}

// A failed accepted stream cannot establish the absence of another key. Only
// a verified positive acceptance can reconcile it; a stored stage stays a fact.
func (s *Service) fallbackStagedIORObservation(
	ctx context.Context, work claimWork, block *big.Int, observationErr error,
) claimStepResult {
	if work.ior != nil && !txsender.HardFailure(observationErr) && ctx.Err() == nil {
		if work.ior.readErr == nil && work.ior.claim.Status == claimStatusAccepted {
			result := s.reconcileRetainedStagedIOR(ctx, work, block)
			result.err = errors.Join(observationErr, result.err)
			return result
		}
		if work.epoch.StagedAtBlock != nil {
			s.reportStagedIOR(work.app, work.epoch, *work.epoch.StagedAtBlock, "stored", block.Uint64())
		}
	}
	return claimDropped(observationErr)
}

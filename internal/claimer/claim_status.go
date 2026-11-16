// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
)

const (
	claimStatusUnstaged uint8 = 0
	claimStatusStaged   uint8 = 1
	claimStatusAccepted uint8 = 2
)

func (s *Service) updateEpochAcceptedFromClaimStatus(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	claim iconsensus.IConsensusClaim,
	site string,
	observedBlock uint64,
) error {
	if err := s.verifyClaimOutputsMatch(ctx, app, epoch, claim, site); err != nil {
		return err
	}
	// getClaim is read-only. It tells us the claim state, but not the
	// transaction hash that accepted the claim. Preserve the existing audit hash;
	// only an observed event can identify a replacement transaction.
	if err := s.repository.UpdateEpochWithAcceptedClaim(
		ctx, epoch.ApplicationID, epoch.Index, nil); err != nil {
		return err
	}
	s.reportAcceptedIOR(app, epoch, "claim_status", observedBlock)
	return nil
}

func (s *Service) updateEpochStagedFromClaimStatus(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	claim iconsensus.IConsensusClaim,
	site string,
	observedBlock uint64,
) (uint64, error) {
	if err := s.verifyClaimOutputsMatch(ctx, app, epoch, claim, site); err != nil {
		return 0, err
	}
	if claim.StagingBlockNumber == nil || !claim.StagingBlockNumber.IsUint64() ||
		claim.StagingBlockNumber.Uint64() > observedBlock {
		return 0, fmt.Errorf("claim status STAGED for epoch %d (%d) has invalid or future staging block", epoch.Index, epoch.VirtualIndex)
	}
	stagingBlock := claim.StagingBlockNumber.Uint64()
	update := s.repository.UpdateEpochReconciledStaged
	if epoch.Status == model.EpochStatus_ClaimSubmitted {
		update = s.repository.UpdateEpochToStaged
	}
	if err := update(ctx, epoch.ApplicationID, epoch.Index, stagingBlock); err != nil {
		return 0, err
	}
	s.reportStagedIOR(app, epoch, stagingBlock, "claim_status", observedBlock)
	return stagingBlock, nil
}

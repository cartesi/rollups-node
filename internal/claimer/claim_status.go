// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
)

func (s *Service) updateEpochAcceptedFromClaimStatus(
	app *model.Application,
	epoch *model.Epoch,
	claim iconsensus.IConsensusClaim,
	site string,
) error {
	if err := s.verifyClaimOutputsMatch(app, epoch, claim, site); err != nil {
		return err
	}
	// getClaim is read-only. It tells us the claim state, but not the
	// transaction hash that accepted the claim. Store NULL for the hash; the
	// DB accepts this for reconciled claims.
	if err := s.repository.UpdateEpochWithAcceptedClaim(
		s.Context, epoch.ApplicationID, epoch.Index, nil); err != nil {
		return err
	}
	return nil
}

func (s *Service) updateEpochStagedFromClaimStatus(
	app *model.Application,
	epoch *model.Epoch,
	claim iconsensus.IConsensusClaim,
	site string,
) (uint64, error) {
	if err := s.verifyClaimOutputsMatch(app, epoch, claim, site); err != nil {
		return 0, err
	}
	if claim.StagingBlockNumber == nil {
		return 0, fmt.Errorf("claim status STAGED for epoch %d (%d) has nil staging block",
			epoch.Index, epoch.VirtualIndex)
	}
	stagingBlock := claim.StagingBlockNumber.Uint64()
	if err := s.repository.UpdateEpochReconciledStaged(
		s.Context, epoch.ApplicationID, epoch.Index, stagingBlock); err != nil {
		return 0, err
	}
	return stagingBlock, nil
}

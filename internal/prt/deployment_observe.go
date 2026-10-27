// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
)

func (s *Service) observeDeployment(
	ctx context.Context, app *model.Application, consensus DaveConsensusAdapter, head txsender.BlockRef,
) (deploymentSnapshot, error) {
	key := deploymentApplicationKey(app)
	captured := s.deployments.capture(key)
	if captured.snapshot.Assessment.confirmed() ||
		captured.snapshot.Assessment.State == deploymentCompatible && captured.snapshot.Head == head {
		return captured.snapshot, nil
	}
	evidence, readErr := consensus.ReadDeployment(ctx, head)
	if evidence.FactoryKnown && evidence.Cause == deploymentCauseEmpty {
		s.Logger.Warn("PRT deployment dependency is not visible at the observation head",
			"application", app.Name, "factory", evidence.Factory, "state_transition", evidence.Transition,
			"block", head.Number, "block_hash", head.Hash, "reason", evidence.Cause)
	}
	previousCanonical := true
	prior := captured.snapshot.Assessment
	if readErr == nil && hasDeploymentConfirmationPair(prior, evidence) {
		previous, err := s.client.HeaderByNumber(ctx, new(big.Int).SetUint64(prior.PendingEvidence.Head.Number))
		if err != nil {
			readErr = fmt.Errorf("rechecking deployment confirmation head: %w", err)
		} else {
			previousCanonical = previous != nil && previous.Hash() == prior.PendingEvidence.Head.Hash
		}
	}
	next := planDeploymentAssessment(captured.snapshot, evidence, readErr, previousCanonical)
	s.deployments.publish(key, next)
	return next, readErr
}

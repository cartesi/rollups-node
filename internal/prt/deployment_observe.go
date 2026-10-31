// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
)

func (s *Service) observeDeployment(
	ctx context.Context, app *model.Application, consensus DaveConsensusAdapter, head txsender.BlockRef,
) (deploymentSnapshot, error) {
	key := deploymentIdentity(app)
	captured, episodeErr := s.deploymentEpisode(ctx, app)
	if episodeErr != nil {
		return captured.snapshot, episodeErr
	}
	if captured.snapshot.Assessment.confirmed() ||
		captured.snapshot.Assessment.State == deploymentCompatible && captured.snapshot.Head == head {
		return captured.snapshot, nil
	}
	evidence, readErr := consensus.ReadDeployment(ctx, head)
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
	persistErr := s.persistDeployment(ctx, app, next)
	if persistErr != nil {
		if next.Assessment.confirmed() && captured.snapshot.Recovery {
			captured.snapshot.Recovery = false
			s.deployments.publish(key, captured.snapshot)
		}
		return captured.snapshot, errors.Join(readErr, persistErr)
	}
	if next.Assessment.State == deploymentCompatible || next.Assessment.confirmed() {
		next.Recovery = false
	}
	s.deploymentNotice(ctx, app, captured.snapshot, &next, evidence, readErr)
	s.deployments.publish(key, next)
	return next, readErr
}

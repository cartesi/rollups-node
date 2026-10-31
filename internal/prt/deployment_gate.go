// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
)

// Saved restrictions never establish fresh evidence. These explicit policy
// cases do not order the tiers or replace confirmation with a durable value.
func effectiveDeployment(snapshot deploymentSnapshot, generation int64, tier *model.DeploymentTier) deploymentState {
	if !snapshot.Acknowledged || snapshot.ResetGeneration != generation {
		return deploymentNone
	}
	state := snapshot.Assessment.State
	if state == "" || state == deploymentNone {
		return deploymentNone
	}
	if state == deploymentPending {
		return state
	}
	if tier != nil && *tier == model.DeploymentTierGeneration {
		return deploymentGeneration
	}
	if tier != nil && *tier == model.DeploymentTierTable && state == deploymentCompatible {
		return deploymentTable
	}
	return state
}

func deploymentAllowsVerdict(state deploymentState) bool {
	return state == deploymentCompatible || state == deploymentTable
}

func deploymentBlocksAction(state deploymentState, kind actionKind) bool {
	return kind != recoverBond && (state == deploymentGeneration || state == deploymentTable && kind == joinTournament)
}

// The repository closes its fresh RR transaction before returning. Identity
// confirmation therefore holds no SQL resource or lock during cache access.
func (s *Service) deploymentCompletion(ctx context.Context, app *model.Application, index uint64,
) (*repository.EpochCompletionState, deploymentState, error) {
	key := deploymentIdentity(app)
	captured := s.deployments.capture(key)
	view, err := s.repository.GetDeploymentCompletionState(ctx, deploymentIdentity(app), index)
	if err == nil && !s.deployments.same(key, captured) {
		captured = s.deployments.capture(key)
		view, err = s.repository.GetDeploymentCompletionState(ctx, deploymentIdentity(app), index)
	}
	if err != nil {
		return nil, deploymentNone, err
	}
	if view == nil || view.Completion == nil || view.Completion.Application == nil {
		return nil, deploymentNone, txsender.ErrIneligible
	}
	current := view.Completion.Application
	if current.ConsensusType != model.Consensus_PRT {
		return nil, deploymentNone, txsender.ErrIneligible
	}
	row := view.Deployment
	// Completion facts remain usable for independent obsolescence. Missing or
	// superseded admission never authorizes a useful protocol action.
	if row == nil || !s.deployments.same(key, captured) {
		return view.Completion, deploymentNone, nil
	}
	return view.Completion, effectiveDeployment(captured.snapshot, row.ResetGeneration, row.ConfirmedTier), nil
}

func (s *delivery) checkDeploymentAction(ctx context.Context, app *model.Application, index uint64,
	kind actionKind, block txsender.BlockRef,
) (*repository.EpochCompletionState, error) {
	completion, state, err := s.deploymentCompletion(ctx, app, index)
	if err != nil {
		return nil, err
	}
	if completion.Epoch == nil {
		return nil, txsender.ErrInconclusive
	}
	allowed, err := completion.AllowsEpochCompletion()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", txsender.ErrInconclusive, err)
	}
	eligibilityErr := applicationEligible(completion.Application, block)
	if eligibilityErr != nil && eligibilityErr != txsender.ErrIneligible {
		return nil, eligibilityErr
	}
	if !allowed || eligibilityErr != nil || deploymentBlocksAction(state, kind) {
		return nil, txsender.ErrIneligible
	}
	if state == deploymentNone || state == deploymentPending {
		return completion, txsender.ErrInconclusive
	}
	return completion, nil
}

func (s *Service) deploymentVerdictState(ctx context.Context, app *model.Application) (deploymentState, int64, error) {
	key := deploymentIdentity(app)
	captured := s.deployments.capture(key)
	row, err := s.repository.GetDeploymentAdmissionState(ctx, deploymentIdentity(app))
	if err != nil {
		return deploymentNone, 0, err
	}
	if !s.deployments.same(key, captured) {
		return deploymentNone, 0, txsender.ErrInconclusive
	}
	if row == nil {
		return deploymentNone, 0, nil
	}
	return effectiveDeployment(captured.snapshot, row.ResetGeneration, row.ConfirmedTier), row.ResetGeneration, nil
}

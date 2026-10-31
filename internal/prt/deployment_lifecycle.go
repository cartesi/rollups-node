// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
)

const deploymentUnknownHeadLimit = 5

func deploymentIdentity(app *model.Application) repository.DeploymentIdentity {
	return repository.DeploymentIdentity{ApplicationID: app.ID,
		ApplicationAddress: app.IApplicationAddress, ConsensusAddress: app.IConsensusAddress}
}

// A process acknowledges its startup fence before reading fresh evidence.
// Later explicit resets already advanced that fence and request a new episode.
func (s *Service) deploymentEpisode(ctx context.Context, app *model.Application) (deploymentCapture, error) {
	identity := deploymentIdentity(app)
	captured := s.deployments.capture(identity)
	row, err := s.repository.GetDeploymentAdmissionState(ctx, identity)
	if err != nil {
		return captured, err
	}
	if row == nil {
		return captured, fmt.Errorf("deployment control is unavailable: %w", txsender.ErrInconclusive)
	}
	if captured.snapshot.Acknowledged && captured.snapshot.ResetGeneration == row.ResetGeneration {
		return captured, nil
	}
	if !captured.snapshot.Acknowledged {
		row, err = s.repository.BeginDeploymentStartup(ctx, identity, row.ResetGeneration)
	}
	if err != nil {
		return captured, err
	}
	if row == nil {
		return captured, fmt.Errorf("deployment episode was not acknowledged: %w", txsender.ErrInconclusive)
	}
	fresh := deploymentSnapshot{Acknowledged: true, ResetGeneration: row.ResetGeneration, Recovery: true,
		Assessment:  deploymentAssessment{State: deploymentNone},
		Alpha6Shape: captured.snapshot.Alpha6Shape, ShapeFactory: captured.snapshot.ShapeFactory}
	s.deployments.publish(identity, fresh)
	return s.deployments.capture(identity), nil
}

func (s *Service) persistDeployment(ctx context.Context, app *model.Application, next deploymentSnapshot) error {
	request := repository.DeploymentAssessment{Identity: deploymentIdentity(app), ResetGeneration: next.ResetGeneration}
	if next.Assessment.confirmed() {
		request.ConfirmedTier = new(model.DeploymentTier(next.Assessment.State))
		request.Reason = string(next.Cause)
	} else if next.Assessment.State != deploymentCompatible || !next.Recovery {
		return nil
	}
	previous, err := s.repository.AcknowledgeDeploymentAssessment(ctx, request)
	if err != nil {
		return err
	}
	if request.ConfirmedTier == nil && previous.ConfirmedTier != nil {
		s.Logger.Info("Recovered saved PRT deployment restriction", "application", app.Name,
			"address", app.IApplicationAddress, "reset_generation", request.ResetGeneration,
			"previous_tier", *previous.ConfirmedTier, "reason", next.Cause)
	}
	return nil
}

func (s *Service) deploymentNotice(ctx context.Context, app *model.Application,
	previous deploymentSnapshot, next *deploymentSnapshot, evidence DeploymentEvidence, cause error,
) {
	if txsender.ShutdownOnly(ctx, cause) {
		return
	}
	head := evidence.Head
	state := next.Assessment.State
	if state == deploymentNone {
		if head.Hash == (common.Hash{}) || slices.Contains(next.UnknownHeadIDs[:next.UnknownHeads], head.Hash) {
			return
		}
		firstError := false
		if next.UnknownHeads < deploymentUnknownHeadLimit {
			next.UnknownHeadIDs[next.UnknownHeads] = head.Hash
			next.UnknownHeads++
			firstError = next.UnknownHeads == deploymentUnknownHeadLimit
		}
		if !warningDue(next.LastNotice) && !firstError {
			return
		}
		next.LastNotice = time.Now()
		level := slog.LevelWarn
		if next.UnknownHeads >= deploymentUnknownHeadLimit {
			level = slog.LevelError
		}
		s.Logger.Log(ctx, level, "PRT deployment evidence is unavailable", "application", app.Name,
			"address", app.IApplicationAddress, "block", head.Number, "block_hash", head.Hash,
			"distinct_heads", next.UnknownHeads, "reason", evidence.Cause,
			"factory", evidence.Factory, "state_transition", evidence.Transition, "error", cause)
	} else if state != previous.Assessment.State {
		s.Logger.Info("PRT deployment assessment changed", "application", app.Name, "address", app.IApplicationAddress,
			"state", state, "reset_generation", next.ResetGeneration, "reason", next.Cause)
	}
}

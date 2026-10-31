// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func (s *Service) forecloseGenerationClaims(ctx context.Context, app *model.Application,
	generation int64,
) error {
	if !app.Enabled || !completionStatusAllowed(app.Status) {
		return nil
	}
	filter := repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}
	epochs, _, err := s.repository.ListEpochs(ctx, app.Name, filter, repository.Pagination{}, false)
	if err != nil {
		return err
	}
	var errs []error
	for _, epoch := range epochs {
		if epoch.FirstBlock > app.ForecloseBlock || !slices.Contains(model.NonTerminalEpochStatuses(), epoch.Status) {
			continue
		}
		if epoch.Status == model.EpochStatus_InputsProcessed && epoch.LastBlock < app.ForecloseBlock {
			pending, eligibilityErr := s.observerCompletionEpoch(ctx, app, epoch)
			if eligibilityErr != nil {
				return errors.Join(append(errs, eligibilityErr)...)
			}
			if pending != nil {
				// The normal validator must atomically publish its claim and every
				// output proof. A permanent producer failure can leave this pending.
				s.Logger.Info("PRT deployment drain awaits normal claim and output proofs", "application", app.Name,
					"address", app.IApplicationAddress, "epoch", epoch.Index, "reset_generation", generation)
				continue
			}
		}
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
		current, currentGeneration, recheckErr := s.deploymentVerdictState(ctx, app)
		if recheckErr != nil {
			return errors.Join(append(errs, recheckErr)...)
		}
		if current != deploymentGeneration || currentGeneration != generation {
			return errors.Join(errs...)
		}
		request := repository.DeploymentDrainRequest{Identity: deploymentIdentity(app),
			EpochIndex: epoch.Index, ResetGeneration: generation}
		if writeErr := s.repository.ForecloseDeploymentEpoch(ctx, request); writeErr != nil {
			if writeErr == repository.ErrNoUpdate {
				continue
			}
			return errors.Join(append(errs, fmt.Errorf("deployment drain application %s epoch %d: %w",
				app.IApplicationAddress, epoch.Index, writeErr))...)
		}
		s.Logger.Info("Terminalized epoch under confirmed incompatible deployment", "application", app.Name,
			"address", app.IApplicationAddress, "epoch", epoch.Index, "reset_generation", generation)
	}
	return errors.Join(errs...)
}

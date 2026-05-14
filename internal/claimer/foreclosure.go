// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

// listEnabledForeclosedNonPRTApps returns enabled, foreclosed Authority/Quorum
// apps, keyed by Application.ID.
//
// Some foreclosed apps no longer have pending claim work, but operators still
// need to see whether pre-foreclosure work is fully drained. This query keeps
// those apps visible to processForeclosedApps.
func (s *Service) listEnabledForeclosedNonPRTApps() (map[int64]*model.Application, error) {
	apps, _, err := s.repository.ListApplications(
		s.Context,
		foreclosedClaimDrainApplicationsFilter(),
		repository.Pagination{},
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("listing enabled foreclosed apps: %w", err)
	}
	out := make(map[int64]*model.Application, len(apps))
	for _, app := range apps {
		out[app.ID] = app
	}
	return out, nil
}

func foreclosedClaimDrainApplicationsFilter() repository.ApplicationFilter {
	return repository.ApplicationFilter{
		Enabled:             new(true),
		Statuses:            []model.ApplicationStatus{model.ApplicationStatus_OK},
		ConsensusTypes:      []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum},
		ForeclosureRecorded: new(true),
	}
}

// processForeclosedApps checks foreclosed apps once per tick and logs whether
// their pre-foreclosure work is still draining. The logs cover three waiting
// points: historical input scan catch-up, pre-foreclosure input advancement,
// and claim reconciliation or CLAIM_FORECLOSED terminalization.
//
// A foreclosed application is identified by foreclose_block, not by status: a
// healthy foreclosed app is enabled with status=OK and foreclose_block set, and
// is drained here. A foreclosed app that is also DIVERGED or CORRUPTED is
// terminal and excluded by the selecting filter, but EVM reader keeps observing
// it (drive-prove, withdrawals) because observation is gated on enabled.
//
// This function does not send transactions. Submit and accept code already skip
// broadcasts when foreclose_block is set. Once all drain checks pass there is no
// final action here.
func (s *Service) processForeclosedApps(
	apps map[int64]*model.Application,
) []error {
	var errs []error
	for _, app := range apps {
		if app.ForecloseBlock == 0 {
			// This should have been filtered by the query.
			continue
		}
		// Bootstrap-readiness invariant. For a newly registered app, EVM reader
		// may record foreclose_block before it has scanned old InputAdded
		// events. Until the input scanner reaches foreclose_block, the DB may
		// look empty even though old inputs still exist on chain. Wait until
		// that scan catches up before trusting the drain queries below.
		if !app.ForeclosureScanCaughtUp() {
			s.Logger.Info(
				"Foreclosed application still ingesting pre-foreclosure inputs",
				"application", app.Name,
				"address", app.IApplicationAddress,
				"last_input_check_block", app.LastInputCheckBlock,
				"foreclose_block", app.ForecloseBlock,
			)
			continue
		}
		terminalized, err := s.repository.ForecloseUnacceptedEpochsAtOrAfterBlock(
			s.Context, app.ID, app.ForecloseBlock,
		)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"terminalizing unaccepted epochs for foreclosed app %s: %w",
				app.IApplicationAddress, err))
			continue
		}
		if terminalized > 0 {
			s.Logger.Info(
				"Foreclosed application terminalized epochs that cannot be accepted",
				"application", app.Name,
				"address", app.IApplicationAddress,
				"foreclose_block", app.ForecloseBlock,
				"epochs", terminalized,
			)
		}
		undrained, err := s.repository.HasUndrainedEpochsBeforeBlock(
			s.Context, app.ID, app.ForecloseBlock,
		)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"checking input drain progress for foreclosed app %s: %w",
				app.IApplicationAddress, err))
			continue
		}
		if undrained {
			s.Logger.Info(
				"Foreclosed application still advancing pre-foreclosure inputs",
				"application", app.Name,
				"address", app.IApplicationAddress,
				"foreclose_block", app.ForecloseBlock,
			)
			continue
		}
		unreconciled, err := s.repository.HasUnreconciledClaimsBeforeBlock(
			s.Context, app.ID, app.ForecloseBlock,
		)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"checking drain progress for foreclosed app %s: %w",
				app.IApplicationAddress, err))
			continue
		}
		if unreconciled {
			s.Logger.Info(
				"Foreclosed application still draining or reconciling pre-foreclosure epochs",
				"application", app.Name,
				"address", app.IApplicationAddress,
				"foreclose_block", app.ForecloseBlock,
			)
			continue
		}
		// All drain checks passed. There is no state change here; EVM reader
		// continues post-foreclosure observation.
	}
	return errs
}

func (s *Service) forecloseClaim(
	app *model.Application,
	epoch *model.Epoch,
	site string,
) error {
	s.Logger.Info("Claim made terminal by application foreclosure",
		"application", app.Name,
		"address", app.IApplicationAddress.String(),
		"epoch_index", epoch.Index,
		"virtual_epoch_index", epoch.VirtualIndex,
		"previous_status", epoch.Status,
		"foreclose_block", app.ForecloseBlock,
		"site", site,
	)

	if err := s.repository.UpdateEpochWithForeclosedClaim(
		s.Context, app.ID, epoch.Index); err != nil {
		return fmt.Errorf("marking epoch %d (%d) CLAIM_FORECLOSED: %w",
			epoch.Index, epoch.VirtualIndex, err)
	}
	epoch.Status = model.EpochStatus_ClaimForeclosed
	return nil
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
)

type protectionTarget struct {
	epoch        *model.Epoch
	index        uint64
	root         common.Address
	lower, upper uint64
}

// observeProtection is diagnostic only. It precedes projection and completion
// gates, and must never change their readiness, cursors, or error decisions.
func (s *Service) observeProtection(ctx context.Context, app *model.Application, epochs []*model.Epoch,
	consensus DaveConsensusAdapter, facts txsender.BlockRef, discoveryErr error,
) []prtprotection.Clock {
	if ctx.Err() != nil {
		return nil
	}
	ctx = withLiveBlock(ctx, facts.Number, facts.Hash)
	var targets []protectionTarget
	for _, epoch := range epochs {
		if epoch != nil && epoch.TournamentAddress != nil && epoch.LastBlock <= facts.Number &&
			epoch.ClaimTransactionHash == nil && slices.Contains(model.NonTerminalEpochStatuses(), epoch.Status) {
			targets = append(targets, protectionTarget{epoch, epoch.Index, *epoch.TournamentAddress,
				epoch.InputIndexLowerBound, epoch.InputIndexUpperBound})
		}
	}
	// The sealed view is independent of stage/accept readiness and event decoding.
	// A nonempty local list cannot prove that a newer root was ingested.
	var sealedErr error
	if consensus != nil {
		sealed, err := consensus.GetCurrentSealedEpoch(pinnedCallOpts(ctx, facts.Number))
		sealedErr = err
		if err == nil && sealed.Tournament != (common.Address{}) {
			known := false
			for _, epoch := range epochs {
				if epoch != nil && epoch.Index == sealed.EpochNumber && epoch.TournamentAddress != nil &&
					*epoch.TournamentAddress == sealed.Tournament {
					known = true // Preserve observed acceptance and foreclosure facts.
					if epoch.LastBlock > facts.Number && epoch.ClaimTransactionHash == nil &&
						slices.Contains(model.NonTerminalEpochStatuses(), epoch.Status) {
						// Current chain evidence identifies this obligation even when
						// its local sealing boundary cannot authenticate the clock.
						targets = append(targets, protectionTarget{epoch, epoch.Index, sealed.Tournament,
							sealed.InputIndexLowerBound, sealed.InputIndexUpperBound})
					}
					break
				}
			}
			if !known {
				targets = append(targets, protectionTarget{index: sealed.EpochNumber, root: sealed.Tournament,
					lower: sealed.InputIndexLowerBound, upper: sealed.InputIndexUpperBound})
			}
		} else if err == nil {
			sealedErr = errors.New("current sealed epoch has no root tournament")
		}
	}
	if txsender.ShutdownOnly(ctx, errors.Join(discoveryErr, sealedErr)) {
		return nil
	}
	s.reportProtectionDiscovery(app, protectionClockDiscovery, errors.Join(discoveryErr, sealedErr))
	first, stopErr := s.protectionStopBoundary(ctx, app, targets)
	if len(targets) > 0 && !txsender.ShutdownOnly(ctx, stopErr) {
		s.reportProtectionDiscovery(app, protectionBoundaryDiscovery, stopErr)
	}
	clocks := make([]prtprotection.Clock, 0, len(targets))
	for _, target := range targets {
		clock := s.readProtectionClock(ctx, app, target, facts)
		if errors.Is(ctx.Err(), context.Canceled) {
			return clocks
		}
		clocks = append(clocks, clock)
		if first != nil && target.index >= *first {
			cause := fmt.Errorf("application status %s", app.Status)
			if app.Reason != nil {
				cause = fmt.Errorf("application status %s: %s", app.Status, *app.Reason)
			}
			if s.publicationStopped() {
				cause = errors.Join(cause, s.publicationFailure())
			}
			prtprotection.ReportClock(s.Logger, app, clock, cause)
		}
		s.reportSealBacklog(app, target, clock, facts)
	}
	return clocks
}

func (s *Service) readProtectionClock(ctx context.Context, app *model.Application,
	target protectionTarget, facts txsender.BlockRef,
) prtprotection.Clock {
	clock := prtprotection.Clock{EpochIndex: target.index, Root: target.root,
		Source: fmt.Sprintf("pinned root descriptor at block %d (%s)", facts.Number, facts.Hash)}
	if target.root == (common.Address{}) {
		clock.Err = errors.New("sealed root tournament identity is unavailable")
		return clock
	}
	if target.epoch != nil && (target.epoch.ApplicationID != app.ID || target.epoch.Index != target.index) {
		clock.Err = errors.New("sealed epoch identity does not match application")
		return clock
	}
	adapter, err := s.adapterFactory.CreateTournamentAdapter(target.root)
	if err != nil {
		clock.Err = err
		return clock
	}
	descriptor, err := adapter.Descriptor(pinnedCallOpts(ctx, facts.Number))
	switch {
	case err != nil:
		clock.Err = err
	case descriptor.Level != uint64(RootLevel):
		clock.Err = errors.New("protection descriptor is not a root tournament")
	case target.epoch != nil && descriptor.StartInstant != target.epoch.LastBlock:
		clock.Err = errors.New("root protection start does not match sealed epoch")
	default:
		end, addErr := prtprotection.ClosingBlock(descriptor.StartInstant, descriptor.Allowance)
		clock.Err = addErr
		if addErr == nil {
			clock.ClosingBlock = new(end)
		}
	}
	return clock
}

func (s *Service) protectionStopBoundary(ctx context.Context, app *model.Application, targets []protectionTarget) (*uint64, error) {
	if s.publicationStopped() {
		return new(uint64(0)), nil
	}
	if app.Status == model.ApplicationStatus_Failed || app.Status == model.ApplicationStatus_Diverged ||
		app.Status == model.ApplicationStatus_Corrupted {
		return new(uint64(0)), nil
	}
	if !app.Status.IsExecutionTerminal() && app.Status != model.ApplicationStatus_InvalidOutputsRoot || len(targets) == 0 {
		return nil, nil
	}
	state, err := s.repository.GetEpochCompletionState(ctx, app.IApplicationAddress.Hex(), targets[0].index)
	if err != nil {
		return nil, err
	}
	if state == nil || state.Application == nil || state.Application.ID != app.ID ||
		state.Epoch == nil || state.Epoch.Index != targets[0].index {
		return nil, errors.New("protection stop boundary identity is unavailable")
	}
	if !state.Application.Enabled || !state.Application.Status.IsExecutionTerminal() &&
		state.Application.Status != model.ApplicationStatus_InvalidOutputsRoot {
		return nil, nil
	}
	if _, err := state.AllowsEpochCompletion(); err != nil {
		return nil, err
	}
	return new(state.Problem.EpochIndex), nil
}

// The publication latch is set before this diagnostic scan. Failed root reads
// retain their own unavailable evidence and do not suppress another application.
func (s *Service) reportGlobalProtectionStop(ctx context.Context, through int64, cause error) {
	filter := repository.ApplicationFilter{ConsensusType: new(model.Consensus_PRT), Enabled: new(true)}
	for after := int64(0); after < through; {
		apps, err := s.repository.ListApplicationsByID(ctx, filter, after, through, deliveryApplicationPageSize)
		if err != nil {
			s.Logger.Error("PRT global stop deadline discovery unavailable", "clock_available", false, "cause", errors.Join(cause, err))
			return
		}
		if len(apps) == 0 {
			return
		}
		for _, app := range apps {
			after = app.ID
			prtprotection.ReportStop(ctx, s.Logger, s.repository, app, 0, cause)
			if ctx.Err() != nil {
				return
			}
		}
	}
}

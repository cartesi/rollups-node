// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/model"
)

// This is an observation-health threshold, not a transaction retry budget.
// Only failed atomic event windows at increasing configured heads count.
const tournamentObservationFailureThreshold = 5

type tournamentObservationFailure struct {
	fromBlock   uint64
	lastHead    uint64
	failedHeads uint8
}

func (s *Service) Ready() bool {
	s.observationHealthMu.RLock()
	defer s.observationHealthMu.RUnlock()
	for _, failure := range s.observationFailures {
		if failure.failedHeads >= tournamentObservationFailureThreshold {
			return false
		}
	}
	return true
}

func (s *Service) recordTournamentObservationFailure(
	ctx context.Context, app *model.Application, head, windowEnd uint64, err error,
) {
	if errors.Is(ctx.Err(), context.Canceled) && errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return
	}

	s.observationHealthMu.Lock()
	failure, exists := s.observationFailures[app.ID]
	if exists && head <= failure.lastHead {
		s.observationHealthMu.Unlock()
		return
	}
	if !exists {
		// The caller starts an observation only when its end exceeds this
		// cursor, so the addition cannot overflow.
		failure.fromBlock = app.LastTournamentCheckBlock + 1
	}
	failure.lastHead = head
	reachedThreshold := failure.failedHeads == tournamentObservationFailureThreshold-1
	if failure.failedHeads < tournamentObservationFailureThreshold {
		failure.failedHeads++
	}
	s.observationFailures[app.ID] = failure
	s.observationHealthMu.Unlock()

	if reachedThreshold {
		s.Logger.Error("PRT tournament observation is stalled; readiness is degraded; observation will retry",
			"application", app.Name, "address", app.IApplicationAddress,
			"operation", "tournament_event_window", "from_block", failure.fromBlock,
			"through_block", windowEnd, "configured_head", head, "failed_heads", failure.failedHeads,
			"error", err)
	}
}

// Only successful atomic publication or loss of application eligibility clears
// a failure. A normal wait does not prove that the failed window can be read.
func (s *Service) clearTournamentObservationFailure(appID int64) {
	s.observationHealthMu.Lock()
	defer s.observationHealthMu.Unlock()
	delete(s.observationFailures, appID)
}

func (s *Service) pruneTournamentObservationFailures(apps []*model.Application) {
	eligible := make(map[int64]struct{}, len(apps))
	for _, app := range apps {
		eligible[app.ID] = struct{}{}
	}
	s.observationHealthMu.Lock()
	defer s.observationHealthMu.Unlock()
	for appID := range s.observationFailures {
		if _, exists := eligible[appID]; !exists {
			delete(s.observationFailures, appID)
		}
	}
}

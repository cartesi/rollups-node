// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"time"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/internal/model"
)

// This is an observation-health threshold, not a transaction retry budget.
// Only application passes with a failed interval at an increasing head count.
const tournamentObservationFailureThreshold = 5

type tournamentObservationFailure struct {
	fromBlock   uint64
	lastHead    uint64
	failedHeads uint8
	warnedAt    time.Time
}

func (s *Service) Ready() bool {
	return s.BaseTemplate.Ready() && !s.publicationStopped() && (!s.submissionEnabled ||
		s.publicationReady.Load())
}

func (s *Service) recordTournamentObservationFailure(
	ctx context.Context, app *model.Application, head, fromBlock uint64, err error,
) {
	if errors.Is(ctx.Err(), context.Canceled) && errutil.IsOnlyCancellation(err) {
		return
	}

	failure, exists := s.observationFailures[app.ID]
	if exists && head <= failure.lastHead {
		if failure.failedHeads < tournamentObservationFailureThreshold || !warningDue(failure.warnedAt) {
			return
		}
	}
	increasing := !exists || head > failure.lastHead
	failure.fromBlock = fromBlock
	failure.lastHead = head
	reachedThreshold := failure.failedHeads == tournamentObservationFailureThreshold-1
	if increasing && failure.failedHeads < tournamentObservationFailureThreshold {
		failure.failedHeads++
	}
	if reachedThreshold || failure.failedHeads >= tournamentObservationFailureThreshold && warningDue(failure.warnedAt) {
		failure.warnedAt = time.Now()
		s.Logger.Error("PRT tournament observation is stalled; incomplete intervals will retry",
			"application", app.Name, "address", app.IApplicationAddress,
			"operation", "tournament_observation", "from_block", failure.fromBlock,
			"through_block", head, "configured_head", head, "failed_heads", failure.failedHeads,
			"error", err)
	}
	s.observationFailures[app.ID] = failure
}

// A pass with completed intervals and no failed target, or explicit disable,
// clears a failure. A successful sibling alone cannot prove recovery.
func (s *Service) clearTournamentObservationFailure(appID int64) {
	delete(s.observationFailures, appID)
}

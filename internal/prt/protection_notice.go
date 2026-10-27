// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/cartesi/rollups-node/internal/txsender"
)

// The observer owns these notices. Each application retains only its current stopped roots.
func (s *Service) reportProtectionStops(ctx context.Context, app *model.Application, clocks []prtprotection.Clock,
	facts txsender.BlockRef, state deploymentState, generation int64, deploymentErr error,
) {
	if txsender.ShutdownOnly(ctx, deploymentErr) {
		return
	}
	first, stopErr := s.protectionStopBoundary(ctx, app, clocks)
	if len(clocks) > 0 && !txsender.ShutdownOnly(ctx, stopErr) {
		s.reportProtectionDiscovery(app, protectionBoundaryDiscovery, stopErr)
	}
	current := make(map[protectionNoticeKey]struct{})
	assessmentCause := s.deployments.capture(deploymentIdentity(app)).snapshot.Cause
	for _, clock := range clocks {
		current[protectionNoticeKey{root: clock.Root, kind: protectionBacklog}] = struct{}{}
		var cause error
		if first != nil && clock.EpochIndex >= *first {
			if app.Status != model.ApplicationStatus_OK {
				cause = fmt.Errorf("application status %s", app.Status)
				if app.Reason != nil {
					cause = fmt.Errorf("application status %s: %s", app.Status, *app.Reason)
				}
			}
			if s.publicationStopped() {
				cause = errors.Join(cause, s.publicationFailure())
			}
		}
		identity := protectionNoticeIdentity{generation: generation, epoch: clock.EpochIndex}
		if cause != nil {
			identity.cause = cause.Error()
		}
		if state != deploymentCompatible {
			identity.cause += "|deployment " + string(state)
			identity.cause += ":" + string(assessmentCause)
			detail := deploymentErr
			if state == deploymentNone || state == deploymentPending {
				detail = errors.Join(txsender.ErrInconclusive, detail)
			}
			cause = errors.Join(cause, fmt.Errorf("deployment assessment %s", state), detail)
		}
		if cause == nil {
			continue
		}
		identity = protectionClockIdentity(identity, clock, facts)
		key := protectionNoticeKey{root: clock.Root, kind: protectionRootStop}
		if s.protectionNoticeDue(app.ID, key, identity) {
			prtprotection.ReportClock(s.Logger, app, clock, cause)
		}
		current[key] = struct{}{}
	}
	for key := range s.protectionNotices[app.ID] {
		if key.kind == protectionClockDiscovery || key.kind == protectionBoundaryDiscovery {
			continue
		}
		if _, present := current[key]; !present {
			delete(s.protectionNotices[app.ID], key)
		}
	}
	if len(s.protectionNotices[app.ID]) == 0 {
		delete(s.protectionNotices, app.ID)
	}
}

func protectionClockIdentity(identity protectionNoticeIdentity, clock prtprotection.Clock,
	facts txsender.BlockRef,
) protectionNoticeIdentity {
	if clock.ClosingBlock != nil {
		identity.available, identity.close = true, *clock.ClosingBlock
		identity.closed = facts.Number >= identity.close
		identity.lastChance = identity.close > 0 && facts.Number == identity.close-1
	}
	return identity
}

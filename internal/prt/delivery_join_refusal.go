// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
)

// A join refusal names a protection opportunity that planning cannot publish.
// Ordinary no-work results, including an already joined commitment, are not refusals.
type joinRefusal struct {
	epoch      uint64
	tournament common.Address
	deadline   uint64
	reason     string
}

func (e *joinRefusal) Error() string { return e.reason }
func (e *joinRefusal) Unwrap() error { return txsender.ErrIneligible }

type joinRefusalWarning struct {
	epoch    uint64
	warnedAt time.Time
}

func (s *delivery) warnJoinPlanningRefusal(app *model.Application, snapshot txsender.BlockRef, err error) {
	var refusal *joinRefusal
	if !errors.As(err, &refusal) {
		return
	}
	if s.diagnostics.joinRefusals == nil {
		s.diagnostics.joinRefusals = map[int64]joinRefusalWarning{}
	}
	// Keep one warning per application, and retire inactive entries. Removed
	// applications must not leave a lifetime warning registry in the publisher.
	for id, warning := range s.diagnostics.joinRefusals {
		if time.Since(warning.warnedAt) >= 2*publicationWarningInterval {
			delete(s.diagnostics.joinRefusals, id)
		}
	}
	previous := s.diagnostics.joinRefusals[app.ID]
	if previous.epoch == refusal.epoch && !warningDue(previous.warnedAt) {
		return
	}
	var deadline any = "unknown"
	if refusal.deadline != 0 {
		deadline = refusal.deadline
	}
	s.Logger.Warn("PRT cannot join tournament; this node cannot defend this epoch",
		"application", app.Name, "address", app.IApplicationAddress, "epoch", refusal.epoch,
		"tournament", refusal.tournament, "block", snapshot.Number, "deadline", deadline, "reason", refusal.reason)
	s.diagnostics.joinRefusals[app.ID] = joinRefusalWarning{epoch: refusal.epoch, warnedAt: time.Now()}
}

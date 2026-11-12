// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"

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

func (s *delivery) warnJoinPlanningRefusal(app *model.Application, snapshot txsender.BlockRef, err error) {
	var refusal *joinRefusal
	if !errors.As(err, &refusal) {
		return
	}
	var deadline any = "unknown"
	if refusal.deadline != 0 {
		deadline = refusal.deadline
	}
	s.Logger.Warn("PRT cannot join tournament; this node cannot defend this epoch",
		"application", app.Name, "address", app.IApplicationAddress, "epoch", refusal.epoch,
		"tournament", refusal.tournament, "block", snapshot.Number, "deadline", deadline, "reason", refusal.reason)
}

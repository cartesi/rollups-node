// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
)

// Local deferrals and changed eligibility or publication snapshots are ordinary
// waits only when every cause agrees. A joined RPC/database failure remains an error.
func deliveryWaitOnly(err error) bool {
	return txsender.WaitOnly(err, repository.ErrTournamentIntervalStale, txsender.ErrLimit)
}

func (s *Service) logDeliveryWait(err error) {
	switch {
	case errors.Is(err, txsender.ErrLimit):
		s.Logger.Debug("PRT transaction waits for funds or the configured fee limit", "error", err)
	case errors.Is(err, txsender.ErrStale):
		s.Logger.Debug("PRT publication waits for the next protected head", "error", err)
	case errors.Is(err, txsender.ErrIneligible):
		s.Logger.Debug("PRT publication skips an action whose eligibility changed; planning will retry", "error", err)
	default:
		s.Logger.Debug("PRT publication waits for complete canonical observations", "error", err)
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *Service) warnUnsupportedDispute(app *Application, epochIndex uint64, tournament common.Address) {
	if _, warned := s.disputeWarnings[tournament]; warned {
		return
	}
	s.disputeWarnings[tournament] = struct{}{}
	s.Logger.Warn("This node release sends no dispute moves; the commitment can lose by timeout",
		"application", app.Name, "epoch_index", epochIndex, "tournament", tournament)
}

func (s *Service) warnZeroStagingPeriod(app *Application) {
	if app.ClaimStagingPeriod != 0 {
		return
	}
	if _, warned := s.zeroStagingWarnings[app.ID]; warned {
		return
	}
	s.zeroStagingWarnings[app.ID] = struct{}{}
	s.Logger.Warn("Application has no claim staging delay; without sentries, a staged result can be accepted immediately",
		"application", app.Name, "address", app.IApplicationAddress,
		"claim_staging_period", app.ClaimStagingPeriod)
}

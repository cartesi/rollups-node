// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/stretchr/testify/require"
)

func TestGetColumnForEvent(t *testing.T) {
	for _, test := range []struct {
		event  MonitoredEvent
		column postgres.ColumnFloat
	}{
		{MonitoredEvent_EpochSealed, table.Application.LastEpochCheckBlock},
		{MonitoredEvent_InputAdded, table.Application.LastInputCheckBlock},
		{MonitoredEvent_OutputExecuted, table.Application.LastOutputCheckBlock},
		{MonitoredEvent_CommitmentJoined, table.Application.LastTournamentCheckBlock},
		{MonitoredEvent_MatchAdvanced, table.Application.LastTournamentCheckBlock},
		{MonitoredEvent_MatchCreated, table.Application.LastTournamentCheckBlock},
		{MonitoredEvent_MatchDeleted, table.Application.LastTournamentCheckBlock},
		{MonitoredEvent_NewInnerTournament, table.Application.LastTournamentCheckBlock},
		{MonitoredEvent_LeafMatchSealed, table.Application.LastTournamentCheckBlock},
		{MonitoredEvent_PartialBondRefund, table.Application.LastTournamentCheckBlock},
		{MonitoredEvent_BondRecovered, table.Application.LastTournamentCheckBlock},
		{MonitoredEvent_ClaimSubmitted, nil},
		{MonitoredEvent_ClaimAccepted, nil},
		{MonitoredEvent_Foreclosure, nil},
		{MonitoredEvent_Withdrawal, nil},
		{MonitoredEvent_AccountsDriveMerkleRootProved, nil},
		{MonitoredEvent("unknown"), nil},
	} {
		t.Run(test.event.String(), func(t *testing.T) {
			column, err := getColumnForEvent(test.event)
			if test.column == nil {
				require.EqualError(t, err, "invalid monitored event type: "+test.event.String())
				require.Nil(t, column)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.column, column)
			}
		})
	}
}

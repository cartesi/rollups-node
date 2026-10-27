// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"time"

	"github.com/cartesi/rollups-node/internal/model"
)

func (s *ApplicationSuite) TestCreateApplicationAdvanceDeadlines() {
	const (
		legacyIncrement   = 10 * time.Second
		legacyMaximum     = 180 * time.Second
		prtIncrement      = 60 * time.Second
		prtMaximum        = 3600 * time.Second
		explicitIncrement = 7 * time.Second
		explicitMaximum   = 43 * time.Second
	)

	cases := []struct {
		name      string
		consensus model.Consensus
		explicit  bool
		increment time.Duration
		maximum   time.Duration
	}{
		{"authority_default", model.Consensus_Authority, false, legacyIncrement, legacyMaximum},
		{"quorum_default", model.Consensus_Quorum, false, legacyIncrement, legacyMaximum},
		{"prt_default", model.Consensus_PRT, false, prtIncrement, prtMaximum},
		{"authority_explicit", model.Consensus_Authority, true, explicitIncrement, explicitMaximum},
		{"quorum_explicit", model.Consensus_Quorum, true, explicitIncrement, explicitMaximum},
		{"prt_explicit", model.Consensus_PRT, true, explicitIncrement, explicitMaximum},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			builder := NewApplicationBuilder().WithConsensus(tc.consensus)
			if tc.explicit {
				builder.WithExecutionParameters(model.ExecutionParameters{
					SnapshotPolicy:        model.SnapshotPolicy_EveryEpoch,
					AdvanceIncCycles:      1,
					AdvanceMaxCycles:      1,
					InspectIncCycles:      1,
					InspectMaxCycles:      1,
					AdvanceIncDeadline:    explicitIncrement,
					AdvanceMaxDeadline:    explicitMaximum,
					InspectIncDeadline:    time.Second,
					InspectMaxDeadline:    time.Second,
					LoadDeadline:          time.Second,
					StoreDeadline:         time.Second,
					FastDeadline:          time.Second,
					MaxConcurrentInspects: 1,
				})
			}
			app := builder.Create(s.Ctx, s.T(), s.Repo)
			got, err := s.Repo.GetExecutionParameters(s.Ctx, app.ID)
			s.Require().NoError(err)
			s.Require().NotNil(got)
			s.Equal(tc.increment, got.AdvanceIncDeadline)
			s.Equal(tc.maximum, got.AdvanceMaxDeadline)
		})
	}
}

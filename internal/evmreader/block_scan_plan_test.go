// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"fmt"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBuildBlockScanPlan_RoutesScannerTargets(t *testing.T) {
	tests := []struct {
		name                string
		apps                []appContracts
		wantIConsensusInput []int64
		wantDaveEpoch       []int64
		wantOutput          []int64
		wantPostForeclosure []int64
	}{
		{
			name:                "OK IConsensus app is executable",
			apps:                []appContracts{planApp(1, planAppConfig{})},
			wantIConsensusInput: []int64{1},
			wantOutput:          []int64{1},
		},
		{
			name: "OK IConsensus app without InputBox data availability remains an input target",
			apps: []appContracts{planApp(2, planAppConfig{
				withoutInputBoxDA: true,
			})},
			wantIConsensusInput: []int64{2},
			wantOutput:          []int64{2},
		},
		{
			name: "OK DaveConsensus app is executable",
			apps: []appContracts{planApp(3, planAppConfig{
				consensus: Consensus_PRT,
			})},
			wantDaveEpoch: []int64{3},
			wantOutput:    []int64{3},
		},
		{
			name: "diverged app without foreclosure is not routed",
			apps: []appContracts{planApp(4, planAppConfig{
				status: ApplicationStatus_Diverged,
			})},
		},
		{
			name: "foreclosed IConsensus app with input cursor behind gets final input catch-up",
			apps: []appContracts{planApp(5, planAppConfig{
				forecloseBlock:      100,
				lastInputCheckBlock: 99,
			})},
			wantIConsensusInput: []int64{5},
			wantOutput:          []int64{5},
			wantPostForeclosure: []int64{5},
		},
		{
			name: "foreclosed IConsensus app without InputBox data availability skips input catch-up",
			apps: []appContracts{planApp(6, planAppConfig{
				withoutInputBoxDA:   true,
				forecloseBlock:      100,
				lastInputCheckBlock: 99,
			})},
			wantOutput:          []int64{6},
			wantPostForeclosure: []int64{6},
		},
		{
			name: "foreclosed DaveConsensus app with epoch cursor behind gets sealed-epoch catch-up",
			apps: []appContracts{planApp(7, planAppConfig{
				consensus:           Consensus_PRT,
				forecloseBlock:      100,
				lastEpochCheckBlock: 99,
			})},
			wantDaveEpoch:       []int64{7},
			wantOutput:          []int64{7},
			wantPostForeclosure: []int64{7},
		},
		{
			name: "foreclosed diverged app still catches up pre-foreclosure work",
			apps: []appContracts{planApp(8, planAppConfig{
				status:              ApplicationStatus_Diverged,
				forecloseBlock:      100,
				lastInputCheckBlock: 99,
			})},
			wantIConsensusInput: []int64{8},
			wantOutput:          []int64{8},
			wantPostForeclosure: []int64{8},
		},
		{
			name: "foreclosed app with cursor at foreclose block only keeps post-foreclosure observation",
			apps: []appContracts{planApp(9, planAppConfig{
				forecloseBlock:      100,
				lastInputCheckBlock: 100,
			})},
			wantOutput:          []int64{9},
			wantPostForeclosure: []int64{9},
		},
		{
			name: "foreclosed OK app is routed once through the foreclosed path",
			apps: []appContracts{planApp(10, planAppConfig{
				status:              ApplicationStatus_OK,
				forecloseBlock:      100,
				lastInputCheckBlock: 99,
			})},
			wantIConsensusInput: []int64{10},
			wantOutput:          []int64{10},
			wantPostForeclosure: []int64{10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := buildBlockScanPlan(tt.apps)

			require.ElementsMatch(t, tt.wantIConsensusInput, planTargetIDs(plan.iConsensusInputTargets))
			require.ElementsMatch(t, tt.wantDaveEpoch, planTargetIDs(plan.daveEpochTargets))
			require.ElementsMatch(t, tt.wantOutput, planTargetIDs(plan.outputTargets))
			require.ElementsMatch(t, tt.wantPostForeclosure, planTargetIDs(plan.postForeclosureTargets))
			require.NoError(t, requireNoDuplicatePlanTargets(plan))
		})
	}
}

func requireNoDuplicatePlanTargets(plan blockScanPlan) error {
	targets := [][]appContracts{
		plan.iConsensusInputTargets,
		plan.daveEpochTargets,
		plan.outputTargets,
		plan.postForeclosureTargets,
	}
	for _, apps := range targets {
		seen := map[int64]struct{}{}
		for _, app := range apps {
			if _, ok := seen[app.application.ID]; ok {
				return fmt.Errorf("duplicate target for application %d", app.application.ID)
			}
			seen[app.application.ID] = struct{}{}
		}
	}
	return nil
}

type planAppConfig struct {
	status              ApplicationStatus
	consensus           Consensus
	withoutInputBoxDA   bool
	forecloseBlock      uint64
	lastInputCheckBlock uint64
	lastEpochCheckBlock uint64
}

func planApp(id int64, cfg planAppConfig) appContracts {
	status := cfg.status
	if status == "" {
		status = ApplicationStatus_OK
	}
	consensus := cfg.consensus
	if consensus == "" {
		consensus = Consensus_Authority
	}
	dataAvailability := DataAvailability_InputBox[:]
	if cfg.withoutInputBoxDA {
		dataAvailability = []byte{0xff}
	}

	return appContracts{application: &Application{
		ID:                  id,
		Enabled:             true,
		Status:              status,
		ConsensusType:       consensus,
		DataAvailability:    dataAvailability,
		ForecloseBlock:      cfg.forecloseBlock,
		LastInputCheckBlock: cfg.lastInputCheckBlock,
		LastEpochCheckBlock: cfg.lastEpochCheckBlock,
	}}
}

func planTargetIDs(apps []appContracts) []int64 {
	ids := make([]int64, 0, len(apps))
	for _, app := range apps {
		ids = append(ids, app.application.ID)
	}
	return ids
}

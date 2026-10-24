// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
	"testing"
)

// Expected decisions come from the terminal-prefix requirements. The focused
// TLC model checks schedules; these cases check its database-domain boundary.
func TestEpochCompletionStateBoundary(t *testing.T) {
	cut := &model.ApplicationProblem{EpochIndex: 2, Kind: model.ApplicationStatus_MachineHalted, InputIndex: new(uint64(7))}
	for _, tc := range []struct {
		name                  string
		status                model.ApplicationStatus
		epoch                 uint64
		problem               *model.ApplicationProblem
		allowed, inconclusive bool
	}{
		{name: "healthy", status: model.ApplicationStatus_OK, allowed: true},
		{name: "earlier under ET", status: model.ApplicationStatus_MachineHalted, epoch: 1, problem: cut, allowed: true},
		{name: "earlier under IOR", status: model.ApplicationStatus_InvalidOutputsRoot, epoch: 1, problem: cut, allowed: true},
		{name: "at cutoff", status: model.ApplicationStatus_MachineHalted, epoch: 2, problem: cut},
		{name: "after cutoff", status: model.ApplicationStatus_MachineHalted, epoch: 3, problem: cut},
		{name: "healthy still respects persisted cutoff", status: model.ApplicationStatus_OK, epoch: 2, problem: cut},
		{name: "FAILED remains global", status: model.ApplicationStatus_Failed, problem: cut},
		{name: "DIVERGED remains global", status: model.ApplicationStatus_Diverged, problem: cut},
		{name: "CORRUPTED remains global", status: model.ApplicationStatus_Corrupted, problem: cut},
		{name: "missing cutoff", status: model.ApplicationStatus_MachineHalted, inconclusive: true},
		{name: "invalid kind", status: model.ApplicationStatus_MachineHalted,
			problem: &model.ApplicationProblem{EpochIndex: 2, Kind: model.ApplicationStatus_OK}, inconclusive: true},
		{name: "ET without input", status: model.ApplicationStatus_MachineHalted,
			problem: &model.ApplicationProblem{EpochIndex: 2, Kind: model.ApplicationStatus_MachineHalted}, inconclusive: true},
		{name: "validator IOR has no input", status: model.ApplicationStatus_InvalidOutputsRoot,
			problem: &model.ApplicationProblem{EpochIndex: 2, Kind: model.ApplicationStatus_InvalidOutputsRoot}, allowed: true},
		{name: "cutoff can be zero", status: model.ApplicationStatus_InvalidOutputsRoot,
			problem: &model.ApplicationProblem{EpochIndex: 0, Kind: model.ApplicationStatus_InvalidOutputsRoot}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &EpochCompletionState{Application: &model.Application{ID: 1, Status: tc.status},
				Epoch: &model.Epoch{ApplicationID: 1, Index: tc.epoch}, Problem: tc.problem}
			allowed, err := state.AllowsEpochCompletion()
			require.Equal(t, tc.allowed, allowed)
			if tc.inconclusive {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	state := &EpochCompletionState{Application: &model.Application{ID: 1, Status: model.ApplicationStatus_OK}}
	_, err := state.AllowsEpochCompletion()
	require.Error(t, err, "missing epoch is inconclusive")
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProtectionConfirmedRestrictionIsConclusive(t *testing.T) {
	for _, state := range []deploymentState{deploymentNone, deploymentPending, deploymentGeneration, deploymentTable} {
		t.Run(string(state), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			var output bytes.Buffer
			f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			clock := prtprotection.Clock{EpochIndex: 1, Root: common.HexToAddress("0x123"), ClosingBlock: new(uint64(110))}
			f.s.reportProtectionStops(t.Context(), f.app, []prtprotection.Clock{clock}, observerFacts(100), state, 0, nil)
			require.Contains(t, output.String(), "deployment assessment "+string(state))
			if state == deploymentNone || state == deploymentPending {
				require.Contains(t, output.String(), txsender.ErrInconclusive.Error())
			} else {
				require.NotContains(t, output.String(), txsender.ErrInconclusive.Error())
			}
			require.NotContains(t, output.String(), "%!w")
		})
	}
}

func TestProtectionDiscoveryIncidentSurvivesRootPruning(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	report := func(head uint64, failure error, want int) {
		t.Helper()
		output.Reset()
		facts := observerFacts(head)
		clocks := f.s.observeProtection(t.Context(), f.app, nil, nil, facts, failure)
		f.s.reportProtectionStops(t.Context(), f.app, clocks, facts, deploymentCompatible, 0, nil)
		require.Equal(t, want, strings.Count(output.String(), "PRT protection deadline discovery unavailable"), output.String())
	}
	report(100, errors.New("provider unavailable"), 1)
	report(101, errors.New("different wrapper at a newer head"), 0)
	report(102, nil, 0)
	report(103, errors.New("provider unavailable again"), 1)
	clock := prtprotection.Clock{EpochIndex: 1, Root: common.HexToAddress("0x123"), ClosingBlock: new(uint64(110))}
	f.s.reportProtectionStops(t.Context(), f.app, []prtprotection.Clock{clock}, observerFacts(103), deploymentNone, 0, nil)
	require.Contains(t, output.String(), "PRT protection stopped")
	require.Contains(t, output.String(), "join_close_block=110")
}

func TestProtectionDiscoverySuccessDoesNotClearBoundaryFailure(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.Status = model.ApplicationStatus_MachineHalted
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	epoch := checkpointEpoch(1, "0x123")
	clock := prtprotection.Clock{EpochIndex: epoch.Index, Root: *epoch.TournamentAddress, ClosingBlock: new(uint64(110))}
	state := &repository.EpochCompletionState{Application: f.app, Epoch: epoch, Problem: &model.ApplicationProblem{
		EpochIndex: epoch.Index, Kind: model.ApplicationStatus_MachineHalted, InputIndex: new(uint64(0)),
	}}
	for i, failed := range []bool{true, true, false, true} {
		output.Reset()
		var err error
		if failed {
			err = fmt.Errorf("boundary read unavailable at block %d", 100+i)
		}
		f.repo.On("GetEpochCompletionState", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).
			Return(state, err).Once()
		facts := observerFacts(uint64(100 + i))
		f.s.observeProtection(t.Context(), f.app, nil, nil, facts, nil)
		f.s.reportProtectionStops(t.Context(), f.app, []prtprotection.Clock{clock}, facts, deploymentCompatible, 0, nil)
		want := 0
		if i == 0 || i == 3 {
			want = 1
		}
		require.Equal(t, want, strings.Count(output.String(), "PRT protection deadline discovery unavailable"), output.String())
	}
}

func TestProtectionBoundaryIncidentSurvivesUnavailableInventory(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.Status = model.ApplicationStatus_MachineHalted
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	epoch := checkpointEpoch(1, "0x123")
	clock := prtprotection.Clock{EpochIndex: epoch.Index, Root: *epoch.TournamentAddress, ClosingBlock: new(uint64(110))}
	failure := errors.New("boundary database query unavailable")
	f.repo.On("GetEpochCompletionState", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).
		Return(nil, failure).Twice()
	f.s.reportProtectionStops(t.Context(), f.app, []prtprotection.Clock{clock}, observerFacts(100), deploymentCompatible, 0, nil)
	require.Equal(t, 1, strings.Count(output.String(), "PRT protection deadline discovery unavailable"))
	output.Reset()
	// The clock inventory failed. No boundary query succeeded in this pass.
	f.s.reportProtectionStops(t.Context(), f.app, nil, observerFacts(101), deploymentCompatible, 0, nil)
	f.s.reportProtectionStops(t.Context(), f.app, []prtprotection.Clock{clock}, observerFacts(102), deploymentCompatible, 0, nil)
	require.Zero(t, strings.Count(output.String(), "PRT protection deadline discovery unavailable"),
		"an absent inventory is not successful recovery of a persistent boundary failure")
}

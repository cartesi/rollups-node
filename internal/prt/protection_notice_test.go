// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestProtectionNoticesKeepImmediateTransitions(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	clock := prtprotection.Clock{EpochIndex: 1, Root: common.HexToAddress("0x123"), ClosingBlock: new(uint64(110))}
	state, generation, readErr := deploymentNone, int64(0), errors.New("read unavailable")
	facts := observerFacts(100)
	report := func(want int) {
		t.Helper()
		output.Reset()
		f.s.reportProtectionStops(t.Context(), f.app, []prtprotection.Clock{clock}, facts, state, generation, readErr)
		require.Equal(t, want, strings.Count(output.String(), "PRT protection stopped"), output.String())
	}
	report(1)
	report(0)
	facts = observerFacts(109)
	report(1)
	facts = observerFacts(110)
	report(1)
	require.Contains(t, output.String(), "join_close_block=110")
	require.NotContains(t, output.String(), "join_window_closed=")
	report(0)
	clock.ClosingBlock, clock.Err = nil, errors.New("descriptor unavailable")
	report(1)
	require.Contains(t, output.String(), "clock_available=false")
	clock.ClosingBlock, clock.Err = new(uint64(120)), nil
	report(1)
	generation++
	report(1)
	readErr = errors.New("same provider failure at another pinned head")
	report(0)
	state, readErr = deploymentGeneration, nil
	report(1)
	clock.Root = common.HexToAddress("0x456")
	report(1)
	require.Len(t, f.s.protectionNotices[f.app.ID], 1, "retired roots must not accumulate")
	notice := f.s.protectionNotices[f.app.ID][protectionNoticeKey{root: clock.Root}]
	notice.warnedAt = time.Now().Add(-publicationWarningInterval)
	f.s.protectionNotices[f.app.ID][protectionNoticeKey{root: clock.Root}] = notice
	report(1)
	state = deploymentCompatible
	report(0)
	require.Empty(t, f.s.protectionNotices)
	state = deploymentNone
	report(1)
}

func TestProtectionNoticesCombineHealthAndDeploymentStops(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.Status = model.ApplicationStatus_Failed
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	clock := prtprotection.Clock{Root: common.HexToAddress("0x123"), ClosingBlock: new(uint64(110))}
	for range 2 {
		f.s.reportProtectionStops(t.Context(), f.app, []prtprotection.Clock{clock}, observerFacts(100), deploymentNone, 0, nil)
	}
	require.Equal(t, 1, strings.Count(output.String(), "PRT protection stopped"))
	require.Contains(t, output.String(), "application status FAILED")
	require.Contains(t, output.String(), "deployment assessment none")
}

func TestProtectionNoticesKeepSiblingClocksOnOneHead(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	clocks := []prtprotection.Clock{
		{EpochIndex: 1, Root: common.HexToAddress("0x123"), Err: errors.New("descriptor unavailable")},
		{EpochIndex: 2, Root: common.HexToAddress("0x456"), ClosingBlock: new(uint64(110))},
	}
	for range 2 {
		f.s.reportProtectionStops(t.Context(), f.app, clocks, observerFacts(100), deploymentNone, 0, nil)
	}
	require.Equal(t, 2, strings.Count(output.String(), "PRT protection stopped"))
	output.Reset()
	clocks[0].ClosingBlock, clocks[0].Err = new(uint64(120)), nil
	f.s.reportProtectionStops(t.Context(), f.app, clocks, observerFacts(100), deploymentNone, 0, nil)
	require.Equal(t, 1, strings.Count(output.String(), "PRT protection stopped"))
	require.Contains(t, output.String(), clocks[0].Root.Hex())
}

func TestProtectionBacklogNoticeKeepsClockTransitions(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	epoch := checkpointEpoch(1, "0x123")
	epoch.Status = model.EpochStatus_InputsProcessed
	clock := prtprotection.Clock{EpochIndex: epoch.Index, Root: *epoch.TournamentAddress}
	target := protectionTarget{epoch: epoch, index: epoch.Index, root: clock.Root}
	for range 2 {
		f.s.reportSealBacklog(f.app, target, clock, observerFacts(100))
	}
	require.Equal(t, 1, strings.Count(output.String(), "PRT sealed epoch is not locally ready"))
	output.Reset()
	clock.ClosingBlock = new(uint64(110))
	f.s.reportSealBacklog(f.app, target, clock, observerFacts(100))
	require.Contains(t, output.String(), "join_close_block=110")
	output.Reset()
	f.s.reportSealBacklog(f.app, target, clock, observerFacts(109))
	require.Contains(t, output.String(), "PRT sealed epoch is not locally ready")
	output.Reset()
	f.s.reportSealBacklog(f.app, target, clock, observerFacts(110))
	require.Contains(t, output.String(), "join_window_closed=true")
}

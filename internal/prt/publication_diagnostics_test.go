// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/require"
)

func TestPRTHeldPublicationErrorsWarnWithBackoff(t *testing.T) {
	f, _, _ := preparePRTAction(t, stageTournament)
	require.NoError(t, f.d.tick(t.Context()))
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	failure := errors.New("provider execution read unavailable")
	f.backend.preflightError = failure
	for range 7 {
		clear(f.d.candidateCooldown)
		err := f.d.tick(t.Context())
		require.ErrorIs(t, err, failure)
		f.d.reportPublicationError(t.Context(), err)
	}
	require.Equal(t, 1, strings.Count(logs.String(), "PRT publication deferred"))
	require.Equal(t, 1, strings.Count(logs.String(), "PRT transaction could not publish"))
	require.NotContains(t, logs.String(), "level=ERROR")
	require.Contains(t, logs.String(), f.app.IApplicationAddress.Hex())
	require.Contains(t, logs.String(), "stage tournament result")
}

func TestPRTPublicationShutdownPreservesRealFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		warn bool
	}{
		{"cancelled waits", errors.Join(context.Canceled, txsender.ErrInconclusive, txsender.ErrLimit), false},
		{"cancelled database failure", errors.Join(context.Canceled, errors.New("database unavailable")), true},
		{"cancelled operation deadline", errors.Join(context.Canceled, context.DeadlineExceeded), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			f.d.reportPublicationError(ctx, test.err)
			require.Equal(t, test.warn, strings.Contains(logs.String(), "level=WARN"), logs.String())
		})
	}
}

func TestPRTRefusalNamesTheSelectedActionWhileCancellationIsPending(t *testing.T) {
	for _, canceling := range []bool{false, true} {
		f, call, _ := preparePRTAction(t, joinTournament)
		var logs bytes.Buffer
		f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
		report := txsender.Report[actionID]{Nonce: 7, Block: f.block, Canceling: canceling,
			Current: &txsender.Current[actionID]{Neutral: true}}
		f.d.warnPublicationRefusal(call.Key, report, errors.New("replacement transaction underpriced"))
		require.Contains(t, logs.String(), f.app.IApplicationAddress.Hex())
		require.Contains(t, logs.String(), "join tournament")
		require.Equal(t, canceling, strings.Contains(logs.String(), "cancel obsolete"), logs.String())
	}
}

func TestPRTConsensusRefusalWarnsBeforeIneligibleFiltering(t *testing.T) {
	f := newDeliveryFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	key := actionID{ApplicationID: f.app.ID, Kind: stageTournament, Epoch: f.epoch.Index}
	refusal := &consensusRefusal{key: key, reason: errors.Join(txsender.ErrIneligible, errors.New("consensus migrated"))}
	for range 3 {
		f.d.warnPlanningPause(f.app, stageTournament, f.block, refusal)
	}
	require.Equal(t, 1, strings.Count(logs.String(), "PRT cannot publish consensus action with the registered contracts"))
	require.Contains(t, logs.String(), f.app.Name)
	require.Contains(t, logs.String(), "consensus migrated")
	logs.Reset()
	f.d.warnPlanningPause(f.app, stageTournament, f.block, txsender.ErrIneligible)
	require.Empty(t, logs.String(), "ordinary ineligibility does not need an operator warning")
}

func TestPRTIncompleteReportDoesNotResetProgress(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	current := &txsender.Current[actionID]{Action: call.Key}
	f.d.recordLiveProgress(txsender.Report[actionID]{Block: f.block, Nonce: 7, Current: current}, nil, nil)
	f.d.diagnostics.headSince = time.Now().Add(-2 * frozenLiveHeadInterval)
	for range 3 {
		f.d.recordLiveProgress(txsender.Report[actionID]{Current: current}, nil, nil)
	}
	require.Equal(t, f.block, f.d.diagnostics.head)
	require.EqualValues(t, 7, f.d.diagnostics.nonce)
	require.Empty(t, logs.String(), "a failed account read is not evidence that block zero or nonce zero was observed")
	f.d.recordLiveProgress(txsender.Report[actionID]{Block: f.block, Nonce: 7, Current: current}, nil, nil)
	require.Contains(t, logs.String(), "PRT latest block has not advanced")
	require.NotContains(t, logs.String(), "block=0")
}

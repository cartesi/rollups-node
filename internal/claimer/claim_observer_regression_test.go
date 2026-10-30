// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const claimCheckpointCase = "checkpoint"

func seedObservedClaims(o *independentClaimObserver, app *model.Application, facts ...observedClaim) *claimObservation {
	s := &claimObservation{safe: 4, loaded: true, read: &observerBatch(29).Read,
		pending: map[observedClaimKey]*pendingObservedClaim{}}
	for _, fact := range facts {
		claim := &pendingObservedClaim{observedClaim: fact}
		claim.element = s.order.PushBack(claim)
		s.pending[fact.Key] = claim
	}
	o.apps = map[int64]*claimObservation{app.ID: s}
	return s
}

func TestIndependentClaimObserverPersistentFailureDoesNotHideReadyMismatch(t *testing.T) {
	for _, failure := range []string{"old roots", "old closure", "fresh roots", "fresh closure", "fetch", "adopt", claimCheckpointCase} {
		t.Run(failure, func(t *testing.T) {
			o, r, c, app := observerFixture(t)
			blocked := observerFact(9, 15, common.HexToHash("0x11"), common.HexToHash("0x22"))
			bad := observerFact(19, 25, common.HexToHash("0xbad"), common.HexToHash("0x44"))
			state := seedObservedClaims(o, app, blocked, bad)
			sticky := errors.New("persistent dependency failure")
			ready := failure != claimCheckpointCase
			r.rootHook = func(covered uint64) (repository.ClaimRoots, error) {
				if covered == 9 && strings.HasSuffix(failure, "roots") {
					return repository.ClaimRoots{}, sticky
				}
				if covered == 19 && ready {
					return readyRoots(common.HexToHash("0x33"), bad.Outputs), nil
				}
				return repository.ClaimRoots{ReasonClass: "validation", Reason: "validation is pending"}, nil
			}
			c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
				switch failure {
				case "fetch":
					return nil, sticky
				case "adopt":
					return observerBatch(39, blocked), nil // a repeated stage cannot replace the certified cache
				default:
					return nil, nil
				}
			}
			if strings.HasPrefix(failure, "fresh") {
				state.phase = claimObservationFresh
				state.fresh = []observedClaimKey{blocked.Key, bad.Key}
			}
			if strings.HasSuffix(failure, "closure") {
				app.ForecloseBlock, c.closureErr = 1, sticky
			}
			if failure == claimCheckpointCase {
				r.cursorErr = sticky
			}
			for visit := 0; visit < 6 && app.Status != model.ApplicationStatus_Diverged; visit++ {
				_, err := o.observe(t.Context(), 29)
				require.NoError(t, err)
				ready = true
			}
			require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
			require.NotNil(t, app.Reason)
			require.Contains(t, *app.Reason, "covered block 19")
			require.Contains(t, state.pending, blocked.Key, "failed obligations remain replayable")
			require.NotContains(t, state.pending, bad.Key)
			require.LessOrEqual(t, r.safe[app.ID], uint64(14), "R must stay before the failed stage at 15")
			require.Equal(t, 1, r.statusCalls)
		})
	}
}

func TestIndependentClaimObserverOldFailureStillDiscoversNewMismatch(t *testing.T) {
	o, r, c, app := observerFixture(t)
	blocked := observerFact(9, 15, common.HexToHash("0x11"), common.Hash{})
	bad := observerFact(19, 35, common.HexToHash("0xbad"), common.Hash{})
	state := seedObservedClaims(o, app, blocked)
	r.rootHook = func(block uint64) (repository.ClaimRoots, error) {
		if block == 9 {
			return repository.ClaimRoots{}, errors.New("old query remains unavailable")
		}
		return readyRoots(common.HexToHash("0x22"), bad.Outputs), nil
	}
	c.scan = func(_ context.Context, _ *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
		require.EqualValues(t, 29, cursor)
		return observerBatch(39, bad), nil
	}
	_, err := o.observe(t.Context(), 39)
	require.NoError(t, err)
	require.Empty(t, c.cursors, "the failed evaluation ends its visit")
	_, err = o.observe(t.Context(), 39)
	require.NoError(t, err)
	require.Equal(t, []uint64{29}, c.cursors, "a failing old claim must not prevent new discovery")
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	require.Contains(t, state.pending, blocked.Key)
	require.NotContains(t, state.pending, bad.Key)
	require.EqualValues(t, 14, r.safe[app.ID])
}

func TestIndependentClaimObserverStatusFailureKeepsStrictVerdictBarrier(t *testing.T) {
	o, r, c, app := observerFixture(t)
	first := observerFact(9, 15, common.HexToHash("0xbad"), common.Hash{})
	second := observerFact(19, 25, common.HexToHash("0xdead"), common.Hash{})
	state := seedObservedClaims(o, app, first, second)
	r.roots[9], r.roots[19] = readyRoots(common.Hash{}, common.Hash{}), readyRoots(common.Hash{}, common.Hash{})
	r.statusErr = errors.New("integrity write unavailable")
	app.ForecloseBlock = 1
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) { return nil, nil }
	for range 3 {
		more, err := o.observe(t.Context(), 29)
		require.NoError(t, err)
		require.False(t, more, "a failed status write alone must not request an immediate retry")
		require.Equal(t, model.ApplicationStatus_OK, app.Status)
		require.NotNil(t, state.verdict)
		require.Equal(t, first.Key, state.verdict.Key)
		require.Equal(t, []uint64{9}, r.rootCalls, "do not evaluate another claim before the verdict is durable")
		require.Empty(t, c.cursors)
		require.Zero(t, c.closures)
		require.Zero(t, r.cursorCalls)
	}
	r.statusErr = nil
	more, err := o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.True(t, more, "completion requests the next cached recheck at the same head")
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	firstReason := *app.Reason
	require.NotContains(t, state.pending, first.Key)
	require.Contains(t, state.pending, second.Key)
	require.Nil(t, state.verdict)
	require.EqualValues(t, 24, r.safe[app.ID], "the remaining source at25 still holds R")
	_, err = o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.Empty(t, state.pending)
	require.Equal(t, []uint64{9, 19}, r.rootCalls)
	require.Equal(t, firstReason, *app.Reason)
	require.Equal(t, 4, r.statusCalls, "three failed writes and one success; the second finding keeps that effect")
	require.EqualValues(t, 29, r.safe[app.ID])
}

func TestIndependentClaimObserverOutputsOnlyMismatchIsDurable(t *testing.T) {
	o, r, c, app := observerFixture(t)
	fact := observerFact(19, 25, common.HexToHash("0x11"), common.HexToHash("0xbad"))
	r.roots[19] = readyRoots(fact.Key.Machine, common.HexToHash("0x22"))
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
		return observerBatch(29, fact), nil
	}
	_, err := o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	require.Contains(t, *app.Reason, "independent_claim_mismatch:")
	require.Contains(t, *app.Reason, fact.Outputs.Hex())
	require.Contains(t, *app.Reason, r.roots[19].OutputsRoot.Hex())
	require.EqualValues(t, 29, r.safe[app.ID])
}

func TestIndependentClaimObserverCheckpointUsesEarliestStageAcrossFIFOOrders(t *testing.T) {
	for repeat := range 16 {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("repeat=%d/reverse=%t", repeat, reverse), func(t *testing.T) {
				o, r, c, app := observerFixture(t)
				early := observerFact(9, 15, common.HexToHash("0x11"), common.Hash{})
				late := observerFact(19, 25, common.HexToHash("0x22"), common.Hash{})
				facts := []observedClaim{early, late}
				if reverse {
					facts[0], facts[1] = facts[1], facts[0]
				}
				state := seedObservedClaims(o, app, facts...)
				c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) { return nil, nil }
				_, err := o.observe(t.Context(), 29)
				require.NoError(t, err)
				require.EqualValues(t, 14, r.safe[app.ID], "the minimum stage is 15, regardless of FIFO or map order")
				require.EqualValues(t, 14, state.safe)
				require.Len(t, state.pending, 2)
			})
		}
	}
}

func TestIndependentClaimObserverCompletedBacklogReschedulesAtFixedHead(t *testing.T) {
	o, r, c, app := observerFixture(t)
	first := observerFact(9, 15, common.HexToHash("0x11"), common.Hash{})
	second := observerFact(19, 25, common.HexToHash("0x22"), common.Hash{})
	third := observerFact(29, 35, common.HexToHash("0x33"), common.Hash{})
	state := seedObservedClaims(o, app, first, second, third)
	state.read = &observerBatch(39).Read
	r.roots[9], r.roots[19], r.roots[29] = readyRoots(first.Key.Machine, first.Outputs),
		readyRoots(second.Key.Machine, second.Outputs), readyRoots(third.Key.Machine, third.Outputs)
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) { return nil, nil }
	for visit := range 3 {
		more, err := o.observe(t.Context(), 39)
		require.NoError(t, err)
		require.Equal(t, visit < 2, more, "completed old work requests another visit while the backlog remains")
		require.Len(t, state.pending, 2-visit)
	}
	require.Equal(t, []uint64{9, 19, 29}, r.rootCalls)
	require.EqualValues(t, 39, r.safe[app.ID])
}

func TestIndependentClaimObserverUnknownAndErrorDoNotBusyLoop(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed=%t", failed), func(t *testing.T) {
			o, r, c, app := observerFixture(t)
			fact := observerFact(9, 15, common.HexToHash("0x11"), common.Hash{})
			state := seedObservedClaims(o, app, fact)
			if failed {
				r.rootErr = errors.New("root read unavailable")
			}
			c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) { return nil, nil }
			for range 4 {
				more, err := o.observe(t.Context(), 29)
				require.NoError(t, err)
				require.False(t, more)
				require.Contains(t, state.pending, fact.Key)
				require.LessOrEqual(t, r.safe[app.ID], uint64(14))
			}
		})
	}
}

func TestIndependentClaimObserverWarningClassesIgnoreMovingDetailsAndFIFO(t *testing.T) {
	o, r, c, app := observerFixture(t)
	var output bytes.Buffer
	o.logger = slog.New(slog.NewTextHandler(&output, nil))
	early := observerFact(9, 15, common.HexToHash("0x11"), common.Hash{})
	late := observerFact(19, 25, common.HexToHash("0x22"), common.Hash{})
	state := seedObservedClaims(o, app, late, early)
	for _, claim := range state.pending {
		claim.cause, claim.reason = "input_coverage", "input coverage ends at block 0"
	}
	coverage := 0
	r.rootHook = func(covered uint64) (repository.ClaimRoots, error) {
		coverage++
		return repository.ClaimRoots{ReasonClass: "input_coverage",
			Reason: fmt.Sprintf("input coverage ends at block %d before %d", coverage, covered)}, nil
	}
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) { return nil, nil }
	for range 6 {
		_, err := o.observe(t.Context(), 29)
		require.NoError(t, err)
	}
	require.Equal(t, 1, strings.Count(output.String(), "Independent claim verification is incomplete"))
	require.Contains(t, output.String(), "example_covered_block=9")
	state.warnings[claimWarningPending].at = state.warnings[claimWarningPending].at.Add(-claimWarningInterval)
	state.order.MoveToFront(state.pending[early.Key].element)
	_, err := o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(output.String(), "example_covered_block=9"))
	require.NotContains(t, output.String(), "example_covered_block=19", "the FIFO front after rotation is not the stable example")
	o.warn(app, state, claimWarningObservation, "dependency", "provider failed at block 100")
	o.warn(app, state, claimWarningObservation, "dependency", "provider failed at block 101")
	o.warn(app, state, claimWarningCheckpoint, "checkpoint_read", "checkpoint unavailable at attempt 1")
	o.warn(app, state, claimWarningCheckpoint, "checkpoint_read", "checkpoint unavailable at attempt 2")
	o.warn(app, state, claimWarningObservation, "dependency", "provider failed at block 102")
	o.warn(app, state, claimWarningPending, "input_coverage", "input coverage ends at block 100")
	require.Equal(t, 4, strings.Count(output.String(), "Independent claim verification is incomplete"),
		"pending, observation and checkpoint warnings have independent throttles")
}

func TestIndependentClaimObserverCheckpointReadWarningsArePerApplication(t *testing.T) {
	o, r, c, app := observerFixture(t)
	sibling := *app
	sibling.ID, sibling.Name, sibling.IApplicationAddress = 2, "sibling", common.HexToAddress("0x2")
	r.apps = append(r.apps, &sibling)
	r.checkpointHook = func(int64) (uint64, error) { return 0, errors.New("checkpoint database unavailable") }
	var output bytes.Buffer
	o.logger = slog.New(slog.NewTextHandler(&output, nil))
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
		t.Fatal("discovery cannot start before its checkpoint is loaded")
		return nil, nil
	}
	for range 3 {
		more, err := o.observe(t.Context(), 29)
		require.NoError(t, err)
		require.False(t, more)
	}
	require.Equal(t, 2, strings.Count(output.String(), "cause_class=checkpoint_read"))
	require.Contains(t, output.String(), "application="+app.Name)
	require.Contains(t, output.String(), "application=sibling")
	require.False(t, o.apps[app.ID].loaded)
	require.False(t, o.apps[sibling.ID].loaded)
}

func TestIndependentClaimObserverNoncanonicalReadEvictsAndReplaysSafePrefix(t *testing.T) {
	o, r, c, app := observerFixture(t)
	fact := observerFact(19, 25, common.HexToHash("0xbad"), common.Hash{})
	old := seedObservedClaims(o, app, fact)
	r.safe[app.ID] = 14
	old.safe = 14
	c.checkErr = txsender.ErrNonCanonical
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
		t.Fatal("an invalid cached reference must be evicted before discovery")
		return nil, nil
	}
	_, err := o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.NotContains(t, o.apps, app.ID)
	require.Empty(t, r.rootCalls)
	require.EqualValues(t, 14, r.safe[app.ID])
	c.checkErr = nil
	r.roots[19] = readyRoots(common.Hash{}, fact.Outputs)
	c.scan = func(_ context.Context, _ *model.Application, cursor uint64, read *claimRead) (*claimScan, error) {
		require.EqualValues(t, 14, cursor)
		require.Nil(t, read)
		return observerBatch(29, fact), nil
	}
	_, err = o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.NotSame(t, old, o.apps[app.ID])
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
}

func TestIndependentClaimObserverForeclosureUsesCertifiedReadAndIgnoresStoredHeight(t *testing.T) {
	for _, stored := range []uint64{1, 1000} {
		t.Run(fmt.Sprintf("stored=%d", stored), func(t *testing.T) {
			o, r, c, app := observerFixture(t)
			app.ForecloseBlock = stored
			fact := observerFact(19, 25, common.HexToHash("0x11"), common.Hash{})
			c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
				return observerBatch(29, fact), nil
			}
			c.closureAt = func(ref txsender.BlockRef) (bool, error) {
				require.Equal(t, observerBatch(29).Read.At, ref, "the newer observed head is not a certified claim read")
				return true, nil
			}
			_, err := o.observe(t.Context(), 100)
			require.NoError(t, err)
			require.Empty(t, o.apps[app.ID].pending)
			require.EqualValues(t, 29, r.safe[app.ID])
			require.Equal(t, 1, c.closures)
		})
	}
}

func TestIndependentClaimObserverFalseForeclosureIsRetriedAtNewCertifiedRead(t *testing.T) {
	o, r, c, app := observerFixture(t)
	app.ForecloseBlock = 1
	fact := observerFact(19, 25, common.HexToHash("0x11"), common.Hash{})
	c.closureAt = func(ref txsender.BlockRef) (bool, error) { return ref.Number >= 39, nil }
	c.scan = func(_ context.Context, _ *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
		switch cursor {
		case 0:
			return observerBatch(29, fact), nil
		case 29:
			return observerBatch(39), nil
		default:
			return nil, nil
		}
	}
	for visit := range 3 {
		_, err := o.observe(t.Context(), 39)
		require.NoError(t, err)
		if visit < 2 {
			require.Contains(t, o.apps[app.ID].pending, fact.Key)
			require.EqualValues(t, 24, r.safe[app.ID])
		}
	}
	require.Empty(t, o.apps[app.ID].pending)
	require.EqualValues(t, 39, r.safe[app.ID])
	require.Equal(t, []txsender.BlockRef{observerBatch(29).Read.At, observerBatch(39).Read.At}, c.closureRefs,
		"a false flag is cached only for its exact certified read")
}

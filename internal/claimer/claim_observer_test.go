// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

type observationTestRepo struct {
	apps                          []*model.Application
	safe                          map[int64]uint64
	roots                         map[uint64]repository.ClaimRoots
	rootCalls                     []uint64
	rootHook                      func(uint64) (repository.ClaimRoots, error)
	appHook                       func(string) (*model.Application, error)
	checkpointHook                func(int64) (uint64, error)
	statusCalls, cursorCalls      int
	statusErr, cursorErr, rootErr error
	commitOnError                 bool
	concurrent                    model.ApplicationStatus
}

func (r *observationTestRepo) ListApplicationsByID(
	_ context.Context, _ repository.ApplicationFilter, after, through int64, _ uint64,
) ([]*model.Application, error) {
	for _, app := range r.apps {
		if app.ID > after && app.ID <= through && app.Enabled {
			stored := *app
			return []*model.Application{&stored}, nil
		}
	}
	return nil, nil
}
func (r *observationTestRepo) GetApplication(_ context.Context, address string) (*model.Application, error) {
	if r.appHook != nil {
		return r.appHook(address)
	}
	for _, app := range r.apps {
		if app.IApplicationAddress.Hex() == address {
			stored := *app
			return &stored, nil
		}
	}
	return nil, nil
}
func (r *observationTestRepo) GetEventLastCheckBlock(_ context.Context, id int64, _ model.MonitoredEvent) (uint64, error) {
	if r.checkpointHook != nil {
		return r.checkpointHook(id)
	}
	return r.safe[id], nil
}
func (r *observationTestRepo) UpdateEventLastCheckBlock(_ context.Context, ids []int64, _ model.MonitoredEvent, block uint64) error {
	r.cursorCalls++
	if r.cursorErr != nil {
		return r.cursorErr
	}
	for _, id := range ids {
		if block > r.safe[id] {
			r.safe[id] = block
		}
	}
	return nil
}
func (r *observationTestRepo) ReadClaimRoots(_ context.Context, _ int64, block uint64) (repository.ClaimRoots, error) {
	r.rootCalls = append(r.rootCalls, block)
	if r.rootHook != nil {
		return r.rootHook(block)
	}
	return r.roots[block], r.rootErr
}
func (r *observationTestRepo) UpdateApplicationStatus(_ context.Context, id int64, status model.ApplicationStatus, reason *string) error {
	r.statusCalls++
	for _, app := range r.apps {
		if app.ID == id {
			if r.concurrent != "" {
				app.Status = r.concurrent
				app.Reason = new("first concurrent integrity finding")
				return errors.New("stale terminal write")
			}
			if r.statusErr == nil || r.commitOnError {
				app.Status = status
				stored := *reason
				app.Reason = &stored
			}
			return r.statusErr
		}
	}
	return repository.ErrNotFound
}

type observationTestChain struct {
	scan        func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error)
	cursors     []uint64
	checkErr    error
	closures    int
	closureErr  error
	closed      bool
	closureAt   func(txsender.BlockRef) (bool, error)
	closureRefs []txsender.BlockRef
}

func (c *observationTestChain) scanClaims(
	ctx context.Context, app *model.Application, cursor uint64, read *claimRead, _ uint64,
) (*claimScan, error) {
	c.cursors = append(c.cursors, cursor)
	return c.scan(ctx, app, cursor, read)
}
func (c *observationTestChain) checkClaimRead(context.Context, txsender.BlockRef) error {
	return c.checkErr
}
func (c *observationTestChain) claimForeclosureFinalized(
	_ context.Context, _ *model.Application, ref txsender.BlockRef,
) (bool, error) {
	c.closures++
	c.closureRefs = append(c.closureRefs, ref)
	if c.closureAt != nil {
		return c.closureAt(ref)
	}
	return c.closed, c.closureErr
}

func observerFixture(t *testing.T) (*independentClaimObserver, *observationTestRepo, *observationTestChain, *model.Application) {
	t.Helper()
	app := makeApplication()
	app.ID = 1
	app.Enabled = true
	r := &observationTestRepo{apps: []*model.Application{app}, safe: map[int64]uint64{}, roots: map[uint64]repository.ClaimRoots{}}
	c := &observationTestChain{closed: true}
	o := &independentClaimObserver{repository: r, blockchain: c, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		policy: model.DefaultBlock_Finalized, budget: time.Second}
	return o, r, c, app
}
func observerFact(block, source uint64, machine, outputs common.Hash) observedClaim {
	return observedClaim{Key: observedClaimKey{Covered: new(big.Int).SetUint64(block).String(), Machine: machine}, Outputs: outputs,
		Stage: &types.Log{BlockNumber: source, BlockHash: common.BigToHash(new(big.Int).SetUint64(source)),
			TxHash: common.HexToHash("0x1")},
		StagingBlock: new(big.Int).SetUint64(source), Period: big.NewInt(20)}
}
func observerBatch(head uint64, facts ...observedClaim) *claimScan {
	return &claimScan{Read: claimRead{At: txsender.BlockRef{Number: head, Hash: common.BigToHash(new(big.Int).SetUint64(head))},
		Staged: big.NewInt(1), Accepted: new(big.Int)}, Claims: facts}
}
func readyRoots(machine, outputs common.Hash) repository.ClaimRoots {
	return repository.ClaimRoots{Ready: true, MachineHash: machine, OutputsRoot: outputs}
}

func TestIndependentClaimObserverCheckpointReconstructsOldPendingAfterRestart(t *testing.T) {
	o, r, c, app := observerFixture(t)
	fact := observerFact(9, 15, common.HexToHash("0xbad"), common.HexToHash("0x22"))
	r.safe[app.ID] = 4
	c.scan = func(_ context.Context, _ *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
		require.Contains(t, []uint64{4, 14}, cursor)
		return observerBatch(19, fact), nil
	}
	_, err := o.observe(t.Context(), 19)
	require.NoError(t, err)
	require.EqualValues(t, 14, r.safe[app.ID], "a pending S=15 holds R below 15")
	require.Len(t, o.apps[app.ID].pending, 1)
	// A long outage changes H, not the safe replay start. There is no age window.
	restarted := &independentClaimObserver{repository: r, blockchain: c, logger: o.logger, policy: o.policy, budget: time.Second}
	r.roots[9] = readyRoots(common.HexToHash("0x11"), fact.Outputs)
	c.scan = func(_ context.Context, _ *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
		require.EqualValues(t, 14, cursor, "resume immediately before the unresolved source")
		return observerBatch(100000, fact), nil
	}
	_, err = restarted.observe(t.Context(), 100000)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	require.EqualValues(t, 100000, r.safe[app.ID])
	require.Empty(t, restarted.apps[app.ID].pending)
}

func TestIndependentClaimObserverRetainsProofUntilStatusWriteWorks(t *testing.T) {
	o, r, c, app := observerFixture(t)
	fact := observerFact(9, 15, common.HexToHash("0xbad"), common.HexToHash("0x22"))
	r.roots[9] = readyRoots(common.HexToHash("0x11"), fact.Outputs)
	r.statusErr = errors.New("database write failed")
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
		return observerBatch(19, fact), nil
	}
	_, err := o.observe(t.Context(), 19)
	require.NoError(t, err, "app-owned errors do not escape Tick")
	require.Zero(t, r.safe[app.ID])
	require.NotNil(t, o.apps[app.ID].verdict)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	app.ForecloseBlock = 18
	app.ForecloseTransaction = new(common.HexToHash("0xf"))
	r.statusErr = nil
	r.rootErr = errors.New("comparison is temporarily unavailable")
	_, err = o.observe(t.Context(), 19)
	require.NoError(t, err)
	require.Equal(t, []uint64{9}, r.rootCalls, "retry the proved verdict without another root read")
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	require.EqualValues(t, 19, r.safe[app.ID])
	require.Zero(t, c.closures, "foreclosure cannot erase a proved mismatch")
}

func TestIndependentClaimObserverCrashBeforeStatusRebuildsQualifiedResult(t *testing.T) {
	o, r, c, app := observerFixture(t)
	fact := observerFact(9, 15, common.HexToHash("0xbad"), common.HexToHash("0x22"))
	fact.Acceptance = &types.Log{BlockNumber: 17}
	r.roots[9] = readyRoots(common.HexToHash("0x11"), fact.Outputs)
	r.statusErr = errors.New("database write failed")
	c.scan = func(_ context.Context, _ *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
		require.Zero(t, cursor)
		return observerBatch(19, fact), nil
	}
	_, err := o.observe(t.Context(), 19)
	require.NoError(t, err)
	require.Zero(t, r.safe[app.ID])
	require.NotNil(t, o.apps[app.ID].verdict)
	// S1 tests the real CF marker. Here the root provider continues to return
	// that qualified result after the observer loses every volatile field.
	r.statusErr = nil
	restarted := &independentClaimObserver{repository: r, blockchain: c, logger: o.logger, policy: o.policy, budget: time.Second}
	_, err = restarted.observe(t.Context(), 19)
	require.NoError(t, err)
	require.Equal(t, []uint64{9, 9}, r.rootCalls)
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	require.EqualValues(t, 19, r.safe[app.ID])
}

func TestIndependentClaimObserverCurrentIntegrityAndAmbiguousStatusCommit(t *testing.T) {
	for _, status := range []model.ApplicationStatus{"", model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		t.Run(string(status), func(t *testing.T) {
			o, r, c, app := observerFixture(t)
			fact := observerFact(9, 15, common.HexToHash("0xbad"), common.HexToHash("0x22"))
			r.roots[9] = readyRoots(common.HexToHash("0x11"), fact.Outputs)
			r.statusErr = errors.New("write response lost")
			r.commitOnError = true
			r.concurrent = status
			c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
				return observerBatch(19, fact), nil
			}
			_, err := o.observe(t.Context(), 19)
			require.NoError(t, err)
			require.EqualValues(t, 19, r.safe[app.ID])
			require.Nil(t, o.apps[app.ID].verdict)
			if status != "" {
				require.Equal(t, status, app.Status)
				require.Equal(t, "first concurrent integrity finding", *app.Reason)
			} else {
				require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
			}
		})
	}
}

func TestIndependentClaimObserverIntegrityDoesNotCompleteAnotherUnknown(t *testing.T) {
	o, r, c, app := observerFixture(t)
	unknown := observerFact(19, 25, common.HexToHash("0x33"), common.HexToHash("0x44"))
	bad := observerFact(9, 25, common.HexToHash("0xbad"), common.HexToHash("0x22"))
	r.roots[9] = readyRoots(common.HexToHash("0x11"), bad.Outputs)
	c.scan = func(_ context.Context, _ *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
		if cursor == 29 {
			return nil, nil
		}
		return observerBatch(29, unknown, bad), nil
	}
	_, err := o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	first := *app.Reason
	require.EqualValues(t, 24, r.safe[app.ID])
	require.Len(t, o.apps[app.ID].pending, 1)
	// A validation already in flight can finish after DF. Health alone must not
	// suppress the recheck. The real query intentionally has no app-OK gate.
	r.roots[19] = readyRoots(unknown.Key.Machine, unknown.Outputs)
	_, err = o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.Equal(t, []uint64{19, 9, 19}, r.rootCalls)
	require.EqualValues(t, 29, r.safe[app.ID])
	require.Equal(t, first, *app.Reason)
	require.Equal(t, 1, r.statusCalls)
}

func TestIndependentClaimObserverFreshAttemptDoesNotWaitForOldBacklog(t *testing.T) {
	o, r, c, app := observerFixture(t)
	state := &claimObservation{safe: 14, loaded: true,
		pending: map[observedClaimKey]*pendingObservedClaim{}, read: &observerBatch(19).Read}
	for i := uint64(1); i <= 100; i++ {
		fact := observerFact(9, 15, common.BigToHash(new(big.Int).SetUint64(i)), common.Hash{})
		entry := &pendingObservedClaim{observedClaim: fact}
		entry.element = state.order.PushBack(entry)
		state.pending[fact.Key] = entry
	}
	fresh := observerFact(19, 25, common.HexToHash("0xff"), common.Hash{})
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
		return observerBatch(29, fresh), nil
	}
	o.apps = map[int64]*claimObservation{app.ID: state}
	_, err := o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.Equal(t, []uint64{9, 19}, r.rootCalls, "one old recheck then every fresh fact")
	require.Len(t, state.pending, 101)
	require.EqualValues(t, 14, state.safe)
}

func TestIndependentClaimObserverSoftAdmissionFinishesExpensiveUnit(t *testing.T) {
	o, r, c, app := observerFixture(t)
	o.budget = time.Millisecond
	first := observerFact(9, 25, common.HexToHash("0x11"), common.Hash{})
	second := observerFact(19, 25, common.HexToHash("0x22"), common.Hash{})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	c.scan = func(observed context.Context, _ *model.Application, _ uint64, _ *claimRead) (*claimScan, error) {
		actual, ok := observed.Deadline()
		require.True(t, ok)
		require.Equal(t, deadline, actual, "poll pacing must not shorten I/O context")
		time.Sleep(10 * time.Millisecond)
		return observerBatch(29, first, second), nil
	}
	_, err := o.observe(ctx, 29)
	require.NoError(t, err)
	require.Equal(t, []uint64{9, 19}, r.rootCalls, "the admitted range receives all first evaluations after budget expiry")
	require.EqualValues(t, 24, r.safe[app.ID])
	require.Len(t, o.apps[app.ID].pending, 2)
}

func TestIndependentClaimObserverCheckpointRetryAndCrashAfterStatus(t *testing.T) {
	o, r, c, app := observerFixture(t)
	fact := observerFact(9, 15, common.HexToHash("0xbad"), common.HexToHash("0x22"))
	r.roots[9] = readyRoots(common.HexToHash("0x11"), fact.Outputs)
	r.cursorErr = errors.New("checkpoint response failed")
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
		return observerBatch(19, fact), nil
	}
	_, err := o.observe(t.Context(), 19)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	reason := *app.Reason
	require.Zero(t, r.safe[app.ID])
	require.Equal(t, 1, r.statusCalls)
	r.cursorErr = nil
	restarted := &independentClaimObserver{repository: r, blockchain: c, logger: o.logger, policy: o.policy, budget: time.Second}
	_, err = restarted.observe(t.Context(), 19)
	require.NoError(t, err)
	require.EqualValues(t, 19, r.safe[app.ID])
	require.Equal(t, 1, r.statusCalls)
	require.Equal(t, reason, *app.Reason)
}

func TestIndependentClaimObserverClosureRequiresFullFinalizedAcceptanceStream(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Finalized, model.DefaultBlock_Latest, model.DefaultBlock_Safe} {
		t.Run(string(policy), func(t *testing.T) {
			o, r, c, app := observerFixture(t)
			o.policy = policy
			app.ForecloseBlock = 30
			app.ForecloseTransaction = new(common.HexToHash("0xf"))
			fact := observerFact(19, 25, common.Hash{}, common.Hash{})
			c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
				return observerBatch(39, fact), nil
			}
			_, err := o.observe(t.Context(), 39)
			require.NoError(t, err)
			if policy == model.DefaultBlock_Finalized {
				require.EqualValues(t, 39, r.safe[app.ID])
				require.Equal(t, 1, c.closures)
			} else {
				require.EqualValues(t, 24, r.safe[app.ID])
				require.Zero(t, c.closures)
				require.Len(t, o.apps[app.ID].pending, 1)
			}
			require.Zero(t, r.statusCalls)
		})
	}
	t.Run("acceptance in F is retained after a partial range", func(t *testing.T) {
		o, r, c, app := observerFixture(t)
		app.ForecloseBlock = 30
		app.ForecloseTransaction = new(common.HexToHash("0xf"))
		fact := observerFact(19, 25, common.Hash{}, common.Hash{})
		c.closureAt = func(ref txsender.BlockRef) (bool, error) { return ref.Number >= 30, nil }
		c.scan = func(_ context.Context, _ *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
			if cursor == 0 {
				return observerBatch(29, fact), nil
			}
			acceptance := observedClaim{Key: fact.Key, Outputs: fact.Outputs, Acceptance: &types.Log{BlockNumber: 30}}
			return observerBatch(39, acceptance), nil
		}
		more, err := o.observe(t.Context(), 39)
		require.NoError(t, err)
		require.True(t, more)
		require.Equal(t, 1, c.closures, "the false flag at the certified read keeps the claim pending")
		require.EqualValues(t, 24, r.safe[app.ID])
		_, err = o.observe(t.Context(), 39)
		require.NoError(t, err)
		require.Equal(t, 1, c.closures, "the accepted projection prevents another closure attempt")
		require.EqualValues(t, 24, r.safe[app.ID])
		require.Len(t, o.apps[app.ID].pending, 1)
		require.NotNil(t, o.apps[app.ID].pending[fact.Key].Acceptance)
	})
}

func TestIndependentClaimObserverAppFaultDoesNotBlockSibling(t *testing.T) {
	o, r, c, app := observerFixture(t)
	sibling := *app
	sibling.ID = 2
	sibling.IApplicationAddress = common.HexToAddress("0x2")
	r.apps = append(r.apps, &sibling)
	fact := observerFact(9, 15, common.HexToHash("0x11"), common.Hash{})
	r.roots[9] = readyRoots(fact.Key.Machine, fact.Outputs)
	c.scan = func(_ context.Context, a *model.Application, _ uint64, _ *claimRead) (*claimScan, error) {
		if a.ID == app.ID {
			return nil, errors.New("one app RPC history failed")
		}
		return observerBatch(19, fact), nil
	}
	_, err := o.observe(t.Context(), 19)
	require.NoError(t, err)
	require.Zero(t, r.safe[app.ID])
	require.EqualValues(t, 19, r.safe[sibling.ID])
}

func TestIndependentClaimObserverPendingWarningNamesDeadlineAndProof(t *testing.T) {
	o, _, c, _ := observerFixture(t)
	var log bytes.Buffer
	o.logger = slog.New(slog.NewTextHandler(&log, nil))
	fact := observerFact(9, 15, common.HexToHash("0x11"), common.HexToHash("0x22"))
	c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
		return observerBatch(19, fact), nil
	}
	_, err := o.observe(t.Context(), 19)
	require.NoError(t, err)
	require.Contains(t, log.String(), "pending_claims=1")
	require.Contains(t, log.String(), "example_covered_block=9")
	require.Contains(t, log.String(), "example_phase=staged")
	require.Contains(t, log.String(), "acceptance_from_contract_block=35")
	require.Contains(t, log.String(), fact.Key.Machine.Hex())
	require.Contains(t, log.String(), fact.Outputs.Hex())
}

type emptyObservedLifecycle struct{ iclaimerRepository }

func (*emptyObservedLifecycle) SelectClaimsToSubmitPage(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error) {
	return repository.ClaimPage{}, nil
}
func (*emptyObservedLifecycle) SelectClaimsToStagePage(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error) {
	return repository.ClaimPage{}, nil
}
func (*emptyObservedLifecycle) SelectClaimsToAcceptPage(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error) {
	return repository.ClaimPage{}, nil
}
func (*emptyObservedLifecycle) ListApplicationsByID(
	context.Context, repository.ApplicationFilter, int64, int64, uint64,
) ([]*model.Application, error) {
	return nil, nil
}

type observedTickHead struct{ iclaimerBlockchain }

func (*observedTickHead) getDefaultBlockNumber(context.Context) (*big.Int, error) {
	return big.NewInt(29), nil
}

func TestIndependentClaimObserverTickDoesNotNeedAnEligibleLocalEpoch(t *testing.T) {
	for _, submit := range []bool{false, true} {
		t.Run(fmt.Sprintf("submission=%t", submit), func(t *testing.T) {
			o, r, c, app := observerFixture(t)
			fact := observerFact(19, 25, common.HexToHash("0xbad"), common.HexToHash("0x22"))
			r.roots[19] = readyRoots(common.HexToHash("0x11"), fact.Outputs)
			c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
				return observerBatch(29, fact), nil
			}
			service := &Service{repository: &emptyObservedLifecycle{}, blockchain: &observedTickHead{},
				claimObserver: o, submissionEnabled: submit}
			_, err := service.Tick(t.Context())
			require.NoError(t, err)
			require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
			require.EqualValues(t, 29, r.safe[app.ID])
		})
	}
}

func TestIndependentClaimObserverFreshQueryFailureReturnsToOldQueue(t *testing.T) {
	o, r, c, app := observerFixture(t)
	first := observerFact(9, 25, common.HexToHash("0x11"), common.Hash{})
	second := observerFact(19, 25, common.HexToHash("0x22"), common.Hash{})
	queryFailed := true
	r.rootHook = func(block uint64) (repository.ClaimRoots, error) {
		if block == 9 {
			return readyRoots(first.Key.Machine, first.Outputs), nil
		}
		if queryFailed {
			return repository.ClaimRoots{}, errors.New("local query failed")
		}
		return repository.ClaimRoots{Reason: "validation is pending"}, nil
	}
	c.scan = func(_ context.Context, _ *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
		if cursor == 29 {
			return nil, nil
		}
		return observerBatch(29, first, second), nil
	}
	_, err := o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.Zero(t, r.safe[app.ID])
	require.Empty(t, o.apps[app.ID].fresh, "the failed key stays pending but does not block the fresh suffix")
	require.Len(t, o.apps[app.ID].pending, 1)
	queryFailed = false
	_, err = o.observe(t.Context(), 29)
	require.NoError(t, err)
	_, err = o.observe(t.Context(), 29)
	require.NoError(t, err)
	require.Equal(t, []uint64{9, 19, 19}, r.rootCalls)
	require.Equal(t, []uint64{0, 29}, c.cursors, "the failed key receives an ordinary old recheck")
	require.EqualValues(t, 24, r.safe[app.ID])
	require.Len(t, o.apps[app.ID].pending, 1)
}

func TestIndependentClaimObserverEvictsWholeInactiveUnitAndReplaysFromSafePoint(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(fmt.Sprintf("removed=%t", removed), func(t *testing.T) {
			o, r, c, app := observerFixture(t)
			sibling := *app
			sibling.ID = 2
			sibling.IApplicationAddress = common.HexToAddress("0x2")
			r.apps = append(r.apps, &sibling)
			fact := observerFact(19, 25, common.HexToHash("0x11"), common.Hash{})
			var replay []uint64
			c.scan = func(_ context.Context, a *model.Application, cursor uint64, _ *claimRead) (*claimScan, error) {
				if a.ID == 2 {
					return nil, nil
				}
				replay = append(replay, cursor)
				return observerBatch(29, fact), nil
			}
			_, err := o.observe(t.Context(), 29)
			require.NoError(t, err)
			old := o.apps[app.ID]
			require.NotNil(t, old)
			require.EqualValues(t, 24, r.safe[app.ID])
			if removed {
				r.apps = []*model.Application{&sibling}
				delete(r.safe, app.ID)
			} else {
				app.Enabled = false
			}
			_, err = o.observe(t.Context(), 29)
			require.NoError(t, err)
			require.NotContains(t, o.apps, app.ID, "wrap removes the whole inactive unit, not just its cache")
			id := app.ID
			if removed {
				registered := *app
				registered.ID = 3
				registered.Name += "-registered"
				id = registered.ID
				r.apps = append(r.apps, &registered)
			} else {
				app.Enabled = true
			}
			_, err = o.observe(t.Context(), 29)
			require.NoError(t, err)
			require.NotSame(t, old, o.apps[id])
			expected := uint64(24)
			if removed {
				expected = 0
			}
			require.Equal(t, []uint64{0, expected}, replay)
			require.Len(t, o.apps[id].pending, 1)
		})
	}
}

func TestIndependentClaimObserverWarningReportsChangedCause(t *testing.T) {
	o, _, _, app := observerFixture(t)
	var log bytes.Buffer
	o.logger = slog.New(slog.NewTextHandler(&log, nil))
	state := &claimObservation{}
	o.warn(app, state, claimWarningPending, "validation", "validation is pending")
	o.warn(app, state, claimWarningPending, "validation", "validation is pending")
	require.Equal(t, 1, strings.Count(log.String(), "Independent claim verification is incomplete"))
	o.warn(app, state, claimWarningPending, "problem", "local CLAIM_FORECLOSED prevents validation")
	require.Equal(t, 2, strings.Count(log.String(), "Independent claim verification is incomplete"))
}

func TestIndependentClaimObserverRemovalDuringStatusConfirmation(t *testing.T) {
	for _, afterWrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("after_write=%t", afterWrite), func(t *testing.T) {
			o, r, c, app := observerFixture(t)
			fact := observerFact(19, 25, common.HexToHash("0xbad"), common.HexToHash("0x22"))
			r.roots[19] = readyRoots(common.HexToHash("0x11"), fact.Outputs)
			reads := 0
			r.appHook = func(string) (*model.Application, error) {
				reads++
				if !afterWrite || reads == 2 {
					return nil, nil
				}
				stored := *app
				return &stored, nil
			}
			c.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
				return observerBatch(29, fact), nil
			}
			var captured bytes.Buffer
			o.logger = slog.New(slog.NewTextHandler(&captured, nil))
			require.NotPanics(t, func() { _, err := o.observe(t.Context(), 29); require.NoError(t, err) })
			require.NotContains(t, o.apps, app.ID)
			require.Zero(t, r.safe[app.ID], "removal is abandonment, not confirmed integrity processing")
			expectedWrites := 0
			if afterWrite {
				expectedWrites = 1
			}
			require.Equal(t, expectedWrites, r.statusCalls)
			require.NotContains(t, captured.String(), "Independent claim verification is incomplete",
				"the removed application must not receive a pending warning")
		})
	}
}

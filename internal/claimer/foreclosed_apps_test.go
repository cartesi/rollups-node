// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// foreclosedAppHelper builds a foreclosed Application instance, optionally
// with a PRT consensus type. ForecloseBlock is non-zero, mirroring what
// the evmreader's checkForForeclosure would have persisted.
// LastInputCheckBlock is parked at the foreclose block so callers that do
// not exercise the bootstrap-readiness guard skip past it; tests that
// exercise the guard override the field explicitly.
func foreclosedAppHelper(id int64, block uint64, consensus model.Consensus) *model.Application {
	txHash := common.HexToHash("0xdeadbeef")
	return &model.Application{
		ID:                   id,
		Name:                 "app",
		IApplicationAddress:  common.BigToAddress(common.Big1),
		ConsensusType:        consensus,
		Enabled:              true,
		Status:               model.ApplicationStatus_OK,
		ForecloseBlock:       block,
		ForecloseTransaction: &txHash,
		LastInputCheckBlock:  block,
	}
}

// TestListEnabledForeclosedNonPRTApps_UsesAuthorityQuorumFilter verifies the
// repository filter used by the Authority/Quorum drain path. PRT apps have
// their own post-foreclosure path, so the claimer asks the repository for only
// Authority and Quorum apps.
func TestListEnabledForeclosedNonPRTApps_UsesAuthorityQuorumFilter(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	auth := foreclosedAppHelper(1, 100, model.Consensus_Authority)
	quorum := foreclosedAppHelper(2, 200, model.Consensus_Quorum)

	// Match the exact filter the service issues so the test catches
	// regressions in either side. ForeclosureRecorded must be passed
	// as &true, Enabled as &true, and ConsensusTypes as Authority/Quorum.
	r.On("ListApplications",
		mock.Anything,
		mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Enabled != nil && *f.Enabled &&
				f.ForeclosureRecorded != nil && *f.ForeclosureRecorded &&
				assert.ElementsMatch(t,
					[]model.Consensus{model.Consensus_Authority, model.Consensus_Quorum},
					f.ConsensusTypes,
				) &&
				assert.ElementsMatch(t,
					[]model.ApplicationStatus{model.ApplicationStatus_OK},
					f.Statuses,
				)
		}),
		mock.Anything,
		mock.Anything,
	).Return([]*model.Application{auth, quorum}, 2, nil).Once()

	got, err := s.listEnabledForeclosedNonPRTApps()
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Contains(t, got, auth.ID)
	assert.Contains(t, got, quorum.ID)
}

// TestListEnabledForeclosedNonPRTApps_ExcludesTerminalStatuses verifies the
// selection boundary that keeps terminal foreclosed apps out of the claim
// drain. A DIVERGED or CORRUPTED app is terminal for claim work; EVM reader
// still observes it, but the claimer must not keep re-running drain checks
// every tick. The protection lives in the selecting filter, which restricts
// Statuses to OK, so terminal apps are never handed to processForeclosedApps.
func TestListEnabledForeclosedNonPRTApps_ExcludesTerminalStatuses(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	r.On("ListApplications",
		mock.Anything,
		mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return assert.ElementsMatch(t,
				[]model.ApplicationStatus{model.ApplicationStatus_OK},
				f.Statuses,
			)
		}),
		mock.Anything,
		mock.Anything,
	).Return([]*model.Application{}, 0, nil).Once()

	got, err := s.listEnabledForeclosedNonPRTApps()
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestProcessForeclosedApps_DefersWhenUnreconciled verifies that an app
// whose pre-foreclosure epochs have not all reached CLAIM_ACCEPTED or
// CLAIM_FORECLOSED stays in its current app status. The deferral branch must NOT issue an
// UpdateApplicationStatus call — transitioning before the advancer/validator
// finish would lose the last-known epoch outputs needed for any in-flight
// dispute; firing before claim reconciliation completes would leave the local
// DB final state divergent from the chain.
func TestProcessForeclosedApps_DefersWhenUnreconciled(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	app := foreclosedAppHelper(1, 100, model.Consensus_Authority)
	s.Context = context.Background()

	r.On("ForecloseUnacceptedEpochsAtOrAfterBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(0, nil).Once()
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()

	// No UpdateApplicationStatus expectation — if it fires, the mock
	// assertion fails the test because we registered no Setup for it.

	errs := s.processForeclosedApps(map[int64]*model.Application{app.ID: app})
	assert.Empty(t, errs, "deferral is not an error")
}

// TestProcessForeclosedApps_DrainCheckErrorsAppendAndContinue verifies the
// resilience contract of the drain pass: when a per-app drain-gate repository
// call errors, the error is collected and the loop moves on to the next app —
// one app's DB failure must not abort the whole pass. With two apps both
// failing the same gate, both errors are surfaced (proving the loop did not
// stop after the first).
func TestProcessForeclosedApps_DrainCheckErrorsAppendAndContinue(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)
	s.Context = context.Background()

	app1 := foreclosedAppHelper(1, 100, model.Consensus_Authority)
	app2 := foreclosedAppHelper(2, 100, model.Consensus_Authority)

	for _, app := range []*model.Application{app1, app2} {
		r.On("HasUndrainedEpochsBeforeBlock",
			mock.Anything, app.ID, app.ForecloseBlock,
		).Return(false, errors.New("db unavailable")).Once()
	}
	// The drain check runs first; its error makes the per-app branch `continue`
	// before terminalizing or reconciling. Neither
	// ForecloseUnacceptedEpochsAtOrAfterBlock nor HasUnreconciledClaimsBeforeBlock
	// is reached — no expectation registered for either.

	errs := s.processForeclosedApps(map[int64]*model.Application{app1.ID: app1, app2.ID: app2})
	assert.Len(t, errs, 2, "each app's drain error is appended; the pass does not abort early")
}

// TestProcessForeclosedApps_NoTransitionWhenDrained verifies that once both
// gates clear (bootstrap-readiness + drain reconciliation), the per-app
// branch is a no-op. No UpdateApplicationStatus call fires — the app stays
// enabled with status OK and foreclose_block set, and the
// post-foreclosure observation work (drive-prove discovery, withdrawal
// indexing) lives in evmreader. CORRUPTED is reserved for genuine corruption.
//
// The mock has no UpdateApplicationStatus expectation registered;
// testify/mock fails the test on an unexpected call, so any regression that
// re-introduces a terminal-state transition trips this test loudly.
func TestProcessForeclosedApps_NoTransitionWhenDrained(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	app := foreclosedAppHelper(1, 100, model.Consensus_Authority)
	s.Context = context.Background()

	r.On("ForecloseUnacceptedEpochsAtOrAfterBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(0, nil).Once()
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()

	// No UpdateApplicationStatus expectation — the assertion is by negation.

	errs := s.processForeclosedApps(map[int64]*model.Application{app.ID: app})
	assert.Empty(t, errs)
}

// TestProcessForeclosedApps_DefersWhenInputsUndrained verifies the drain-gate
// ordering that protects an input landing in the foreclose block. While any
// pre-foreclosure input is still undrained, the pass defers WITHOUT
// terminalizing. Terminalizing the straddling epoch first would flip it to
// CLAIM_FORECLOSED and strand its unprocessed same-block input (it would vanish
// from both this drain check and the manager's machine-drain gate, and the
// machine would be torn down before advancing it). The absent
// ForecloseUnacceptedEpochsAtOrAfterBlock expectation is the regression guard:
// testify/mock fails on the unexpected call if terminalization runs too early.
func TestProcessForeclosedApps_DefersWhenInputsUndrained(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	app := foreclosedAppHelper(1, 100, model.Consensus_Authority)
	s.Context = context.Background()

	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()
	// No ForecloseUnacceptedEpochsAtOrAfterBlock and no
	// HasUnreconciledClaimsBeforeBlock: an undrained input defers the whole pass
	// before terminalization and before claim reconciliation.

	errs := s.processForeclosedApps(map[int64]*model.Application{app.ID: app})
	assert.Empty(t, errs, "input-drain deferral is not an error")
}

// TestProcessForeclosedApps_TerminalizesUnacceptedOverlapAfterDrain verifies the
// other side of the gate: once the drain check clears (no undrained inputs), the
// straddling/after epochs that can never be accepted are terminalized to
// CLAIM_FORECLOSED, then reconciliation completes.
func TestProcessForeclosedApps_TerminalizesUnacceptedOverlapAfterDrain(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	app := foreclosedAppHelper(1, 100, model.Consensus_Authority)
	s.Context = context.Background()

	// Pin the sequence: the drain check MUST run before terminalization (else a
	// straddling-epoch input is stranded — the bug this ordering prevents), and
	// terminalization before the claim-reconciliation check.
	drain := r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	terminalize := r.On("ForecloseUnacceptedEpochsAtOrAfterBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(2, nil).Once()
	reconcile := r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	mock.InOrder(drain, terminalize, reconcile)

	errs := s.processForeclosedApps(map[int64]*model.Application{app.ID: app})
	assert.Empty(t, errs)
}

// TestProcessForeclosedApps_SkipsZeroForecloseBlock is a defensive check on
// the loop's "should have been filtered" guard. partitionForeclosedApps is
// the only intended source of input, but a caller bug or future refactor
// could feed an app with a zero ForecloseBlock here; the loop must skip it
// silently rather than treat block 0 as a real foreclosure marker.
func TestProcessForeclosedApps_SkipsZeroForecloseBlock(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	app := &model.Application{ID: 99, ConsensusType: model.Consensus_Authority}
	s.Context = context.Background()

	// No mock expectations — the loop must skip before any repo call.
	errs := s.processForeclosedApps(map[int64]*model.Application{app.ID: app})
	assert.Empty(t, errs)
}

// TestProcessForeclosedApps_DefersWhenStillBackfilling verifies the
// bootstrap-readiness guard. When a freshly registered Authority/Quorum
// app encounters an already-foreclosed contract, evmreader sets
// ForecloseBlock before checkForNewInputs has had time to ingest the
// historical inputs. If the drain check fires inside that window, the gate
// sees an empty epoch table and incorrectly returns false, making the app look
// drained before any pre-foreclosure epoch is observed locally. The guard must
// defer the drain check until LastInputCheckBlock >= ForecloseBlock.
//
// Neither HasUndrainedEpochsBeforeBlock, HasUnreconciledClaimsBeforeBlock nor
// UpdateApplicationStatus has an `.On` registered; testify/mock panics on an
// unexpected call, so any reach attempt fails the test loudly.
func TestProcessForeclosedApps_DefersWhenStillBackfilling(t *testing.T) {
	s, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	app := foreclosedAppHelper(1, 100, model.Consensus_Authority)
	app.LastInputCheckBlock = 50 // scanner well below the foreclose block
	s.Context = context.Background()

	errs := s.processForeclosedApps(map[int64]*model.Application{app.ID: app})
	assert.Empty(t, errs, "bootstrap deferral is not an error")
}

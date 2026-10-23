// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

type recoveryQueryPort struct {
	app                      int64
	observed, through, limit uint64
	after                    *uint64
}
type recoveryRepositoryPort struct {
	*deliveryRepoFake
	queries     []recoveryQueryPort
	err         error
	beforeQuery func()
}

func (r *recoveryRepositoryPort) ListRecoverableRootBonds(
	ctx context.Context, app int64, claimer common.Address, observed uint64, after *uint64, through, limit uint64,
) ([]repository.RootBondRecoveryCandidate, error) {
	r.queries = append(r.queries, recoveryQueryPort{app, observed, through, limit, after})
	if r.beforeQuery != nil {
		r.beforeQuery()
	}
	if r.err != nil {
		return nil, r.err
	}
	return r.deliveryRepoFake.ListRecoverableRootBonds(ctx, app, claimer, observed, after, through, limit)
}
func bondCandidate(epoch uint64) repository.RootBondRecoveryCandidate {
	return repository.RootBondRecoveryCandidate{EpochIndex: epoch, Tournament: common.BigToAddress(new(big.Int).SetUint64(epoch + 1))}
}
func recoveryCandidatePort(t *testing.T, f *deliveryFixture, candidate repository.RootBondRecoveryCandidate,
	state model.BondDisposition, readErr error) *recoveryPortAdapter {
	t.Helper()
	adapter := &recoveryPortAdapter{recovery: canonicalBondRecovery(state, f.repo.signer, 1), err: readErr}
	f.factory.On("CreateTournamentAdapter", candidate.Tournament).Return(adapter, nil).Maybe()
	f.backend.code[f.repo.signer] = nil
	return adapter
}
func planRecoveryPagePort(t *testing.T, f *deliveryFixture, facts uint64) (*plannedAction, error) {
	t.Helper()
	return f.d.planAction(t.Context(), f.app, recoverBond, f.block, txsender.BlockRef{Number: facts})
}

func TestPublishedRecoveryRediscoversAcceptedEpochWithoutLocalCommitment(t *testing.T) {
	for _, foreclosed := range []bool{false, true} {
		f := newRecoveryPortFixture(t)
		f.epoch.Status = model.EpochStatus_ClaimAccepted
		if foreclosed {
			f.app.ForecloseBlock = 90
			f.app.LastEpochCheckBlock = 1
			f.app.LastInputCheckBlock = 1
		}
		call, err := f.plan(t.Context())
		require.NoError(t, err)
		require.NotNil(t, call)
		f.repo.AssertNotCalled(t, "GetCommitment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		f.factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
	}
}

func TestPublishedRecoveryBoundsPagesAndWrapsFiniteSweep(t *testing.T) {
	f := newDeliveryFixture(t)
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).Return(new(uint64(16)), nil)
	for epoch := uint64(0); epoch <= 16; epoch++ {
		candidate := bondCandidate(epoch)
		f.repo.recoveries = append(f.repo.recoveries, candidate)
		recoveryCandidatePort(t, f, candidate, model.BondDispositionRecovered, nil)
	}
	call, err := planRecoveryPagePort(t, f, 100)
	require.NoError(t, err)
	require.Nil(t, call)
	require.Equal(t, uint64(15), *f.d.rootBondRecoveryScans[f.app.ID].AfterEpoch)
	require.Len(t, f.factory.Calls, 16, "one page is bounded")
	f.app.LastTournamentCheckBlock = 110
	call, err = planRecoveryPagePort(t, f, 110)
	require.NoError(t, err)
	require.Nil(t, call)
	require.Empty(t, f.d.rootBondRecoveryScans)
	require.Len(t, f.factory.Calls, 17, "a new observation does not extend the captured sweep")
	call, err = planRecoveryPagePort(t, f, 110)
	require.NoError(t, err)
	require.Nil(t, call)
	require.Len(t, f.factory.Calls, 33, "a later finite sweep can revisit previous live verdicts")
}

func TestPublishedRecoverySkipsSuppressedPageWithoutStarvingLaterRoot(t *testing.T) {
	f := newDeliveryFixture(t)
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).Return(new(uint64(16)), nil)
	for epoch := uint64(0); epoch <= 16; epoch++ {
		candidate := bondCandidate(epoch)
		f.repo.recoveries = append(f.repo.recoveries, candidate)
		failure := errors.New("unsupported recipient")
		if epoch == 16 {
			failure = nil
		}
		recoveryCandidatePort(t, f, candidate, model.BondDispositionRecoverable, failure)
	}
	call, err := planRecoveryPagePort(t, f, 100)
	require.NoError(t, err)
	require.Nil(t, call)
	call, err = planRecoveryPagePort(t, f, 100)
	require.NoError(t, err)
	require.NotNil(t, call)
	require.Equal(t, uint64(16), call.Key.Epoch)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
}

func TestPublishedRecoveryTemporaryPreflightFailureDefersOnlyThatRoot(t *testing.T) {
	f := newDeliveryFixture(t)
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).Return(new(uint64(1)), nil)
	first, second := bondCandidate(0), bondCandidate(1)
	f.repo.recoveries = []repository.RootBondRecoveryCandidate{first, second}
	adapter := recoveryCandidatePort(t, f, first, model.BondDispositionRecoverable, errors.New("provider unavailable"))
	recoveryCandidatePort(t, f, second, model.BondDispositionRecoverable, nil)
	call, err := planRecoveryPagePort(t, f, 100)
	require.NoError(t, err)
	require.NotNil(t, call)
	require.Equal(t, second.Tournament, call.To, "one failed live read cannot starve the next root")
	adapter.err = nil
	call, err = planRecoveryPagePort(t, f, 100)
	require.NoError(t, err)
	require.Nil(t, call, "finish the finite sweep before retrying older roots")
	call, err = planRecoveryPagePort(t, f, 100)
	require.NoError(t, err)
	require.NotNil(t, call)
	require.Equal(t, first.Tournament, call.To)
}

func TestPublishedRecoveryUsesLowerObservationBoundAndRetriesQueryErrors(t *testing.T) {
	f := newRecoveryPortFixture(t)
	repository := &recoveryRepositoryPort{deliveryRepoFake: f.repo, err: errors.New("database unavailable")}
	f.d.repository = repository
	call, err := f.d.planAction(t.Context(), f.app, recoverBond, f.block, txsender.BlockRef{Number: 80})
	require.ErrorIs(t, err, repository.err)
	require.Nil(t, call)
	require.Nil(t, f.d.rootBondRecoveryScans[f.app.ID].AfterEpoch)
	repository.err = nil
	call, err = f.d.planAction(t.Context(), f.app, recoverBond, f.block, txsender.BlockRef{Number: 70})
	require.NoError(t, err)
	require.NotNil(t, call)
	require.Equal(t, uint64(80), repository.queries[0].observed)
	require.Equal(t, uint64(70), repository.queries[1].observed)
	require.Equal(t, uint64(16), repository.queries[1].limit)
}

func TestPublishedRecoveryEmptyOrUnavailableSweepBoundary(t *testing.T) {
	for _, failure := range []error{nil, errors.New("epoch read failed")} {
		f := newDeliveryFixture(t)
		f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).Return((*uint64)(nil), failure).Once()
		call, err := planRecoveryPagePort(t, f, 100)
		if failure != nil {
			require.ErrorIs(t, err, failure)
		} else {
			require.NoError(t, err)
		}
		require.Nil(t, call)
		require.Empty(t, f.d.rootBondRecoveryScans)
		require.Empty(t, f.factory.Calls)
	}
}

func TestPublishedRecoverySuppressionIsApplicationScoped(t *testing.T) {
	f := newRecoveryPortFixture(t)
	f.d.candidateCooldown[actionID{ApplicationID: f.app.ID + 1, Kind: recoverBond}] = time.Now().Add(time.Hour)
	call, err := f.plan(t.Context())
	require.NoError(t, err)
	require.NotNil(t, call)
	require.Equal(t, f.app.ID, call.Key.ApplicationID)
}

func TestPublishedRecoverySkipsDiscoveryWithoutPublishedEvidence(t *testing.T) {
	for _, bound := range []struct{ facts, cursor uint64 }{{0, 100}} {
		f := newDeliveryFixture(t)
		f.app.LastTournamentCheckBlock = bound.cursor
		call, err := planRecoveryPagePort(t, f, bound.facts)
		require.NoError(t, err)
		require.Nil(t, call)
		require.Empty(t, f.factory.Calls)
		f.repo.AssertNotCalled(t, "GetLastNonOpenEpochIndex", mock.Anything, mock.Anything)
	}
}

func TestPublishedRecoveryHealthAndSubmissionGates(t *testing.T) {
	for _, status := range model.ApplicationStatusAllValues {
		for _, enabled := range []bool{false, true} {
			t.Run(status.String()+"/enabled="+fmt.Sprint(enabled), func(t *testing.T) {
				f := newRecoveryPortFixture(t)
				f.app.Status, f.app.Enabled = status, enabled
				call, err := f.plan(t.Context())
				require.NoError(t, err)
				if enabled {
					require.NotNil(t, call, "refund eligibility is independent of application health")
				} else {
					require.Nil(t, call, "disable stops recovery under every application status")
					f.repo.AssertNotCalled(t, "GetLastNonOpenEpochIndex", mock.Anything, mock.Anything)
					f.factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
				}
				require.Equal(t, status, f.app.Status)
			})
		}
	}
}

func TestRecoveryUsesItsCommittedRowsWithoutApplicationCoverageBarrier(t *testing.T) {
	f := newRecoveryPortFixture(t)
	f.app.LastTournamentCheckBlock = 0
	call, err := f.plan(t.Context())
	require.NoError(t, err)
	require.NotNil(t, call)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
)

const recoveryObservationBlock uint64 = 100

func storeRecoveryRoot(
	t *testing.T, r *PostgresRepository, app *model.Application, index uint64,
	claimer common.Address, disposition model.BondDisposition, observedBlock uint64,
) (*model.Epoch, repository.RootBondRecoveryCandidate) {
	t.Helper()
	root := repotest.NewTournamentBuilder(app.ID).WithEpochIndex(index).Build()
	epoch := repotest.NewEpochBuilder(app.ID).WithIndex(index).WithStatus(model.EpochStatus_Closed).Build()
	epoch.TournamentAddress = &root.Address
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
		map[*model.Epoch][]*model.Input{epoch: {}}, observedBlock))
	root.Snapshot.AsOfBlock = observedBlock
	root.Snapshot.BondRecovery.Disposition = disposition
	if disposition != model.BondDispositionTournamentRunning {
		root.Snapshot.AcceptsJoins = false
		root.Snapshot.FinishedAtBlock = observedBlock
		root.Snapshot.Standing = model.TournamentStandingRootFailed
		if disposition != model.BondDispositionNoWinner {
			root.Snapshot.Standing = model.TournamentStandingRootWinner
			root.Snapshot.Candidate = new(repotest.UniqueHash())
			root.Snapshot.WinnerCommitment = root.Snapshot.Candidate
			root.Snapshot.FinalStateHash = new(repotest.UniqueHash())
		}
	}
	if disposition == model.BondDispositionRecoverable {
		root.Snapshot.BondRecovery.Claimer = &claimer
		root.Snapshot.BondRecovery.Payment = new(model.Uint256)
	}
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID,
		[]*repository.TournamentEventBatch{{Tournament: root}}, observedBlock))
	return epoch, repository.RootBondRecoveryCandidate{EpochIndex: index, Tournament: root.Address}
}

func TestListRecoverableRootBondsFiltersPublishedOwnedRoots(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
	otherApp := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
	owner, otherOwner := repotest.UniqueAddress(), repotest.UniqueAddress()
	want := make([]repository.RootBondRecoveryCandidate, 0, 3)
	for index, status := range []model.EpochStatus{
		model.EpochStatus_ClaimAccepted, model.EpochStatus_Closed, model.EpochStatus_ClaimForeclosed,
	} {
		epoch, candidate := storeRecoveryRoot(t, r, app, uint64(index), owner,
			model.BondDispositionRecoverable, recoveryObservationBlock)
		if status != model.EpochStatus_Closed {
			repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epoch, status)
		}
		stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
		require.NoError(t, err)
		require.Equal(t, status, stored.Status)
		if status != model.EpochStatus_ClaimAccepted {
			require.Nil(t, stored.Commitment, "discovery must not require a local commitment")
		}
		want = append(want, candidate)
	}
	_, foreignOwned := storeRecoveryRoot(t, r, app, 3, otherOwner,
		model.BondDispositionRecoverable, recoveryObservationBlock)
	_, otherApplication := storeRecoveryRoot(t, r, otherApp, 0, owner,
		model.BondDispositionRecoverable, recoveryObservationBlock)
	for index, disposition := range []model.BondDisposition{
		model.BondDispositionTournamentRunning, model.BondDispositionNoWinner, model.BondDispositionRecovered,
	} {
		storeRecoveryRoot(t, r, app, uint64(index)+4, owner, disposition, recoveryObservationBlock)
	}
	_, newer := storeRecoveryRoot(t, r, app, 7, owner, model.BondDispositionRecoverable, recoveryObservationBlock+1)
	for _, test := range []struct {
		name     string
		appID    int64
		claimer  common.Address
		observed uint64
		want     []repository.RootBondRecoveryCandidate
	}{
		{"owned published roots", app.ID, owner, recoveryObservationBlock, want},
		{"other signer", app.ID, otherOwner, recoveryObservationBlock, []repository.RootBondRecoveryCandidate{foreignOwned}},
		{"other application", otherApp.ID, owner, recoveryObservationBlock, []repository.RootBondRecoveryCandidate{otherApplication}},
		{"unknown application", otherApp.ID + 1, owner, recoveryObservationBlock, nil},
		{"zero publication cursor", app.ID, owner, 0, nil},
		{"older observation head", app.ID, owner, recoveryObservationBlock - 1, nil},
		{"publication catches up", app.ID, owner, recoveryObservationBlock + 1,
			append(append([]repository.RootBondRecoveryCandidate{}, want...), newer)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := r.ListRecoverableRootBonds(t.Context(), test.appID, test.claimer, test.observed, nil, math.MaxUint64, 10)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
	var joins int
	require.NoError(t, r.db.QueryRow(t.Context(), "SELECT count(*) FROM commitments").Scan(&joins))
	require.Zero(t, joins, "candidates do not depend on recorded joins")
}

func TestListRecoverableRootBondsExcludesInnerTournament(t *testing.T) {
	r, app, root, match := newTournamentIntegrityFixture(t)
	require.NoError(t, r.CreateMatch(t.Context(), app.Name, match))
	owner := repotest.UniqueAddress()
	child := repotest.NewTournamentBuilder(app.ID).WithLevel(1).WithParent(root.Address, match.IDHash).Build()
	child.Snapshot.Standing = model.TournamentStandingInnerEliminableWinnerExpired
	child.Snapshot.AcceptsJoins = false
	child.Snapshot.FinishedAtBlock = recoveryObservationBlock
	child.Snapshot.InnerResult.Disposition = model.InnerTournamentEliminable
	child.Snapshot.BondRecovery = model.TournamentBondRecovery{
		Disposition: model.BondDispositionRecoverable, Claimer: &owner, Payment: new(model.Uint256),
	}
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID,
		[]*repository.TournamentEventBatch{{Tournament: child}}, recoveryObservationBlock))
	got, err := r.ListRecoverableRootBonds(t.Context(), app.ID, owner, recoveryObservationBlock, nil, math.MaxUint64, 10)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestListRecoverableRootBondsKeysetPagination(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
	owner := repotest.UniqueAddress()
	var want []repository.RootBondRecoveryCandidate
	// Reverse insertion order to prove that pagination uses epoch order.
	for _, index := range []uint64{math.MaxUint64, 10, 5, 2, 0} {
		_, candidate := storeRecoveryRoot(t, r, app, index, owner, model.BondDispositionRecoverable, recoveryObservationBlock)
		want = append([]repository.RootBondRecoveryCandidate{candidate}, want...)
	}
	for _, test := range []struct {
		name    string
		after   *uint64
		through uint64
		limit   uint64
		want    []repository.RootBondRecoveryCandidate
	}{
		{"first page includes zero", nil, 10, 2, want[:2]},
		{"next page excludes cursor", new(uint64(2)), 10, 2, want[2:4]},
		{"zero upper bound", nil, 0, 2, want[:1]},
		{"inclusive upper bound", nil, 5, 10, want[:3]},
		{"cursor need not be an existing epoch", new(uint64(3)), 5, 2, want[2:3]},
		{"exhausted sweep", new(uint64(10)), 10, 2, nil},
		{"cursor above sweep", new(uint64(11)), 10, 2, nil},
		{"full uint64 epoch range", new(uint64(math.MaxInt64)), math.MaxUint64, 2, want[4:]},
		{"max epoch cursor does not overflow", new(uint64(math.MaxUint64)), math.MaxUint64, 2, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := r.ListRecoverableRootBonds(t.Context(), app.ID, owner,
				recoveryObservationBlock, test.after, test.through, test.limit)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestListRecoverableRootBondsRejectsUnboundedLimit(t *testing.T) {
	r := &PostgresRepository{}
	for _, limit := range []uint64{0, uint64(math.MaxInt64) + 1, math.MaxUint64} {
		_, err := r.ListRecoverableRootBonds(t.Context(), 1, common.Address{}, 100, nil, 0, limit)
		require.ErrorContains(t, err, "root bond recovery limit must be between")
	}
}

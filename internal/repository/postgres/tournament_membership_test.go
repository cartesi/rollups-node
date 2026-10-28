// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"strconv"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

const (
	intervalCommitmentKind = "commitment"
	intervalLeafSealKind   = "leaf seal"
)

func TestTournamentIntervalRejectsDuplicateParticipantIdentities(t *testing.T) {
	for _, kind := range []string{intervalCommitmentKind, "match"} {
		for _, boundary := range []int{1, tournamentRowsPerStatement + 1} {
			t.Run(kind+"/"+strconv.Itoa(boundary), func(t *testing.T) {
				r, app, batch := newRootEventInterval(t)
				for range boundary {
					if kind == intervalCommitmentKind {
						addIntervalCommitment(batch)
					} else {
						addIntervalMatch(batch)
					}
				}
				if kind == intervalCommitmentKind {
					duplicate := *batch.Commitments[0]
					batch.Commitments = append(batch.Commitments, &duplicate)
				} else {
					duplicate := *batch.Matches[0]
					batch.Matches = append(batch.Matches, &duplicate)
				}
				err := r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch})
				require.ErrorContains(t, err, "duplicate "+kind)
				stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
				require.NoError(t, err)
				require.Nil(t, stored, "duplicates are rejected independently of their insert chunk")
				storedApp, err := r.GetApplication(t.Context(), app.Name)
				require.NoError(t, err)
				require.Zero(t, storedApp.LastTournamentCheckBlock)
			})
		}
	}
}

func TestTournamentIntervalHistoricalMembershipRequiresExactScope(t *testing.T) {
	r, app, batch := newRootEventInterval(t)
	addIntervalMatch(batch)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
	tx, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	for _, kind := range []string{intervalCommitmentKind, "match"} {
		for _, scope := range []string{"application", "epoch", "tournament"} {
			t.Run(kind+"/"+scope, func(t *testing.T) {
				root := *batch.Tournament
				id := app.ID
				switch scope {
				case "application":
					id++
				case "epoch":
					root.EpochIndex++
				case "tournament":
					root.Address = repotest.UniqueAddress()
				}
				check := &repository.TournamentEventBatch{Tournament: &root}
				if kind == intervalCommitmentKind {
					check.Commitments = batch.Commitments
				} else {
					check.Matches = batch.Matches
				}
				err := validatePriorTournamentParticipants(t.Context(), tx, id, check, 101)
				require.ErrorContains(t, err, "new "+kind+" precedes tournament interval")
			})
		}
	}
}

func TestTournamentIntervalHistoricalParticipantsDenseGenericPlan(t *testing.T) {
	r, app, batch := newRootEventInterval(t)
	// A complete, distinct prefix exercises the generic-plan path that used to
	// scan all stored keys once for every scalar IN parameter.
	const participants = 32000
	for range participants {
		addIntervalCommitment(batch)
	}
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
	tx, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	// This PostgreSQL planner setting has no Jet statement form. It is local
	// to this test transaction and changes no application data or saved config.
	_, err = tx.Exec(t.Context(), "SET LOCAL plan_cache_mode = force_generic_plan")
	require.NoError(t, err)
	require.NoError(t, validatePriorTournamentParticipants(t.Context(), tx, app.ID, batch, 101))
	absent := *batch.Commitments[0]
	absent.Commitment = repotest.UniqueHash()
	batch.Commitments[len(batch.Commitments)-1] = &absent
	err = validatePriorTournamentParticipants(t.Context(), tx, app.ID, batch, 101)
	require.ErrorContains(t, err, "new commitment precedes tournament interval", "array membership must include every key")
}

func setIntervalOptionalEvent(match *model.Match, kind string, block uint64) {
	if kind == intervalLeafSealKind {
		match.LeafSeal = &model.LeafMatchSeal{BlockNumber: block, TxHash: repotest.UniqueHash()}
		return
	}
	match.DeletionReason = model.MatchDeletionReason_TIMEOUT
	match.DeletionBlockNumber = block
	match.DeletionTxHash, match.DeletionLogIndex = new(repotest.UniqueHash()), new(uint64(0))
	match.Snapshot = model.MatchSnapshot{AsOfBlock: match.Snapshot.AsOfBlock, Phase: model.MatchPhaseUninitialized,
		TimeoutOutcome: model.MatchTimeoutNone}
}

func TestTournamentIntervalRejectsFirstOptionalEventBelowCommittedCursor(t *testing.T) {
	for _, kind := range []string{intervalLeafSealKind, "match deletion"} {
		t.Run(kind, func(t *testing.T) {
			r, app, batch := newRootEventInterval(t)
			match := addIntervalMatch(batch)
			require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
			before, err := r.GetTournamentEventCounts(t.Context(), app.ID, 0, batch.Tournament.Address, 100)
			require.NoError(t, err)
			batch.Tournament.Snapshot.AsOfBlock = 110
			match.Snapshot.AsOfBlock = 110
			batch.Commitments = nil
			setIntervalOptionalEvent(match, kind, 95)
			err = r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(100)), []*repository.TournamentEventBatch{batch})
			require.ErrorContains(t, err, "new "+kind+" precedes tournament interval")
			after, err := r.GetTournamentEventCounts(t.Context(), app.ID, 0, batch.Tournament.Address, 100)
			require.NoError(t, err)
			require.Equal(t, before, after, "a later interval cannot add evidence beneath a committed cursor")
			stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
			require.NoError(t, err)
			require.Equal(t, uint64(100), stored.Snapshot.AsOfBlock)
			// The first event at the next inclusive block is valid.
			setIntervalOptionalEvent(match, kind, 101)
			require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(100)),
				[]*repository.TournamentEventBatch{batch}))
			// A later participant refresh may repeat exactly that saved event.
			batch.Tournament.Snapshot.AsOfBlock, match.Snapshot.AsOfBlock = 120, 120
			require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(110)),
				[]*repository.TournamentEventBatch{batch}))
		})
	}
}

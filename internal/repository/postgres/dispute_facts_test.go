// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"math"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	disputeApplicationScope = "application scope"
	disputeEpochScope       = "epoch scope"
	disputeTournamentScope  = "tournament scope"
)

func TestDisputeFactsProductionIntervalStoresEveryNewColumn(t *testing.T) {
	r, app, batches := newTournamentInterval(t)
	root, child := batches[0].Tournament, batches[1].Tournament
	root.BondValue, child.BondValue = model.Uint256{19}, model.Uint256{23}
	child.ContestedFinalStateOne = new(common.Hash{})
	child.ContestedFinalStateTwo = new(common.HexToHash("0x31"))
	batches[0].Matches[0].ResponderDeadline = 17
	match := batches[1].Matches[0]
	match.ResponderDeadline = 19
	seal := match.LeafSeal
	seal.AgreeState, seal.FinalStateOne, seal.FinalStateTwo = common.Hash{}, common.HexToHash("0x41"), common.HexToHash("0x42")
	seal.DivergencePosition = model.Uint256{7}
	standing := &model.StandingChanged{
		ApplicationID: app.ID, EpochIndex: child.EpochIndex, TournamentAddress: child.Address,
		Dangling: new(batches[1].Commitments[0].Commitment), ParentCommitment: new(batches[0].Matches[0].CommitmentOne),
		ResultAt: 80, WinnerExpiresAt: 150, BlockNumber: 70, TxHash: common.HexToHash("0x51"), LogIndex: 3,
	}
	batches[1].StandingChanges = []*model.StandingChanged{standing}
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches))
	var rootCreationFields int
	require.NoError(t, r.db.QueryRow(t.Context(), `SELECT num_nonnulls(creation_block_number,creation_tx_hash,creation_log_index)
  FROM tournaments WHERE address=$1`, root.Address.Bytes()).Scan(&rootCreationFields))
	require.Zero(t, rootCreationFields, "root creation provenance must be physically absent")

	// Literal SQL is independent of the repository column lists and scanners.
	var bond, one, two string
	var creationBlock uint64
	require.NoError(t, r.db.QueryRow(t.Context(), `SELECT bond_value::text,
        encode(contested_final_state_one,'hex'),encode(contested_final_state_two,'hex'),creation_block_number
        FROM tournaments WHERE address=$1`, child.Address.Bytes()).Scan(&bond, &one, &two, &creationBlock))
	require.Equal(t, "23", bond)
	require.Equal(t, common.Hash{}.Hex()[2:], one)
	require.Equal(t, common.HexToHash("0x31").Hex()[2:], two)
	require.Equal(t, uint64(50), creationBlock)
	var responder, deadlineOne, deadlineTwo, eliminable uint64
	var agree, finalOne, finalTwo, position string
	require.NoError(t, r.db.QueryRow(t.Context(), `SELECT responder_deadline,seal_deadline_one,seal_deadline_two,
        seal_eliminable_at,encode(seal_agree_state,'hex'),encode(seal_final_state_one,'hex'),
        encode(seal_final_state_two,'hex'),seal_divergence_position::text FROM matches WHERE id_hash=$1`,
		match.IDHash.Bytes()).Scan(&responder, &deadlineOne, &deadlineTwo, &eliminable, &agree, &finalOne, &finalTwo, &position))
	require.Equal(t, uint64(19), responder)
	require.Equal(t, uint64(85), deadlineOne)
	require.Equal(t, uint64(90), deadlineTwo)
	require.Equal(t, uint64(90), eliminable)
	require.Equal(t, common.Hash{}.Hex()[2:], agree)
	require.Equal(t, common.HexToHash("0x41").Hex()[2:], finalOne)
	require.Equal(t, common.HexToHash("0x42").Hex()[2:], finalTwo)
	require.Equal(t, "7", position)
	for index, expected := range []struct{ height, deadline uint64 }{{2, 84}, {1, 85}} {
		advance := batches[1].MatchAdvances[index]
		var height, deadline uint64
		require.NoError(t, r.db.QueryRow(t.Context(), `SELECT current_height,responder_deadline
            FROM match_advances WHERE tx_hash=$1 AND log_index=$2`, advance.TxHash.Bytes(), advance.LogIndex).
			Scan(&height, &deadline))
		require.Equal(t, expected.height, height)
		require.Equal(t, expected.deadline, deadline)
	}
	var count, result, expiry, block, log uint64
	var dangling, parent, transaction string
	require.NoError(t, r.db.QueryRow(t.Context(), `SELECT match_count,result_at,winner_expires_at,block_number,log_index,
        encode(dangling,'hex'),encode(parent_commitment,'hex'),encode(tx_hash,'hex') FROM standing_changes`).
		Scan(&count, &result, &expiry, &block, &log, &dangling, &parent, &transaction))
	require.Zero(t, count)
	require.Equal(t, uint64(80), result)
	require.Equal(t, uint64(150), expiry)
	require.Equal(t, uint64(70), block)
	require.Equal(t, uint64(3), log)
	require.Equal(t, standing.Dangling.Hex()[2:], dangling)
	require.Equal(t, standing.ParentCommitment.Hex()[2:], parent)
	require.Equal(t, common.HexToHash("0x51").Hex()[2:], transaction)

	stored, err := r.GetMatch(t.Context(), app.Name, child.EpochIndex, child.Address.Hex(), match.IDHash.Hex())
	require.NoError(t, err)
	require.Equal(t, seal, stored.LeafSeal, "the deleted view must retain every historical seal field")
	storedChild, err := r.GetTournament(t.Context(), app.Name, child.Address.Hex())
	require.NoError(t, err)
	require.Equal(t, child.BondValue, storedChild.BondValue)
	require.Equal(t, child.ContestedFinalStateOne, storedChild.ContestedFinalStateOne)
	require.Equal(t, child.ContestedFinalStateTwo, storedChild.ContestedFinalStateTwo)
	counts, err := r.GetTournamentEventCounts(t.Context(), app.ID, child.EpochIndex, child.Address, 69)
	require.NoError(t, err)
	require.Zero(t, counts.StandingChanged)
	counts, err = r.GetTournamentEventCounts(t.Context(), app.ID, child.EpochIndex, child.Address, 70)
	require.NoError(t, err)
	require.Equal(t, uint64(1), counts.StandingChanged)
}

func TestStandingChangeReadsLiteralRowsAndExactScope(t *testing.T) {
	r, app, batch := newRootEventInterval(t)
	commitment := addIntervalCommitment(batch)
	commitment.Commitment = common.Hash{}
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
	// A stored-count query must not invent standing events from participant counts.
	counts, err := r.GetTournamentEventCounts(t.Context(), app.ID, 0, batch.Tournament.Address, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(1), counts.CommitmentJoined)
	require.Zero(t, counts.StandingChanged)
	transaction := common.HexToHash("0x44")
	_, err = r.db.Exec(t.Context(), `INSERT INTO standing_changes(application_id,epoch_index,tournament_address,
        match_count,dangling,parent_commitment,result_at,winner_expires_at,block_number,tx_hash,log_index)
        VALUES($1,0,$2,0,$3,$4,120,140,100,$5,2)`, app.ID, batch.Tournament.Address.Bytes(),
		common.Hash{}.Bytes(), common.HexToHash("0x33").Bytes(), transaction.Bytes())
	require.NoError(t, err)
	value, err := r.GetStandingChange(t.Context(), app.Name, 0, batch.Tournament.Address.Hex(), transaction, 2)
	require.NoError(t, err)
	require.NotNil(t, value)
	require.Equal(t, app.ID, value.ApplicationID)
	require.Equal(t, uint64(0), value.EpochIndex)
	require.Equal(t, batch.Tournament.Address, value.TournamentAddress)
	require.Equal(t, model.Uint256{}, value.MatchCount)
	require.Equal(t, new(common.Hash{}), value.Dangling)
	require.Equal(t, new(common.HexToHash("0x33")), value.ParentCommitment)
	require.Equal(t, uint64(120), value.ResultAt)
	require.Equal(t, uint64(140), value.WinnerExpiresAt)
	require.Equal(t, uint64(100), value.BlockNumber)
	require.Equal(t, transaction, value.TxHash)
	require.Equal(t, uint64(2), value.LogIndex)
	require.False(t, value.CreatedAt.IsZero())
	require.Equal(t, value.CreatedAt, value.UpdatedAt)
	for _, scope := range []struct {
		name, application, tournament string
		epoch, log                    uint64
		tx                            common.Hash
	}{
		{disputeApplicationScope, "missing", batch.Tournament.Address.Hex(), 0, 2, transaction},
		{disputeEpochScope, app.Name, batch.Tournament.Address.Hex(), 1, 2, transaction},
		{disputeTournamentScope, app.Name, repotest.UniqueAddress().Hex(), 0, 2, transaction},
		{"transaction", app.Name, batch.Tournament.Address.Hex(), 0, 2, common.Hash{}},
		{"log", app.Name, batch.Tournament.Address.Hex(), 0, 3, transaction},
	} {
		t.Run(scope.name, func(t *testing.T) {
			got, err := r.GetStandingChange(t.Context(), scope.application, scope.epoch, scope.tournament, scope.tx, scope.log)
			require.NoError(t, err)
			require.Nil(t, got)
		})
	}
	// Tied block/log rows require the final transaction-hash ordering key.
	for _, row := range []struct {
		tx         string
		block, log uint64
	}{{"0x11", 100, 2}, {"0x22", 100, 1}, {"0x66", 99, 9}} {
		_, err = r.db.Exec(t.Context(), `INSERT INTO standing_changes(application_id,epoch_index,tournament_address,
            match_count,result_at,winner_expires_at,block_number,tx_hash,log_index)
            VALUES($1,0,$2,18446744073709551616,0,0,$3,$4,$5)`, app.ID, batch.Tournament.Address.Bytes(),
			row.block, common.HexToHash(row.tx).Bytes(), row.log)
		require.NoError(t, err)
	}
	for _, descending := range []bool{false, true} {
		values, total, err := r.ListStandingChanges(t.Context(), app.IApplicationAddress.Hex(), 0,
			batch.Tournament.Address.Hex(), repository.Pagination{}, descending)
		require.NoError(t, err)
		require.Equal(t, uint64(4), total)
		want := []common.Hash{common.HexToHash("0x66"), common.HexToHash("0x22"), common.HexToHash("0x11"), transaction}
		if descending {
			for i, j := 0, len(want)-1; i < j; i, j = i+1, j-1 {
				want[i], want[j] = want[j], want[i]
			}
		}
		require.Len(t, values, len(want))
		for i := range want {
			require.Equal(t, want[i], values[i].TxHash)
		}
		paged, total, err := r.ListStandingChanges(t.Context(), app.Name, 0,
			batch.Tournament.Address.Hex(), repository.Pagination{Limit: 2, Offset: 1}, descending)
		require.NoError(t, err)
		require.Equal(t, uint64(4), total)
		require.Equal(t, values[1:3], paged)
	}
	values, total, err := r.ListStandingChanges(t.Context(), app.Name, 1, batch.Tournament.Address.Hex(), repository.Pagination{}, false)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, values)
	for _, pagination := range []repository.Pagination{{Limit: math.MaxUint64}, {Offset: math.MaxUint64}} {
		_, _, err = r.ListStandingChanges(t.Context(), app.Name, 0, batch.Tournament.Address.Hex(), pagination, false)
		require.ErrorContains(t, err, "pagination")
	}
}

func TestStandingChangesProductionChunksReplayAndRollback(t *testing.T) {
	r, app, batch := newRootEventInterval(t)
	addIntervalMatch(batch)
	batch.Tournament.Snapshot.Standing = model.TournamentStandingMatchesActive
	for range tournamentRowsPerStatement {
		batch.StandingChanges = append(batch.StandingChanges, &model.StandingChanged{
			ApplicationID: app.ID, TournamentAddress: batch.Tournament.Address, MatchCount: model.Uint256{1},
			BlockNumber: 100, TxHash: repotest.UniqueHash(),
		})
	}
	exact := *batch.StandingChanges[0]
	batch.StandingChanges = append(batch.StandingChanges, &exact)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
	first, err := r.GetStandingChange(t.Context(), app.Name, 0, batch.Tournament.Address.Hex(), exact.TxHash, exact.LogIndex)
	require.NoError(t, err)
	counts, err := r.GetTournamentEventCounts(t.Context(), app.ID, 0, batch.Tournament.Address, 100)
	require.NoError(t, err)
	require.EqualValues(t, tournamentRowsPerStatement, counts.StandingChanged)

	root := *batch.Tournament
	root.Snapshot.AsOfBlock = 110
	next := &repository.TournamentEventBatch{Tournament: &root}
	for range tournamentRowsPerStatement {
		next.StandingChanges = append(next.StandingChanges, &model.StandingChanged{
			ApplicationID: app.ID, TournamentAddress: root.Address, MatchCount: model.Uint256{1},
			BlockNumber: 105, TxHash: repotest.UniqueHash(),
		})
	}
	conflict := *next.StandingChanges[0]
	conflict.MatchCount = model.Uint256{2}
	next.StandingChanges = append(next.StandingChanges, &conflict)
	err = r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(100)), []*repository.TournamentEventBatch{next})
	require.ErrorIs(t, err, repository.ErrTournamentEventConflict)
	counts, err = r.GetTournamentEventCounts(t.Context(), app.ID, 0, root.Address, 110)
	require.NoError(t, err)
	require.EqualValues(t, tournamentRowsPerStatement, counts.StandingChanged, "the entire first new chunk must roll back")
	after, err := r.GetStandingChange(t.Context(), app.Name, 0, root.Address.Hex(), exact.TxHash, exact.LogIndex)
	require.NoError(t, err)
	require.Equal(t, first, after, "the original immutable row and timestamps survive")
	stored, err := r.GetTournament(t.Context(), app.Name, root.Address.Hex())
	require.NoError(t, err)
	require.Equal(t, uint64(100), stored.Snapshot.AsOfBlock)
	storedApp, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, uint64(100), storedApp.LastTournamentCheckBlock)
}

func TestStandingChangesProductionRejectsInvalidBoundsAndScope(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*model.StandingChanged)
	}{
		{"lower block", func(v *model.StandingChanged) { v.BlockNumber = 98 }},
		{"upper block", func(v *model.StandingChanged) { v.BlockNumber = 101 }},
		{disputeApplicationScope, func(v *model.StandingChanged) { v.ApplicationID++ }},
		{disputeEpochScope, func(v *model.StandingChanged) { v.EpochIndex++ }},
		{disputeTournamentScope, func(v *model.StandingChanged) { v.TournamentAddress = repotest.UniqueAddress() }},
		{"nil", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, app, batch := newRootEventInterval(t)
			value := &model.StandingChanged{ApplicationID: app.ID, TournamentAddress: batch.Tournament.Address,
				MatchCount: model.Uint256{1}, BlockNumber: 100, TxHash: repotest.UniqueHash()}
			if test.change == nil {
				value = nil
			} else {
				test.change(value)
			}
			batch.StandingChanges = []*model.StandingChanged{value}
			require.Error(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
			stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
			require.NoError(t, err)
			require.Nil(t, stored)
			storedApp, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Zero(t, storedApp.LastTournamentCheckBlock)
		})
	}
}

func TestDisputeFactsRejectsImmutableReplayAndKeepsCursor(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*model.Tournament, *model.Match)
	}{
		{"bond", func(v *model.Tournament, _ *model.Match) { v.BondValue = model.Uint256{2} }},
		{"contested one", func(v *model.Tournament, _ *model.Match) { v.ContestedFinalStateOne = new(common.Hash{}) }},
		{"contested two", func(v *model.Tournament, _ *model.Match) { v.ContestedFinalStateTwo = new(common.Hash{}) }},
		{"creation deadline", func(_ *model.Tournament, v *model.Match) { v.ResponderDeadline = 1 }},
		{"seal agree", func(_ *model.Tournament, v *model.Match) { v.LeafSeal.AgreeState = repotest.UniqueHash() }},
		{"seal position", func(_ *model.Tournament, v *model.Match) { v.LeafSeal.DivergencePosition = model.Uint256{4} }},
		{"seal one", func(_ *model.Tournament, v *model.Match) { v.LeafSeal.FinalStateOne = repotest.UniqueHash() }},
		{"seal two", func(_ *model.Tournament, v *model.Match) { v.LeafSeal.FinalStateTwo = repotest.UniqueHash() }},
		{"seal deadline one", func(_ *model.Tournament, v *model.Match) { v.LeafSeal.DeadlineOne = 84 }},
		{"seal deadline two", func(_ *model.Tournament, v *model.Match) { v.LeafSeal.DeadlineTwo = 89 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, app, batches := newTournamentInterval(t)
			child, match := batches[1].Tournament, batches[1].Matches[0]
			if test.name == "seal deadline two" {
				match.LeafSeal.DeadlineOne = 95
				match.LeafSeal.EliminableAt = 95
			}
			require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches))
			before, err := r.GetTournament(t.Context(), app.Name, child.Address.Hex())
			require.NoError(t, err)
			test.change(child, match)
			child.Snapshot.AsOfBlock = 110
			match.Snapshot.AsOfBlock = 110
			next := &repository.TournamentEventBatch{Tournament: child, Matches: []*model.Match{match}}
			err = r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(100)), []*repository.TournamentEventBatch{next})
			require.ErrorIs(t, err, repository.ErrTournamentEventConflict)
			after, err := r.GetTournament(t.Context(), app.Name, child.Address.Hex())
			require.NoError(t, err)
			require.Equal(t, before, after)
			storedApp, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Equal(t, uint64(100), storedApp.LastTournamentCheckBlock)
		})
	}
}

func TestDisputeAdvanceNewPayloadConflictsRollBackInterval(t *testing.T) {
	for _, field := range []string{"height", "deadline"} {
		t.Run(field, func(t *testing.T) {
			r, app, batches := newTournamentInterval(t)
			conflict := *batches[1].MatchAdvances[0]
			if field == "height" {
				conflict.CurrentHeight = 3
			} else {
				conflict.ResponderDeadline--
			}
			batches[1].MatchAdvances = append(batches[1].MatchAdvances, &conflict)
			require.ErrorIs(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches), repository.ErrTournamentEventConflict)
			for _, batch := range batches {
				stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
				require.NoError(t, err)
				require.Nil(t, stored)
			}
			storedApp, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Zero(t, storedApp.LastTournamentCheckBlock)
		})
	}
}

func TestStandingChangePayloadComparisonPreservesNullablePresence(t *testing.T) {
	first := model.StandingChanged{ApplicationID: 1, EpochIndex: 2, TournamentAddress: common.HexToAddress("0x11"),
		MatchCount: model.Uint256{3}, ResultAt: 4, WinnerExpiresAt: 5, BlockNumber: 6, TxHash: common.HexToHash("0x22"), LogIndex: 7}
	require.True(t, sameStandingChangedEvent(&first, &first))
	for _, test := range []struct {
		name   string
		change func(*model.StandingChanged)
	}{
		{disputeApplicationScope, func(v *model.StandingChanged) { v.ApplicationID++ }},
		{disputeEpochScope, func(v *model.StandingChanged) { v.EpochIndex++ }},
		{disputeTournamentScope, func(v *model.StandingChanged) { v.TournamentAddress = common.Address{} }},
		{"count", func(v *model.StandingChanged) { v.MatchCount = model.Uint256{4} }},
		{"dangling zero", func(v *model.StandingChanged) { v.Dangling = new(common.Hash{}) }},
		{"parent zero", func(v *model.StandingChanged) { v.ParentCommitment = new(common.Hash{}) }},
		{"result", func(v *model.StandingChanged) { v.ResultAt++ }},
		{"expiry", func(v *model.StandingChanged) { v.WinnerExpiresAt++ }},
		{"block", func(v *model.StandingChanged) { v.BlockNumber++ }},
		{"transaction", func(v *model.StandingChanged) { v.TxHash = common.Hash{} }},
		{"log", func(v *model.StandingChanged) { v.LogIndex++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			second := first
			test.change(&second)
			require.False(t, sameStandingChangedEvent(&first, &second))
			require.False(t, sameStandingChangedEvent(&second, &first))
		})
	}
	withZero := first
	withZero.Dangling = new(common.Hash{})
	copyZero := withZero
	copyZero.Dangling = new(common.Hash{})
	require.True(t, sameStandingChangedEvent(&withZero, &copyZero))
	copyZero.Dangling = new(common.HexToHash("0x11"))
	require.False(t, sameStandingChangedEvent(&withZero, &copyZero))
}

func TestDisputeRootCreationRequiresObservedEpochSeal(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*model.Tournament)
	}{
		{"unexpected root creation", func(v *model.Tournament) { v.CreationEvent = &model.TournamentCreationEvent{} }},
		{"wrong start", func(v *model.Tournament) { v.StartInstant-- }},
		{"wrong root identity", func(v *model.Tournament) { v.Address = repotest.UniqueAddress() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, app, batch := newRootEventInterval(t)
			test.change(batch.Tournament)
			require.Error(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
			stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
			require.NoError(t, err)
			require.Nil(t, stored)
			storedApp, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Zero(t, storedApp.LastTournamentCheckBlock)
		})
	}
}

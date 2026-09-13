// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const observerCheckpoint = uint64(100)

func observerUint256(t *testing.T, text string) Uint256 {
	t.Helper()
	number, ok := new(big.Int).SetString(text, 10)
	require.True(t, ok)
	value, err := Uint256FromBig(number)
	require.NoError(t, err)
	return value
}

// newPassiveObserverWindow has all eight event types and final current views.
// Its inner match was advanced twice, sealed, deleted, and its bond recovered.
func newPassiveObserverWindow(t *testing.T) (*PostgresRepository, *Application, []*repository.TournamentEventBatch) {
	t.Helper()
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(t.Context(), t, r)
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(EpochStatus_Closed).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*Epoch][]*Input{epoch: {}}, observerCheckpoint))
	root := repotest.NewTournamentBuilder(app.ID).Build()
	root.MaxLevel = 2
	makePair := func(tournament *Tournament) (*Commitment, *Commitment, *Match) {
		one := repotest.NewCommitmentBuilder(app.ID).WithTournamentAddress(tournament.Address).Build()
		two := repotest.NewCommitmentBuilder(app.ID).WithTournamentAddress(tournament.Address).Build()
		match := repotest.NewMatchBuilder(app.ID).WithTournamentAddress(tournament.Address).
			WithCommitmentOne(one.Commitment).WithCommitmentTwo(two.Commitment).Build()
		one.BlockNumber, two.BlockNumber, match.BlockNumber = 10, 11, 11
		match.TxHash, match.LogIndex = two.TxHash, 1
		match.EliminableAt = 50
		match.Winner = WinnerCommitment_ONE
		match.DeletionReason = MatchDeletionReason_CHILD_TOURNAMENT
		match.DeletionBlockNumber, match.DeletionTxHash, match.DeletionLogIndex = 80, new(repotest.UniqueHash()), new(uint64(0))
		match.Snapshot = MatchSnapshot{AsOfBlock: observerCheckpoint, Phase: MatchPhaseUninitialized, TimeoutOutcome: MatchTimeoutNone}
		return one, two, match
	}
	one, two, parentMatch := makePair(root)
	child := repotest.NewTournamentBuilder(app.ID).WithLevel(1).WithParent(root.Address, parentMatch.IDHash).Build()
	child.MaxLevel, child.Kind, child.Log2Step = 2, TournamentKindLeaf, 0
	child.CreationEvent.BlockNumber = 50
	child.StartInstant = 50
	childOne, childTwo, childMatch := makePair(child)
	childOne.BlockNumber, childTwo.BlockNumber, childMatch.BlockNumber = 51, 52, 52
	childMatch.DeletionReason, childMatch.DeletionBlockNumber = MatchDeletionReason_STEP, 70
	childMatch.LeafSeal = &LeafMatchSeal{EliminableAt: 90, BlockNumber: 60, TxHash: repotest.UniqueHash()}
	root.Snapshot = TournamentSnapshot{
		AsOfBlock: observerCheckpoint, Standing: TournamentStandingRootWinner,
		Candidate: new(one.Commitment), WinnerCommitment: new(one.Commitment), FinalStateHash: new(one.FinalStateHash),
		FinishedAtBlock: 90,
		BondRecovery: TournamentBondRecovery{
			Disposition: BondDispositionRecoverable, Claimer: new(one.SubmitterAddress), Payment: new(Uint256),
		},
	}
	child.Snapshot = TournamentSnapshot{
		AsOfBlock: observerCheckpoint, Standing: TournamentStandingInnerWinner,
		Candidate: new(childOne.Commitment), WinnerCommitment: new(childOne.Commitment), FinalStateHash: new(childOne.FinalStateHash),
		FinishedAtBlock: 80, ParentCommitment: new(parentMatch.CommitmentOne), WinnerExpiresAt: 150,
		InnerResult: &TournamentInnerResult{
			Disposition: InnerTournamentWinner, ParentCommitment: new(parentMatch.CommitmentOne), PausedAllowance: 50,
		},
		BondRecovery: TournamentBondRecovery{Disposition: BondDispositionRecovered},
	}
	childOne.Snapshot.Claimer = common.Address{}
	firstAdvance := repotest.NewMatchAdvancedBuilder(app.ID).WithTournamentAddress(child.Address).WithIDHash(childMatch.IDHash).Build()
	firstAdvance.BlockNumber, firstAdvance.LogIndex, firstAdvance.EliminableAt = 55, 0, 85
	firstAdvance.SegmentStartPosition = observerUint256(t, "18446744073709551616")
	secondAdvance := *firstAdvance
	secondAdvance.LogIndex, secondAdvance.EliminableAt = 1, 86
	secondAdvance.LeftNode = repotest.UniqueHash()
	refund := &BondEvent{
		ApplicationID: app.ID, EpochIndex: child.EpochIndex, TournamentAddress: child.Address, Type: BondEventPartialRefund,
		BlockNumber: childMatch.DeletionBlockNumber, TxHash: *childMatch.DeletionTxHash, LogIndex: 1,
		Refund: &PartialBondRefund{Recipient: childOne.SubmitterAddress, Value: observerUint256(t, "7"), Success: false},
	}
	recovery := &BondEvent{
		ApplicationID: app.ID, EpochIndex: child.EpochIndex, TournamentAddress: child.Address, Type: BondEventRecovered,
		BlockNumber: 95, TxHash: repotest.UniqueHash(), LogIndex: 0,
		Recovery: &BondRecovered{Commitment: childOne.Commitment, Claimer: childOne.SubmitterAddress,
			Payment: Uint256{}, Burned: observerUint256(t, "18446744073709551616")},
	}
	return r, app, []*repository.TournamentEventBatch{
		{Tournament: root, Commitments: []*Commitment{one, two}, Matches: []*Match{parentMatch}},
		{Tournament: child, Commitments: []*Commitment{childOne, childTwo}, Matches: []*Match{childMatch},
			MatchAdvances: []*MatchAdvanced{firstAdvance, &secondAdvance}, BondEvents: []*BondEvent{refund, recovery}},
	}
}

func TestPassiveObserverEventsAndCurrentViewsRoundTrip(t *testing.T) {
	r, app, batches := newPassiveObserverWindow(t)
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, batches, observerCheckpoint))
	for _, batch := range batches {
		stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
		require.NoError(t, err)
		require.Equal(t, batch.Tournament.Snapshot, stored.Snapshot)
		require.Equal(t, batch.Tournament.CreationEvent, stored.CreationEvent)
		for _, commitment := range batch.Commitments {
			got, err := r.GetCommitment(
				t.Context(), app.Name, commitment.EpochIndex, commitment.TournamentAddress.Hex(), commitment.Commitment.Hex(),
			)
			require.NoError(t, err)
			require.Equal(t, commitment.Snapshot, got.Snapshot)
			require.Equal(t, commitment.SubmitterAddress, got.SubmitterAddress)
			require.Equal(t, commitment.LogIndex, got.LogIndex)
		}
		for _, match := range batch.Matches {
			got, err := r.GetMatch(t.Context(), app.Name, match.EpochIndex, match.TournamentAddress.Hex(), match.IDHash.Hex())
			require.NoError(t, err)
			require.Equal(t, match.Snapshot, got.Snapshot)
			require.Equal(t, match.LeafSeal, got.LeafSeal)
			require.Equal(t, match.DeletionLogIndex, got.DeletionLogIndex)
			require.Equal(t, match.EliminableAt, got.EliminableAt)
		}
	}
	child := batches[1].Tournament
	childMatch := batches[1].Matches[0]
	advances, total, err := r.ListMatchAdvances(
		t.Context(), app.Name, child.EpochIndex, child.Address.Hex(), childMatch.IDHash.Hex(), repository.Pagination{}, false,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(2), total)
	require.Equal(t, advances[0].OtherParent, advances[1].OtherParent, "parent hashes are not event identities")
	require.Equal(t, uint64(0), advances[0].LogIndex)
	require.Equal(t, uint64(1), advances[1].LogIndex)
	require.Equal(t, batches[1].MatchAdvances[0].SegmentStartPosition, advances[0].SegmentStartPosition)
	for _, advance := range advances {
		got, err := r.GetMatchAdvanced(t.Context(), app.Name, child.EpochIndex, child.Address.Hex(), childMatch.IDHash.Hex(),
			advance.TxHash, advance.LogIndex)
		require.NoError(t, err)
		require.Equal(t, advance, got)
	}
	bonds, total, err := r.ListBondEvents(t.Context(), app.Name, repository.BondEventFilter{}, repository.Pagination{}, false)
	require.NoError(t, err)
	require.Equal(t, uint64(2), total)
	require.Equal(t, batches[1].BondEvents[0].Refund, bonds[0].Refund)
	require.False(t, bonds[0].Refund.Success, "a failed refund is a recorded request, not a transferred amount")
	require.Equal(t, batches[1].BondEvents[1].Recovery, bonds[1].Recovery)
	require.Zero(t, bonds[1].Recovery.Payment)
	for _, bond := range bonds {
		got, err := r.GetBondEvent(t.Context(), app.Name, bond.TxHash, bond.LogIndex)
		require.NoError(t, err)
		require.Equal(t, bond, got)
	}
	// Replay must retain event rows, their timestamps, and their log identities.
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, batches, observerCheckpoint))
	replayed, _, err := r.ListBondEvents(t.Context(), app.Name, repository.BondEventFilter{}, repository.Pagination{}, false)
	require.NoError(t, err)
	require.Equal(t, bonds, replayed)
	replayedAdvances, _, err := r.ListMatchAdvances(
		t.Context(), app.Name, child.EpochIndex, child.Address.Hex(), childMatch.IDHash.Hex(), repository.Pagination{}, false,
	)
	require.NoError(t, err)
	require.Equal(t, advances, replayedAdvances)
}

func TestPassiveObserverRefreshClearsExpiredInnerWinner(t *testing.T) {
	r, app, batches := newPassiveObserverWindow(t)
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, batches, observerCheckpoint))
	child := *batches[1].Tournament
	child.Snapshot.AsOfBlock = 150
	child.Snapshot.Standing = TournamentStandingInnerEliminableWinnerExpired
	child.Snapshot.WinnerCommitment, child.Snapshot.FinalStateHash, child.Snapshot.ParentCommitment = nil, nil, nil
	child.Snapshot.WinnerExpiresAt = 0
	child.Snapshot.InnerResult = &TournamentInnerResult{Disposition: InnerTournamentEliminable}
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, []*repository.TournamentEventBatch{{Tournament: &child}}, 150))
	stored, err := r.GetTournament(t.Context(), app.Name, child.Address.Hex())
	require.NoError(t, err)
	require.Equal(t, child.Snapshot, stored.Snapshot)
	require.NotNil(t, stored.Snapshot.Candidate)
	require.Nil(t, stored.Snapshot.WinnerCommitment)
	require.Nil(t, stored.Snapshot.FinalStateHash)
	parent, err := r.GetTournament(t.Context(), app.Name, batches[0].Tournament.Address.Hex())
	require.NoError(t, err)
	require.Equal(t, observerCheckpoint, parent.Snapshot.AsOfBlock, "a child-only batch does not relabel the parent view")
	appState, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, uint64(150), appState.LastTournamentCheckBlock)
}

func TestPassiveObserverEmptyWindowKeepsStoredViews(t *testing.T) {
	r, app, batches := newPassiveObserverWindow(t)
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, batches, observerCheckpoint))
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, nil, observerCheckpoint+1))
	for _, batch := range batches {
		stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
		require.NoError(t, err)
		require.Equal(t, batch.Tournament.Snapshot, stored.Snapshot)
	}
	storedApp, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, observerCheckpoint+1, storedApp.LastTournamentCheckBlock)
}

func TestPassiveObserverAdvanceQueriesStayWithinMatch(t *testing.T) {
	r, app, batches := newPassiveObserverWindow(t)
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, batches, observerCheckpoint))
	child, match := batches[1].Tournament, batches[1].Matches[0]
	advance := batches[1].MatchAdvances[0]
	otherApp := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(t.Context(), t, r)
	for _, scope := range []struct {
		name       string
		appName    string
		epoch      uint64
		tournament common.Address
		match      common.Hash
	}{
		{"wrong application", otherApp.Name, child.EpochIndex, child.Address, match.IDHash},
		{"wrong epoch", app.Name, child.EpochIndex + 1, child.Address, match.IDHash},
		{"wrong tournament", app.Name, child.EpochIndex, batches[0].Tournament.Address, match.IDHash},
		{"wrong match", app.Name, child.EpochIndex, child.Address, batches[0].Matches[0].IDHash},
	} {
		t.Run(scope.name, func(t *testing.T) {
			got, err := r.GetMatchAdvanced(t.Context(), scope.appName, scope.epoch, scope.tournament.Hex(), scope.match.Hex(),
				advance.TxHash, advance.LogIndex)
			require.NoError(t, err)
			require.Nil(t, got)
			rows, total, err := r.ListMatchAdvances(t.Context(), scope.appName, scope.epoch, scope.tournament.Hex(), scope.match.Hex(),
				repository.Pagination{}, false)
			require.NoError(t, err)
			require.Zero(t, total)
			require.Empty(t, rows)
		})
	}
	for _, descending := range []bool{false, true} {
		for offset := uint64(0); offset < 2; offset++ {
			rows, total, err := r.ListMatchAdvances(t.Context(), app.Name, child.EpochIndex, child.Address.Hex(), match.IDHash.Hex(),
				repository.Pagination{Limit: 1, Offset: offset}, descending)
			require.NoError(t, err)
			require.Equal(t, uint64(2), total)
			require.Len(t, rows, 1)
			expectedLog := offset
			if descending {
				expectedLog = 1 - offset
			}
			require.Equal(t, expectedLog, rows[0].LogIndex)
		}
	}
}

func TestPassiveObserverSQLMatchesModelEnums(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	for _, enum := range []struct {
		name   string
		values []string
	}{
		{"TournamentKind", observerEnumStrings(TournamentKindAllValues)},
		{"TournamentStandingState", observerEnumStrings(TournamentStandingStateAllValues)},
		{"MatchPhase", observerEnumStrings(MatchPhaseAllValues)},
		{"CommitmentSide", observerEnumStrings(CommitmentSideAllValues)},
		{"MatchTimeoutOutcome", observerEnumStrings(MatchTimeoutOutcomeAllValues)},
		{"InnerTournamentDisposition", observerEnumStrings(InnerTournamentDispositionAllValues)},
		{"BondDisposition", observerEnumStrings(BondDispositionAllValues)},
		{"BondEventType", observerEnumStrings(BondEventTypeAllValues)},
	} {
		t.Run(enum.name, func(t *testing.T) {
			rows, err := r.db.Query(t.Context(), `
				SELECT enumlabel FROM pg_enum
				JOIN pg_type ON pg_type.oid = pg_enum.enumtypid
				WHERE pg_type.typname = $1 ORDER BY enumsortorder`, enum.name)
			require.NoError(t, err)
			labels, err := pgx.CollectRows(rows, pgx.RowTo[string])
			require.NoError(t, err)
			require.Equal(t, enum.values, labels)
		})
	}
}

func observerEnumStrings[T ~string](values []T) []string {
	labels := make([]string, len(values))
	for i, value := range values {
		labels[i] = string(value)
	}
	return labels
}

func TestPassiveObserverRejectsImmutableFactChangesAndOlderViews(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*repository.TournamentEventBatch)
	}{
		{"child creation", func(b *repository.TournamentEventBatch) { b.Tournament.CreationEvent.LogIndex++ }},
		{"commitment join", func(b *repository.TournamentEventBatch) { b.Commitments[0].FinalStateHash = repotest.UniqueHash() }},
		{"match creation", func(b *repository.TournamentEventBatch) { b.Matches[0].LeftOfTwo = repotest.UniqueHash() }},
		{"leaf seal", func(b *repository.TournamentEventBatch) { b.Matches[0].LeafSeal.EliminableAt++ }},
		{"match deletion", func(b *repository.TournamentEventBatch) { b.Matches[0].DeletionTxHash = new(repotest.UniqueHash()) }},
		{"older view", func(b *repository.TournamentEventBatch) {
			b.Tournament.Snapshot.AsOfBlock = observerCheckpoint - 1
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, app, batches := newPassiveObserverWindow(t)
			require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, batches, observerCheckpoint))
			batch := batches[1]
			before, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
			require.NoError(t, err)
			batch.Tournament.Snapshot.AsOfBlock++
			test.change(batch)
			head := batch.Tournament.Snapshot.AsOfBlock
			for _, commitment := range batch.Commitments {
				commitment.Snapshot.AsOfBlock = head
			}
			for _, match := range batch.Matches {
				match.Snapshot.AsOfBlock = head
			}
			err = r.StoreTournamentEvents(t.Context(), app.ID, []*repository.TournamentEventBatch{batch}, head)
			require.ErrorIs(t, err, repository.ErrTournamentEventConflict)
			after, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
			require.NoError(t, err)
			require.Equal(t, before, after, "the earlier tournament refresh must roll back with a later conflict")
			storedApp, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Equal(t, observerCheckpoint, storedApp.LastTournamentCheckBlock)
		})
	}
}

func TestPassiveObserverConflictingEventRollsBackSnapshot(t *testing.T) {
	for _, event := range []string{"advance", "bond"} {
		t.Run(event, func(t *testing.T) {
			r, app, batches := newPassiveObserverWindow(t)
			require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, batches, observerCheckpoint))
			child := *batches[1].Tournament
			child.Snapshot.AsOfBlock = 101
			child.Snapshot.InnerResult = &TournamentInnerResult{Disposition: InnerTournamentWinner,
				ParentCommitment: child.Snapshot.ParentCommitment, PausedAllowance: 49}
			batch := &repository.TournamentEventBatch{Tournament: &child}
			if event == "advance" {
				advance := *batches[1].MatchAdvances[0]
				advance.LeftNode = repotest.UniqueHash()
				batch.MatchAdvances = []*MatchAdvanced{&advance}
			} else {
				bond := *batches[1].BondEvents[0]
				refund := *bond.Refund
				refund.Success = true
				bond.Refund = &refund
				batch.BondEvents = []*BondEvent{&bond}
			}
			err := r.StoreTournamentEvents(t.Context(), app.ID, []*repository.TournamentEventBatch{batch}, 101)
			require.ErrorIs(t, err, repository.ErrTournamentEventConflict)
			stored, err := r.GetTournament(t.Context(), app.Name, child.Address.Hex())
			require.NoError(t, err)
			require.Equal(t, batches[1].Tournament.Snapshot, stored.Snapshot)
			storedApp, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Equal(t, observerCheckpoint, storedApp.LastTournamentCheckBlock)
		})
	}
}

func TestUint256DomainBoundsAndExactIntegerScale(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	for _, text := range []string{"0", "1.00", "18446744073709551616",
		"115792089237316195423570985008687907853269984665640564039457584007913129639935"} {
		var value Uint256
		require.NoError(t, r.db.QueryRow(t.Context(), "SELECT $1::uint256", text).Scan(&value))
	}
	for _, text := range []string{"-1", "0.5", "NaN", "Infinity",
		"115792089237316195423570985008687907853269984665640564039457584007913129639936"} {
		var value Uint256
		err := r.db.QueryRow(t.Context(), "SELECT $1::uint256", text).Scan(&value)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr, text)
		require.Equal(t, "23514", pgErr.Code, text)
	}
}

func TestPassiveObserverMatchPhaseRoundTrip(t *testing.T) {
	r, app, tournament, match := newTournamentIntegrityFixture(t)
	tournament.Snapshot.Standing = TournamentStandingMatchesActive
	for _, phase := range []MatchPhase{MatchPhaseBisecting, MatchPhaseReadyToSeal, MatchPhaseSealed, MatchPhaseUninitialized} {
		t.Run(phase.String(), func(t *testing.T) {
			head := tournament.Snapshot.AsOfBlock + 1
			tournament.Snapshot.AsOfBlock = head
			tournament.Snapshot.AcceptsJoins = false
			match.Snapshot.AsOfBlock = head
			match.Snapshot.Phase = phase
			switch phase {
			case MatchPhaseBisecting:
				match.Snapshot.Bisection.SegmentStartPosition = observerUint256(t, "18446744073709551616")
				match.Snapshot.Bisection.SegmentStartCycle = observerUint256(t, "18446744073709551617")
			case MatchPhaseReadyToSeal:
				match.Snapshot.Bisection.CurrentHeight = nil
			case MatchPhaseSealed:
				match.Snapshot.Bisection = nil
				match.Snapshot.Sealed = &MatchSealedSnapshot{AgreeState: common.Hash{},
					DivergencePosition: observerUint256(t, "18446744073709551616"),
					DivergenceCycle:    observerUint256(t, "18446744073709551617"),
					FinalStateOne:      repotest.UniqueHash(), FinalStateTwo: repotest.UniqueHash()}
			case MatchPhaseUninitialized:
				match.Snapshot.Sealed = nil
				match.DeletionReason = MatchDeletionReason_TIMEOUT
				match.DeletionBlockNumber, match.DeletionTxHash, match.DeletionLogIndex = head, new(common.Hash), new(uint64(0))
				tournament.Snapshot.Standing = TournamentStandingRootFailed
				tournament.Snapshot.FinishedAtBlock = head
				tournament.Snapshot.BondRecovery.Disposition = BondDispositionNoWinner
			}
			require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID,
				[]*repository.TournamentEventBatch{{Tournament: tournament, Matches: []*Match{match}}}, head))
			stored, err := r.GetMatch(t.Context(), app.Name, match.EpochIndex, tournament.Address.Hex(), match.IDHash.Hex())
			require.NoError(t, err)
			require.Equal(t, match.Snapshot, stored.Snapshot)
		})
	}
}

func TestPassiveObserverRejectsInvalidPhasePayload(t *testing.T) {
	r, app, _, initial := newTournamentIntegrityFixture(t)
	for _, test := range []struct {
		name   string
		change func(*Match)
	}{
		{"bisecting without payload", func(m *Match) { m.Snapshot.Bisection = nil }},
		{"ready with height", func(m *Match) { m.Snapshot.Phase = MatchPhaseReadyToSeal }},
		{"sealed with bisection", func(m *Match) { m.Snapshot.Phase = MatchPhaseSealed }},
		{"uninitialized without deletion", func(m *Match) { m.Snapshot.Phase = MatchPhaseUninitialized }},
	} {
		t.Run(test.name, func(t *testing.T) {
			match := *initial
			test.change(&match)
			err := r.CreateMatch(t.Context(), app.Name, &match)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			require.Equal(t, "matches_current_phase_check", pgErr.ConstraintName)
		})
	}
}

func TestPassiveObserverEpochFilterIncludesTerminalEpochs(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(t.Context(), t, r)
	accepted := repotest.NewEpochBuilder(app.ID).WithIndex(0).WithStatus(EpochStatus_Closed).Build()
	closed := repotest.NewEpochBuilder(app.ID).WithIndex(1).WithStatus(EpochStatus_Closed).Build()
	open := repotest.NewEpochBuilder(app.ID).WithIndex(2).WithStatus(EpochStatus_Open).Build()
	closed.TournamentAddress, accepted.TournamentAddress = new(repotest.UniqueAddress()), new(repotest.UniqueAddress())
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
		map[*Epoch][]*Input{closed: {}, accepted: {}, open: {}}, observerCheckpoint))
	repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, accepted, EpochStatus_ClaimAccepted)
	epochs, total, err := r.ListEpochs(t.Context(), app.Name,
		repository.EpochFilter{HasTournament: new(true)}, repository.Pagination{}, false)
	require.NoError(t, err)
	require.Equal(t, uint64(2), total)
	require.Equal(t, EpochStatus_ClaimAccepted, epochs[0].Status)
	require.Equal(t, EpochStatus_Closed, epochs[1].Status)
}

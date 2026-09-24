// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"
	"slices"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

// tournamentEventBatch folds immutable event facts into copies of the stored
// participants, then refreshes mutable views at the same pinned block.
func (s *Service) tournamentEventBatch(
	ctx context.Context, app *Application, epoch *Epoch, tournament *Tournament,
	adapter TournamentAdapter, events *TournamentEvents, block uint64,
) (*repository.TournamentEventBatch, error) {
	address := tournament.Address.Hex()
	commitments, _, err := s.repository.ListCommitments(ctx, app.Name,
		repository.CommitmentFilter{EpochIndex: &epoch.Index, TournamentAddress: &address}, repository.Pagination{}, false)
	if err != nil {
		return nil, fmt.Errorf("loading tournament commitments: %w", err)
	}
	matches, _, err := s.repository.ListMatches(ctx, app.Name,
		repository.MatchFilter{EpochIndex: &epoch.Index, TournamentAddress: &address}, repository.Pagination{}, false)
	if err != nil {
		return nil, fmt.Errorf("loading tournament matches: %w", err)
	}
	commitmentByHash := make(map[common.Hash]*Commitment, len(commitments)+len(events.CommitmentJoined))
	for _, stored := range commitments {
		projection := *stored
		commitmentByHash[stored.Commitment] = &projection
	}
	matchByHash := make(map[common.Hash]*Match, len(matches)+len(events.MatchCreated))
	unchangedDeletedMatches := make(map[common.Hash]bool, len(matches))
	for _, stored := range matches {
		projection := *stored
		matchByHash[stored.IDHash] = &projection
		if deletedMatchObservationComplete(stored, app.LastTournamentCheckBlock, block) {
			unchangedDeletedMatches[stored.IDHash] = true
		}
	}
	batch := &repository.TournamentEventBatch{Tournament: tournament}
	for _, event := range events.CommitmentJoined {
		joined := &Commitment{ApplicationID: app.ID, EpochIndex: epoch.Index, TournamentAddress: tournament.Address,
			Commitment: event.Commitment, FinalStateHash: event.FinalStateHash, SubmitterAddress: event.Submitter,
			BlockNumber: event.Raw.BlockNumber, TxHash: event.Raw.TxHash, LogIndex: uint64(event.Raw.Index)}
		if previous := commitmentByHash[joined.Commitment]; previous != nil {
			if previous.FinalStateHash != joined.FinalStateHash || previous.SubmitterAddress != joined.SubmitterAddress ||
				previous.BlockNumber != joined.BlockNumber || previous.TxHash != joined.TxHash || previous.LogIndex != joined.LogIndex {
				return nil, fmt.Errorf("commitment %s has conflicting join events", joined.Commitment)
			}
		} else {
			commitmentByHash[joined.Commitment] = joined
		}
	}
	for _, event := range events.MatchCreated {
		delete(unchangedDeletedMatches, event.MatchIdHash)
		created := &Match{ApplicationID: app.ID, EpochIndex: epoch.Index, TournamentAddress: tournament.Address,
			IDHash: event.MatchIdHash, CommitmentOne: event.One, CommitmentTwo: event.Two, LeftOfTwo: event.LeftOfTwo,
			BlockNumber: event.Raw.BlockNumber, TxHash: event.Raw.TxHash, LogIndex: uint64(event.Raw.Index),
			EliminableAt: event.EliminableAt, Winner: WinnerCommitment_NONE, DeletionReason: MatchDeletionReason_NOT_DELETED}
		if previous := matchByHash[created.IDHash]; previous != nil {
			if previous.CommitmentOne != created.CommitmentOne || previous.CommitmentTwo != created.CommitmentTwo ||
				previous.LeftOfTwo != created.LeftOfTwo || previous.BlockNumber != created.BlockNumber ||
				previous.TxHash != created.TxHash || previous.LogIndex != created.LogIndex ||
				previous.EliminableAt != created.EliminableAt {
				return nil, fmt.Errorf("match %s has conflicting creation events", created.IDHash)
			}
		} else {
			matchByHash[created.IDHash] = created
		}
	}
	for _, event := range events.MatchAdvanced {
		delete(unchangedDeletedMatches, event.MatchIdHash)
		if matchByHash[event.MatchIdHash] == nil {
			return nil, fmt.Errorf("match advance has no creation event for %x", event.MatchIdHash)
		}
		position, err := Uint256FromBig(event.SegmentStartPosition)
		if err != nil {
			return nil, fmt.Errorf("match advance position: %w", err)
		}
		batch.MatchAdvances = append(batch.MatchAdvances, &MatchAdvanced{
			ApplicationID: app.ID, EpochIndex: epoch.Index, TournamentAddress: tournament.Address,
			IDHash: event.MatchIdHash, OtherParent: event.OtherParent, LeftNode: event.LeftNode,
			SegmentStartPosition: position, EliminableAt: event.EliminableAt,
			BlockNumber: event.Raw.BlockNumber, TxHash: event.Raw.TxHash, LogIndex: uint64(event.Raw.Index),
		})
	}
	for _, event := range events.LeafMatchSealed {
		delete(unchangedDeletedMatches, event.MatchIdHash)
		match := matchByHash[event.MatchIdHash]
		if match == nil {
			return nil, fmt.Errorf("leaf seal has no creation event for %x", event.MatchIdHash)
		}
		seal := &LeafMatchSeal{EliminableAt: event.EliminableAt,
			BlockNumber: event.Raw.BlockNumber, TxHash: event.Raw.TxHash, LogIndex: uint64(event.Raw.Index)}
		if match.LeafSeal != nil && *match.LeafSeal != *seal {
			return nil, fmt.Errorf("match %s has conflicting leaf seal events", match.IDHash)
		}
		match.LeafSeal = seal
	}
	for _, event := range events.MatchDeleted {
		delete(unchangedDeletedMatches, event.MatchIdHash)
		match := matchByHash[event.MatchIdHash]
		if match == nil || match.CommitmentOne != event.One || match.CommitmentTwo != event.Two {
			return nil, fmt.Errorf("match deletion does not match its creation pair for %x", event.MatchIdHash)
		}
		reason, err := MatchDeletionReasonFromUint8(event.Reason)
		if err != nil {
			return nil, err
		}
		winner, err := WinnerCommitmentFromUint8(event.WinnerCommitment)
		if err != nil {
			return nil, err
		}
		if match.DeletionTxHash != nil && (*match.DeletionTxHash != event.Raw.TxHash ||
			match.DeletionLogIndex == nil || *match.DeletionLogIndex != uint64(event.Raw.Index) ||
			match.DeletionBlockNumber != event.Raw.BlockNumber || match.DeletionReason != reason || match.Winner != winner) {
			return nil, fmt.Errorf("match %s has conflicting deletion events", match.IDHash)
		}
		match.DeletionReason, match.Winner = reason, winner
		match.DeletionBlockNumber, match.DeletionTxHash = event.Raw.BlockNumber, new(event.Raw.TxHash)
		match.DeletionLogIndex = new(uint64(event.Raw.Index))
	}
	if err := appendTournamentBondEvents(batch, app, epoch, events); err != nil {
		return nil, err
	}
	for _, commitment := range commitmentByHash {
		standing, err := adapter.CommitmentStanding(pinnedCallOpts(ctx, block), commitment.Commitment)
		if err != nil {
			return nil, fmt.Errorf("reading commitment %s standing: %w", commitment.Commitment, err)
		}
		if !standing.Joined || standing.FinalState != commitment.FinalStateHash {
			return nil, fmt.Errorf("commitment %s standing does not match its join event", commitment.Commitment)
		}
		commitment.Snapshot = CommitmentSnapshot{AsOfBlock: block, Claimer: standing.Claimer,
			ClockRunning: standing.ClockRunning, ClockDeadline: standing.ClockDeadline, ClockAllowance: standing.ClockAllowance}
		batch.Commitments = append(batch.Commitments, commitment)
	}
	for _, match := range matchByHash {
		if unchangedDeletedMatches[match.IDHash] {
			// Keep the stored certification block. There is no new match view to publish.
			continue
		}
		view, err := adapter.MatchSnapshot(pinnedCallOpts(ctx, block), match.CommitmentOne, match.CommitmentTwo)
		if err != nil {
			return nil, fmt.Errorf("reading match %s snapshot: %w", match.IDHash, err)
		}
		match.Snapshot, err = projectMatchSnapshot(view, block, match.DeletionTxHash != nil)
		if err != nil {
			return nil, fmt.Errorf("match %s: %w", match.IDHash, err)
		}
		batch.Matches = append(batch.Matches, match)
	}
	// Stable ordering makes the batch deterministic without changing event chronology.
	slices.SortFunc(batch.Commitments, func(a, b *Commitment) int { return a.Commitment.Cmp(b.Commitment) })
	slices.SortFunc(batch.Matches, func(a, b *Match) int { return a.IDHash.Cmp(b.IDHash) })
	return batch, nil
}

// Deleted matches have no mutable contract state. Reuse only a snapshot that
// was validated after deletion and committed within the published observation.
func deletedMatchObservationComplete(match *Match, cursor, head uint64) bool {
	if match.DeletionTxHash == nil || *match.DeletionTxHash == (common.Hash{}) || match.DeletionLogIndex == nil ||
		match.DeletionBlockNumber == 0 || match.DeletionBlockNumber > match.Snapshot.AsOfBlock ||
		match.Snapshot.AsOfBlock > cursor || cursor > head ||
		match.DeletionReason == MatchDeletionReason_NOT_DELETED || !slices.Contains(MatchDeletionReasonAllValues, match.DeletionReason) ||
		!slices.Contains(WinnerCommitmentAllValues, match.Winner) {
		return false
	}
	return match.Snapshot == (MatchSnapshot{AsOfBlock: match.Snapshot.AsOfBlock,
		Phase: MatchPhaseUninitialized, TimeoutOutcome: MatchTimeoutNone})
}

func appendTournamentBondEvents(batch *repository.TournamentEventBatch, app *Application, epoch *Epoch, events *TournamentEvents) error {
	for _, event := range events.PartialBondRefund {
		value, err := Uint256FromBig(event.Value)
		if err != nil {
			return fmt.Errorf("partial bond refund value: %w", err)
		}
		batch.BondEvents = append(batch.BondEvents, &BondEvent{
			ApplicationID: app.ID, EpochIndex: epoch.Index, TournamentAddress: batch.Tournament.Address,
			Type: BondEventPartialRefund, BlockNumber: event.Raw.BlockNumber, TxHash: event.Raw.TxHash, LogIndex: uint64(event.Raw.Index),
			Refund: &PartialBondRefund{Recipient: event.Recipient, Value: value, Success: event.Success},
		})
	}
	for _, event := range events.BondRecovered {
		payment, err := Uint256FromBig(event.Payment)
		if err != nil {
			return fmt.Errorf("bond recovery payment: %w", err)
		}
		burned, err := Uint256FromBig(event.Burned)
		if err != nil {
			return fmt.Errorf("bond recovery burn: %w", err)
		}
		batch.BondEvents = append(batch.BondEvents, &BondEvent{
			ApplicationID: app.ID, EpochIndex: epoch.Index, TournamentAddress: batch.Tournament.Address,
			Type: BondEventRecovered, BlockNumber: event.Raw.BlockNumber, TxHash: event.Raw.TxHash, LogIndex: uint64(event.Raw.Index),
			Recovery: &BondRecovered{Commitment: event.Commitment, Claimer: event.Claimer, Payment: payment, Burned: burned},
		})
	}
	return nil
}

func projectMatchSnapshot(view ObservedMatchSnapshot, block uint64, deleted bool) (MatchSnapshot, error) {
	result := MatchSnapshot{AsOfBlock: block, Phase: view.Phase,
		TimeoutOutcome: view.TimeoutOutcome, DeferredCharge: view.DeferredCharge}
	if deleted && view.Phase != MatchPhaseUninitialized {
		return MatchSnapshot{}, fmt.Errorf("deleted match has phase %s", view.Phase)
	}
	switch view.Phase {
	case MatchPhaseBisecting:
		if view.Bisecting == nil || view.ReadyToSeal != nil || view.Sealed != nil {
			return MatchSnapshot{}, fmt.Errorf("invalid bisecting match payload")
		}
		v := view.Bisecting
		position, err := Uint256FromBig(v.SegmentStartPosition)
		if err != nil {
			return MatchSnapshot{}, err
		}
		cycle, err := Uint256FromBig(v.SegmentStartCycle)
		if err != nil {
			return MatchSnapshot{}, err
		}
		result.Bisection = &MatchBisectionSnapshot{RevealingParent: v.RevealingParent, WaitingLeft: v.WaitingLeft,
			WaitingRight: v.WaitingRight, SegmentStartPosition: position, SegmentStartCycle: cycle,
			CurrentHeight: new(v.CurrentHeight), Responder: v.Responder}
	case MatchPhaseReadyToSeal:
		if view.ReadyToSeal == nil || view.Bisecting != nil || view.Sealed != nil {
			return MatchSnapshot{}, fmt.Errorf("invalid ready-to-seal match payload")
		}
		v := view.ReadyToSeal
		position, err := Uint256FromBig(v.SegmentStartPosition)
		if err != nil {
			return MatchSnapshot{}, err
		}
		cycle, err := Uint256FromBig(v.SegmentStartCycle)
		if err != nil {
			return MatchSnapshot{}, err
		}
		result.Bisection = &MatchBisectionSnapshot{RevealingParent: v.RevealingParent, WaitingLeft: v.WaitingLeft,
			WaitingRight: v.WaitingRight, SegmentStartPosition: position, SegmentStartCycle: cycle, Responder: v.Responder}
	case MatchPhaseSealed:
		if view.Sealed == nil || view.Bisecting != nil || view.ReadyToSeal != nil {
			return MatchSnapshot{}, fmt.Errorf("invalid sealed match payload")
		}
		v := view.Sealed
		position, err := Uint256FromBig(v.DivergencePosition)
		if err != nil {
			return MatchSnapshot{}, err
		}
		cycle, err := Uint256FromBig(v.DivergenceCycle)
		if err != nil {
			return MatchSnapshot{}, err
		}
		result.Sealed = &MatchSealedSnapshot{AgreeState: v.AgreeState, DivergencePosition: position, DivergenceCycle: cycle,
			FinalStateOne: v.FinalStateOne, FinalStateTwo: v.FinalStateTwo}
	case MatchPhaseUninitialized:
		if !deleted || view.Bisecting != nil || view.ReadyToSeal != nil || view.Sealed != nil ||
			view.TimeoutOutcome != MatchTimeoutNone || view.DeferredCharge != 0 {
			return MatchSnapshot{}, fmt.Errorf("uninitialized match has no matching deletion or has live payload")
		}
	default:
		return MatchSnapshot{}, fmt.Errorf("unknown match phase %s", view.Phase)
	}
	return result, nil
}

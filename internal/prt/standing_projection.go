// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"
	"math"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

func optionalStandingHash(hash common.Hash) *common.Hash {
	if hash == (common.Hash{}) {
		return nil
	}
	return new(hash)
}

func sameStandingHash(one, two *common.Hash) bool {
	return (one == nil && two == nil) || (one != nil && two != nil && *one == *two)
}

func appendTournamentStandingEvents(batch *repository.TournamentEventBatch, app *model.Application, epoch *model.Epoch,
	events *TournamentEvents, matches map[common.Hash]*model.Match, parent *model.Match,
) error {
	t := batch.Tournament
	var latest *model.StandingChanged
	for _, event := range events.StandingChanged {
		count, err := model.Uint256FromBig(event.MatchCount)
		if err != nil {
			return fmt.Errorf("standing match count: %w", err)
		}
		standing := &model.StandingChanged{ApplicationID: app.ID, EpochIndex: epoch.Index, TournamentAddress: t.Address,
			MatchCount: count, Dangling: optionalStandingHash(event.Dangling),
			ParentCommitment: optionalStandingHash(event.ParentCommitment),
			ResultAt:         event.ResultAt, WinnerExpiresAt: event.WinnerExpiresAt, BlockNumber: event.Raw.BlockNumber,
			TxHash:   event.Raw.TxHash,
			LogIndex: uint64(event.Raw.Index)}
		if standing.ParentCommitment != nil && (t.Level == uint64(RootLevel) || parent == nil ||
			(*standing.ParentCommitment != parent.CommitmentOne && *standing.ParentCommitment != parent.CommitmentTwo)) {
			return fmt.Errorf("standing parent commitment is outside the parent match pair")
		}
		if count.ToBig().Sign() == 0 && (t.Allowance > math.MaxUint64-t.StartInstant || standing.ResultAt < t.StartInstant+t.Allowance) {
			return fmt.Errorf("standing result precedes tournament closure")
		}
		batch.StandingChanges = append(batch.StandingChanges, standing)
		if latest == nil || standing.BlockNumber > latest.BlockNumber ||
			(standing.BlockNumber == latest.BlockNumber && standing.LogIndex > latest.LogIndex) {
			latest = standing
		}
	}
	if latest == nil {
		return nil
	}
	live := uint64(0)
	for _, match := range matches {
		if match.DeletionReason == model.MatchDeletionReason_NOT_DELETED {
			live++
		}
	}
	count := latest.MatchCount.ToBig()
	if count.Cmp(new(big.Int).SetUint64(live)) != 0 {
		return fmt.Errorf("standing match count does not match folded live matches")
	}
	snapshot := t.Snapshot
	if (count.Sign() > 0) != (snapshot.Standing == model.TournamentStandingMatchesActive) {
		return fmt.Errorf("standing active matches do not match current view")
	}
	if !sameStandingHash(latest.Dangling, snapshot.Candidate) {
		return fmt.Errorf("standing dangling does not match current candidate")
	}
	if isTerminalTournamentStanding(snapshot.Standing) && latest.ResultAt != snapshot.FinishedAtBlock {
		return fmt.Errorf("standing result does not match terminal current view")
	}
	if snapshot.Standing == model.TournamentStandingInnerWinner &&
		(!sameStandingHash(latest.ParentCommitment, snapshot.ParentCommitment) || latest.WinnerExpiresAt != snapshot.WinnerExpiresAt) {
		return fmt.Errorf("standing winner does not match current inner result")
	}
	return nil
}

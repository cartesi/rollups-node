// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
)

const (
	tournamentEventCommitmentJoined   = "CommitmentJoined"
	tournamentEventMatchCreated       = "MatchCreated"
	tournamentEventMatchAdvanced      = "MatchAdvanced"
	tournamentEventLeafMatchSealed    = "LeafMatchSealed"
	tournamentEventMatchDeleted       = "MatchDeleted"
	tournamentEventNewInnerTournament = "NewInnerTournament"
)

func zeroStructuralEventCounts() StructuralEventCounts {
	return StructuralEventCounts{
		CommitmentJoined: new(big.Int), MatchCreated: new(big.Int), MatchAdvanced: new(big.Int),
		LeafMatchSealed: new(big.Int), MatchDeleted: new(big.Int), NewInnerTournament: new(big.Int),
	}
}

// Each successful refundable action emits one advance, leaf seal, deletion,
// or child creation, followed by one refund event even if payment fails.
// Successful bond recovery is a one-shot transition and has its own event.
func validateTournamentFinancialEvents(before, after model.BondDisposition, events *TournamentEvents) error {
	expectedRefunds := len(events.MatchAdvanced) + len(events.LeafMatchSealed) + len(events.MatchDeleted) + len(events.NewInnerTournament)
	if len(events.PartialBondRefund) != expectedRefunds {
		return fmt.Errorf("PartialBondRefund event count mismatch: expected %d, fetched %d", expectedRefunds, len(events.PartialBondRefund))
	}
	if before == model.BondDispositionRecovered && after != model.BondDispositionRecovered {
		return fmt.Errorf("bond recovery disposition regressed from RECOVERED to %s", after)
	}
	expectedRecoveries := 0
	if before != model.BondDispositionRecovered && after == model.BondDispositionRecovered {
		expectedRecoveries = 1
	}
	if len(events.BondRecovered) != expectedRecoveries {
		return fmt.Errorf("BondRecovered event count mismatch: expected %d, fetched %d", expectedRecoveries, len(events.BondRecovered))
	}
	return nil
}

// validateTournamentEventCounts checks each independent structural stream.
// A mismatch is an observation failure, not proof of local data corruption.
func validateTournamentEventCounts(before, after StructuralEventCounts, events *TournamentEvents) error {
	for _, stream := range []struct {
		name          string
		before, after *big.Int
		fetched       int
	}{
		{tournamentEventCommitmentJoined, before.CommitmentJoined, after.CommitmentJoined, len(events.CommitmentJoined)},
		{tournamentEventMatchCreated, before.MatchCreated, after.MatchCreated, len(events.MatchCreated)},
		{tournamentEventMatchAdvanced, before.MatchAdvanced, after.MatchAdvanced, len(events.MatchAdvanced)},
		{tournamentEventLeafMatchSealed, before.LeafMatchSealed, after.LeafMatchSealed, len(events.LeafMatchSealed)},
		{tournamentEventMatchDeleted, before.MatchDeleted, after.MatchDeleted, len(events.MatchDeleted)},
		{tournamentEventNewInnerTournament, before.NewInnerTournament, after.NewInnerTournament, len(events.NewInnerTournament)},
	} {
		previous, err := tournamentUint256(stream.name+" previous count", stream.before)
		if err != nil {
			return err
		}
		current, err := tournamentUint256(stream.name+" current count", stream.after)
		if err != nil {
			return err
		}
		expected := new(big.Int).Sub(current, previous)
		if expected.Sign() < 0 {
			return fmt.Errorf("%s count decreased from %s to %s", stream.name, previous, current)
		}
		if expected.Cmp(big.NewInt(int64(stream.fetched))) != 0 {
			return fmt.Errorf("%s event count mismatch: expected %s, fetched %d", stream.name, expected, stream.fetched)
		}
	}
	return nil
}

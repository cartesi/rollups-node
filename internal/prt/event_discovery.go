// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"math/big"
	"slices"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

func (a *ITournamentAdapterImpl) findEventBlocks(ctx context.Context, interval TournamentEventQuery) ([]uint64, error) {
	hits := make(map[uint64]struct{})
	onHit := func(block uint64) error {
		hits[block] = struct{}{}
		return nil
	}
	for _, counter := range []struct {
		name           string
		previous, head *big.Int
		read           func(*bind.CallOpts) (*big.Int, error)
	}{
		{tournamentEventCommitmentJoined, interval.Previous.CommitmentJoined, interval.Current.CommitmentJoined,
			a.tournament.GetCommitmentJoinedCount},
		{tournamentEventMatchCreated, interval.Previous.MatchCreated, interval.Current.MatchCreated, a.tournament.GetMatchCreatedCount},
		{tournamentEventMatchAdvanced, interval.Previous.MatchAdvanced, interval.Current.MatchAdvanced, a.tournament.GetMatchAdvancedCount},
		{tournamentEventLeafMatchSealed, interval.Previous.LeafMatchSealed, interval.Current.LeafMatchSealed,
			a.tournament.GetLeafMatchSealedCount},
		{tournamentEventMatchDeleted, interval.Previous.MatchDeleted, interval.Current.MatchDeleted, a.tournament.GetMatchDeletedCount},
		{tournamentEventNewInnerTournament, interval.Previous.NewInnerTournament, interval.Current.NewInnerTournament,
			a.tournament.GetNewInnerTournamentCount},
	} {
		query := func(ctx context.Context, block uint64) (*big.Int, error) {
			return counter.read(&bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)})
		}
		if err := findTournamentTransitions(ctx, interval, counter.previous, counter.head, query, onHit); err != nil {
			return nil, fmt.Errorf("discovering %s events: %w", counter.name, err)
		}
	}
	if interval.FindRecovery {
		query := func(ctx context.Context, block uint64) (*big.Int, error) {
			recovery, err := a.BondRecovery(&bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)})
			if err != nil {
				return nil, err
			}
			if recovery.Disposition == model.BondDispositionRecovered {
				return big.NewInt(1), nil
			}
			return new(big.Int), nil
		}
		if err := findTournamentTransitions(ctx, interval, new(big.Int), big.NewInt(1), query, onHit); err != nil {
			return nil, fmt.Errorf("discovering BondRecovered events: %w", err)
		}
	}
	blocks := slices.Collect(maps.Keys(hits))
	slices.Sort(blocks)
	return blocks, nil
}

// findTournamentTransitions reuses the pinned head value and checks the samples
// actually returned by the provider. It does not establish branch coherence.
func findTournamentTransitions(
	ctx context.Context, interval TournamentEventQuery, previous, head *big.Int,
	query ethutil.TransitionQueryFn, onHit ethutil.OnHitFn,
) error {
	seed, err := tournamentUint256("previous count", previous)
	if err != nil {
		return err
	}
	current, err := tournamentUint256("head count", head)
	if err != nil {
		return err
	}
	if current.Cmp(seed) < 0 {
		return fmt.Errorf("%w: count decreased from %s to %s", ethutil.ErrMonotonicViolation, seed, current)
	}
	if current.Cmp(seed) == 0 {
		return nil
	}
	type sample struct {
		block uint64
		value *big.Int
	}
	var samples []sample
	read := func(ctx context.Context, block uint64) (*big.Int, error) {
		value := current
		if block != interval.End {
			var err error
			value, err = query(ctx, block)
			if err != nil {
				return nil, err
			}
		}
		value, err := tournamentUint256("sample count", value)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample{block, value})
		return value, nil
	}
	if _, err := ethutil.FindTransitions(ctx, interval.Start, interval.End, seed, read, onHit); err != nil {
		return err
	}
	slices.SortFunc(samples, func(one, two sample) int { return cmp.Compare(one.block, two.block) })
	for i := 1; i < len(samples); i++ {
		before, after := samples[i-1], samples[i]
		if before.value.Cmp(after.value) > 0 || before.block == after.block && before.value.Cmp(after.value) != 0 {
			return fmt.Errorf("%w: inconsistent counter samples at blocks %d and %d",
				ethutil.ErrMonotonicViolation, before.block, after.block)
		}
	}
	return nil
}

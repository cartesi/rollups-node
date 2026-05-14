// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
)

// ErrMonotonicViolation is returned by FindTransitions when the seed prevValue
// is greater than the value observed at startBlock. The queried counter is
// assumed monotonic non-decreasing, so a seed that exceeds the on-chain value
// means the caller's previous value is no longer consistent with the chain.
// Callers can match it with errors.Is to distinguish this corruption signal
// from transient query failures.
var ErrMonotonicViolation = errors.New("monotonic assumption violated")

// TransitionQueryFn for binary search
type TransitionQueryFn func(ctx context.Context, block uint64) (*big.Int, error)

type OnHitFn func(block uint64) error

// sortAndCallOnHit sorts the transition blocks and calls onHit for each in chronological order
func sortAndCallOnHit(transitionBlocks []uint64, onHit OnHitFn) (uint64, error) {
	slices.Sort(transitionBlocks)
	for _, block := range transitionBlocks {
		if err := onHit(block); err != nil {
			return 0, err
		}
	}
	return uint64(len(transitionBlocks)), nil
}

// FindTransitions performs divide-and-conquer search for transitions using oracle
// and calls onHit for each transition block in chronological order.
// The prevValue is optional and used to detect a transition exactly at startBlock.
// It assumes the queried value is monotonic (e.g., increasing counter); if it can revert,
// intermediate transitions may be missed when net change is zero.
func FindTransitions(ctx context.Context, startBlock, endBlock uint64, prevValue *big.Int,
	transitionQuery TransitionQueryFn, onHit OnHitFn) (uint64, error) {
	if startBlock > endBlock {
		return 0, nil
	}

	type interval struct {
		StartBlock, EndBlock uint64
		StartValue, EndValue *big.Int
	}

	startValue, err := transitionQuery(ctx, startBlock)
	if err != nil {
		return 0, fmt.Errorf("transitionQuery(startBlock=%d): %w", startBlock, err)
	}

	var transitionBlocks []uint64
	if prevValue != nil {
		comparisonResult := prevValue.Cmp(startValue)
		if comparisonResult > 0 {
			return 0, fmt.Errorf("%w: prevValue %s > startValue %s at block %d",
				ErrMonotonicViolation, prevValue.String(), startValue.String(), startBlock)
		}
		if comparisonResult < 0 {
			transitionBlocks = append(transitionBlocks, startBlock)
		}
	}

	if startBlock == endBlock {
		return sortAndCallOnHit(transitionBlocks, onHit)
	}

	endValue, err := transitionQuery(ctx, endBlock)
	if err != nil {
		return 0, fmt.Errorf("transitionQuery(endBlock=%d): %w", endBlock, err)
	}

	if startValue.Cmp(endValue) == 0 {
		// No further transitions, but may have added startBlock
		return sortAndCallOnHit(transitionBlocks, onHit)
	}

	// First phase: collect all transition blocks
	stack := []interval{{StartBlock: startBlock, EndBlock: endBlock, StartValue: startValue, EndValue: endValue}}
	for len(stack) > 0 {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		// Pop from stack
		iv := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if iv.StartBlock == iv.EndBlock {
			// Found a transition block
			transitionBlocks = append(transitionBlocks, iv.StartBlock)
			continue
		}

		midBlock := iv.StartBlock + (iv.EndBlock-iv.StartBlock)/2 //nolint:mnd
		midValue, err := transitionQuery(ctx, midBlock)
		if err != nil {
			return 0, fmt.Errorf("transitionQuery(midBlock=%d): %w", midBlock, err)
		}

		// Add new intervals to stack if there are transitions
		if midValue.Cmp(iv.EndValue) != 0 {
			stack = append(stack, interval{StartBlock: midBlock + 1, EndBlock: iv.EndBlock, StartValue: midValue, EndValue: iv.EndValue})
		}
		if iv.StartValue.Cmp(midValue) != 0 {
			stack = append(stack, interval{StartBlock: iv.StartBlock, EndBlock: midBlock, StartValue: iv.StartValue, EndValue: midValue})
		}
	}

	// Second phase: sort transition blocks and call onHit in chronological order
	return sortAndCallOnHit(transitionBlocks, onHit)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"errors"
	"fmt"
	"math/big"
)

// ErrMonotonicViolation is returned by FindTransitions when the seed prevValue
// is greater than the value observed at startBlock. The queried counter is
// assumed monotonic non-decreasing, so a seed that exceeds the on-chain value
// means the caller's previous value is no longer consistent with the chain.
// Callers can match it with errors.Is to distinguish this corruption signal
// from transient query failures.
var ErrMonotonicViolation = errors.New("monotonic assumption violated")
var ErrIncompleteProcessing = errors.New("incomplete processing of transitions")

// TransitionQueryFn for binary search
type TransitionQueryFn func(ctx context.Context, block uint64) (*big.Int, error)

type OnHitFn func(block uint64, prevValue, currValue *big.Int) error

// FindTransitions searches for transitions using transitionQuery and calls onHit
// during traversal in chronological order. The optional lowerBound seeds the
// previous counter value to detect a transition exactly at startBlock.
// The optional upperBound stops the search after the transition reaching or
// crossing that counter value; onHit still receives the full counter change.
// It assumes the queried value is monotonic (e.g., increasing counter); if it can revert,
// intermediate transitions may be missed when net change is zero.
//
// The results are a transition count and a completed-block cursor. Callback
// errors, including ErrIncompleteProcessing, are propagated with the cursor set
// to the block before that transition, allowing callers to resume within it.
func FindTransitions(ctx context.Context, startBlock, endBlock uint64, lowerBound, upperBound *big.Int,
	transitionQuery TransitionQueryFn, onHit OnHitFn) (uint64, uint64, error) {

	if startBlock > endBlock {
		return 0, endBlock, nil
	}

	type interval struct {
		StartBlock, EndBlock, LastBlock uint64
		StartValue, EndValue            *big.Int
	}

	startValue, err := transitionQuery(ctx, startBlock)
	if err != nil {
		return 0, startBlock - 1, fmt.Errorf("transitionQuery(startBlock=%d): %w", startBlock, err)
	}

	var count uint64
	if lowerBound != nil {
		comparisonResult := lowerBound.Cmp(startValue)
		if comparisonResult > 0 {
			return 0, startBlock - 1, fmt.Errorf("%w: lowerBound %s > startValue %s at block %d",
				ErrMonotonicViolation, lowerBound.String(), startValue.String(), startBlock)
		}
		if comparisonResult < 0 {
			if err := onHit(startBlock, lowerBound, startValue); err != nil {
				// Resume at this block with the partially consumed counter,
				// rather than querying an older block with a smaller counter.
				return count, startBlock - 1, err
			}
			count++
		}
	}

	if startBlock == endBlock || (upperBound != nil && startValue.Cmp(upperBound) >= 0) {
		return count, startBlock, nil
	}

	endValue, err := transitionQuery(ctx, endBlock)
	if err != nil {
		return count, startBlock - 1, fmt.Errorf("transitionQuery(endBlock=%d): %w", endBlock, err)
	}

	if startValue.Cmp(endValue) == 0 {
		// No further transitions, but may have added startBlock
		return count, endBlock, nil
	}

	lastProcessedBlock := startBlock

	// Visit intervals from left to right, invoking onHit as transitions are found.
	stack := []interval{{
		StartBlock: startBlock,
		EndBlock:   endBlock,
		LastBlock:  endBlock,
		StartValue: startValue,
		EndValue:   endValue,
	}}
	for len(stack) > 0 {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return 0, lastProcessedBlock, ctx.Err()
		default:
		}

		// Pop from stack
		iv := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if iv.StartBlock == iv.EndBlock {
			// Found a transition block
			if err := onHit(iv.StartBlock, iv.StartValue, iv.EndValue); err != nil {
				// Resume at this block with the partially consumed counter,
				// rather than querying an older block with a smaller counter.
				return count, iv.StartBlock - 1, err
			}
			count++
			lastProcessedBlock = iv.LastBlock
			continue
		}

		midBlock := iv.StartBlock + (iv.EndBlock-iv.StartBlock)/2 //nolint:mnd
		midValue, err := transitionQuery(ctx, midBlock)
		if err != nil {
			return 0, lastProcessedBlock, fmt.Errorf("transitionQuery(midBlock=%d): %w", midBlock, err)
		}

		// Add new intervals to stack if there are transitions
		lastBlock := iv.LastBlock
		if midValue.Cmp(iv.EndValue) != 0 {
			if upperBound == nil || midValue.Cmp(upperBound) < 0 {
				stack = append(stack, interval{
					StartBlock: midBlock + 1,
					EndBlock:   iv.EndBlock,
					LastBlock:  lastBlock,
					StartValue: midValue,
					EndValue:   iv.EndValue,
				})
			}
			lastBlock = midBlock
		}
		if iv.StartValue.Cmp(midValue) != 0 {
			stack = append(stack, interval{
				StartBlock: iv.StartBlock,
				EndBlock:   midBlock,
				LastBlock:  lastBlock,
				StartValue: iv.StartValue,
				EndValue:   midValue,
			})
		}
	}

	return count, lastProcessedBlock, nil
}

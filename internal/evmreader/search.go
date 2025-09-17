// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"fmt"
	"math/big"
	"slices"
)

// TransitionQueryFn for binary search
type TransitionQueryFn func(ctx context.Context, block uint64) (*big.Int, error)

type OnHitFn func(block uint64) error

// FindTransitions performs divide-and-conquer search for transitions using oracle
// and calls onHit for each transition block in chronological order
func FindTransitions(ctx context.Context, lo, hi uint64, transitionQuery TransitionQueryFn, onHit OnHitFn) error {
	type interval struct {
		Lo, Hi   uint64
		FLo, FHi *big.Int
	}

	fLo, err := transitionQuery(ctx, lo)
	if err != nil {
		return fmt.Errorf("transitionQuery(lo=%d): %w", lo, err)
	}
	fHi, err := transitionQuery(ctx, hi)
	if err != nil {
		return fmt.Errorf("transitionQuery(hi=%d): %w", hi, err)
	}

	if fLo.Cmp(fHi) == 0 {
		return nil
	}

	// First phase: collect all transition blocks
	var transitionBlocks []uint64
	stack := []interval{{Lo: lo, Hi: hi, FLo: fLo, FHi: fHi}}

	for len(stack) > 0 {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Pop from stack
		iv := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if iv.Lo == iv.Hi {
			// Found a transition block
			transitionBlocks = append(transitionBlocks, iv.Lo)
			continue
		}

		mid := iv.Lo + (iv.Hi-iv.Lo)/2 //nolint: mnd
		fMid, err := transitionQuery(ctx, mid)
		if err != nil {
			return fmt.Errorf("transitionQuery(mid=%d): %w", mid, err)
		}

		// Add new intervals to stack if there are transitions
		if fMid.Cmp(iv.FHi) != 0 {
			stack = append(stack, interval{Lo: mid + 1, Hi: iv.Hi, FLo: fMid, FHi: iv.FHi})
		}
		if iv.FLo.Cmp(fMid) != 0 {
			stack = append(stack, interval{Lo: iv.Lo, Hi: mid, FLo: iv.FLo, FHi: fMid})
		}
	}

	// Second phase: sort transition blocks and call onHit in chronological order
	slices.Sort(transitionBlocks)

	// Call onHit for each transition in chronological order
	for _, block := range transitionBlocks {
		if err := onHit(block); err != nil {
			return err
		}
	}

	return nil
}

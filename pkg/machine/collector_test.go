// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
)

func denseTestEntry(leaves []Hash) Hash {
	level := append([]Hash(nil), leaves...)
	for len(level) > 1 {
		next := make([]Hash, len(level)/2)
		for index := range next {
			next[index] = crypto.Keccak256Hash(level[2*index][:], level[2*index+1][:])
		}
		level = next
	}
	return level[0]
}

// collectTestEntries follows sample pushes and bundle completion for fake backends.
func collectTestEntries(state *HashCollectorState, delta uint64, fixed bool, periodic, final Hash) {
	period := state.MCycleSamplingPeriod
	samples := delta / period
	if fixed {
		samples = 0
		if delta != 0 {
			samples = (delta - 1) / period
		}
	}
	leaves := make([]Hash, samples)
	for index := range leaves {
		leaves[index] = periodic
	}
	if fixed {
		leaves = append(leaves, final)
	}
	width := 1 << state.Log2BundleMCycleCount
	state.Hashes = nil
	state.PartialBundle = nil
	state.MCyclePhase = delta % period
	for start := 0; start < len(leaves); start += width {
		remaining := len(leaves) - start
		if remaining < width && !fixed {
			state.PartialBundle = []byte(fmt.Sprintf(`{"leaf_count":%d}`, remaining))
			break
		}
		bundle := make([]Hash, width)
		for index := range bundle {
			bundle[index] = final
			if index < remaining {
				bundle[index] = leaves[start+index]
			}
		}
		state.Hashes = append(state.Hashes, denseTestEntry(bundle))
	}
	if fixed && width > 1 {
		bundle := make([]Hash, width)
		for index := range bundle {
			bundle[index] = final
		}
		state.Hashes = append(state.Hashes, denseTestEntry(bundle))
	}
}

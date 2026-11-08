// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/base64"
	"encoding/json"

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
			// Each set bit represents a completed subtree in the native carry.
			context := make([]string, 0)
			for bit := 0; 1<<bit <= remaining; bit++ {
				if remaining&(1<<bit) == 0 {
					continue
				}
				subtree := make([]Hash, 1<<bit)
				for index := range subtree {
					subtree[index] = periodic
				}
				root := denseTestEntry(subtree)
				context = append(context, base64.StdEncoding.EncodeToString(root[:]))
			}
			state.PartialBundle, _ = json.Marshal(struct {
				Height    int32    `json:"log2_max_leaves"`
				Function  string   `json:"hash_function"`
				LeafCount int      `json:"leaf_count"`
				Context   []string `json:"context"`
			}{state.Log2BundleMCycleCount, testKeccakHashFunction, remaining, context})
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

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"cmp"
	"slices"

	. "github.com/cartesi/rollups-node/internal/model"
)

// CalculateEpochIndex calculates the epoch index given the input block number
// and epoch length
func CalculateEpochIndex(epochLength uint64, blockNumber uint64) uint64 {
	return blockNumber / epochLength
}

// appsToAddresses
func appsToAddresses(apps []application) []Address {
	var addresses []Address
	for _, app := range apps {
		addresses = append(addresses, app.ContractAddress)
	}
	return addresses
}

// sortByInputIndex is a compare function that orders Inputs
// by index field. It is intended to be used with `insertSorted`, see insertSorted()
func sortByInputIndex(a, b *Input) int {
	return cmp.Compare(a.Index, b.Index)
}

// insertSorted inserts the received input in the slice at the position defined
// by its index property.
func insertSorted[T any](compare func(a, b *T) int, slice []*T, item *T) []*T {
	// Insert Sorted
	i, _ := slices.BinarySearchFunc(
		slice,
		item,
		compare)
	return slices.Insert(slice, i, item)
}

// Index applications given a key extractor function
func indexApps[K comparable](
	keyExtractor func(application) K,
	apps []application,
) map[K][]application {

	result := make(map[K][]application)
	for _, item := range apps {
		key := keyExtractor(item)
		result[key] = append(result[key], item)
	}
	return result
}

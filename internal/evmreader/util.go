// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"cmp"
	"slices"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

// calculateEpochIndex calculates the epoch index given the input block number
// and epoch length
func calculateEpochIndex(epochLength uint64, blockNumber uint64) uint64 {
	return blockNumber / epochLength
}

// appsToAddresses
func appsToAddresses(apps []appContracts) []common.Address {
	var addresses []common.Address
	for _, app := range apps {
		addresses = append(addresses, app.application.IApplicationAddress)
	}
	return addresses
}

func mapAddressToApp(apps []appContracts) map[common.Address]appContracts {
	result := make(map[common.Address]appContracts)
	for _, app := range apps {
		result[app.application.IApplicationAddress] = app
	}
	return result
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
	keyExtractor func(appContracts) K,
	apps []appContracts,
) map[K][]appContracts {

	result := make(map[K][]appContracts)
	for _, item := range apps {
		key := keyExtractor(item)
		result[key] = append(result[key], item)
	}
	return result
}

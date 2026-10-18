// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// A dispatch round pins every binding read to the same canonical block hash.
// Finalized historical observation still uses an explicit numeric block.
type liveBlockContextKey struct{}
type liveBlock struct {
	number uint64
	hash   common.Hash
}

func withLiveBlock(ctx context.Context, number uint64, hash common.Hash) context.Context {
	return context.WithValue(ctx, liveBlockContextKey{}, liveBlock{number, hash})
}

func pinnedCallOpts(ctx context.Context, block uint64) *bind.CallOpts {
	if pinned, ok := ctx.Value(liveBlockContextKey{}).(liveBlock); ok && pinned.number == block {
		return &bind.CallOpts{Context: ctx, BlockHash: pinned.hash}
	}
	return &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
}

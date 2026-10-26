// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"iter"
	"log/slog"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Filter struct {
	MaxChunkSize *big.Int
	Logger       *slog.Logger
}

// ChunkedFilterLogs emits matching logs in order over inclusive block ranges.
// It narrows failed queries until a single block succeeds or returns an error.
func (f *Filter) ChunkedFilterLogs(
	ctx context.Context,
	client *ethclient.Client,
	q ethereum.FilterQuery,
) (
	iter.Seq2[*types.Log, error],
	error,
) {
	if q.FromBlock == nil {
		q.FromBlock = big.NewInt(0)
	}
	if q.ToBlock == nil {
		end, err := client.BlockNumber(ctx)
		if err != nil {
			return nil, err
		}
		q.ToBlock = big.NewInt(0).SetUint64(end)
	}
	maximum := f.MaxChunkSize
	if maximum == nil || maximum.Uint64() == 0 {
		maximum = new(big.Int).SetUint64(math.MaxUint64)
	}

	return func(yield func(log *types.Log, err error) bool) {
		one := big.NewInt(1)
		endBlock := new(big.Int).Set(q.ToBlock)

		delta := new(big.Int).Sub(q.ToBlock, q.FromBlock)

		// split the query
		if delta.Cmp(maximum) >= 0 {
			q.ToBlock.Add(q.FromBlock, maximum)

			// range is inclusive, so remove 1
			// e.g. a chunk size of 500 is: from: 0, to: 499
			q.ToBlock.Sub(q.ToBlock, one)
		}

		for q.FromBlock.Cmp(endBlock) <= 0 {
			f.Logger.Debug("ChunkedFilterLogs for range", "from", q.FromBlock, "to", q.ToBlock, "max", maximum)
			logs, err := client.FilterLogs(ctx, q)
			delta := new(big.Int).Sub(q.ToBlock, q.FromBlock)

			if err != nil {
				if delta.Sign() == 0 || ctx.Err() != nil {
					yield(nil, err)
					return
				}
				f.Logger.Debug("ChunkedFilterLogs narrowing failed range", "from", q.FromBlock, "to", q.ToBlock)
				q.ToBlock.Add(q.FromBlock, new(big.Int).Rsh(delta, 1))
				continue
			}

			for _, log := range logs {
				if !yield(&log, nil) {
					return
				}
			}

			// ------------------------
			//  [  delta  |  delta  ]
			//  ^         ^
			// From       To
			// ------------------------
			//  [  delta  |  delta  ]
			//             ^        ^
			//            From      To
			// ------------------------
			q.FromBlock.Add(q.ToBlock, one)
			q.ToBlock.Add(q.FromBlock, delta)
			if q.ToBlock.Cmp(endBlock) > 0 {
				q.ToBlock.Set(endBlock)
			}
		}
	}, nil
}

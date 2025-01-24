// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"iter"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	minChunk = new(big.Int).SetInt64(64)
)

// read chunkedFilterLogs comment for additional information.
//
// NOTE: There is no standard reply among providers, add as needed. This
// function assumes that any server side error codes represent block range that
// is too large.
// ┌────────────────────────────┬───────┬────────┬────────────┐
// │          provider          │ limit │  code  │ checked at │
// ├────────────────────────────┼───────┼────────┼────────────┤
// │ https://cloudflare-eth.com │   800 │ -32047 │ 2025-01-24 │
// └────────────────────────────┴───────┴────────┴────────────┘
func queryBlockRangeTooLarge(err error) bool {
	if err != nil {
		switch e := err.(type) {
		case rpc.Error:
			return -32099 <= e.ErrorCode() && e.ErrorCode() <= -32000
		}
	}
	return false
}

// chunkedFilterLogs is very similar to LogFilterer FilterLogs. Both functions
// query blockchain events (logs) and return the ones matching the filter
// criteria. In addition to the basic functionality, this version splits large
// (From, To) block ranges into multiple smaller calls when it detects the
// provider rejected the query for this specific reason. Detection is a
// heuristic and implemented in the function queryBlockRangeTooLarge. It
// potentially has to be adjusted to accomodate each provider.
func ChunkedFilterLogs(
	ctx context.Context,
	client simulated.Client,
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

	return func(yield func(log *types.Log, err error) bool) {
		endBlock := new(big.Int).Set(q.ToBlock)
		for q.FromBlock.Cmp(endBlock) < 0 {
			logs, err := client.FilterLogs(ctx, q)
			delta := new(big.Int).Sub(q.ToBlock, q.FromBlock)

			if queryBlockRangeTooLarge(err) {
				if delta.Cmp(minChunk) < 0 {
					yield(nil, err)
					return
				}
				q.ToBlock.Sub(q.ToBlock, delta.Rsh(delta, 1))
				continue
			} else if err != nil {
				yield(nil, err)
				return
			}

			for _, log := range logs {
				if !yield(&log, nil) {
					return
				}
			}

			q.FromBlock.Add(q.FromBlock, delta)
			q.ToBlock.Add(q.ToBlock, delta)
			q.ToBlock.Add(q.ToBlock, delta) // try to 2x the block range back up
			if q.ToBlock.Cmp(endBlock) > 0 {
				q.ToBlock.Set(endBlock)
			}
		}
	}, nil
}

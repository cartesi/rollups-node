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
	//testChunk = new(big.Int).SetInt64(128)
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
func queryBlockRangeTooLarge(err error, _ ethereum.FilterQuery) bool {
	if err != nil {
		switch e := err.(type) {
		case rpc.Error:
			return -32099 <= e.ErrorCode() && e.ErrorCode() <= -32000
		}
	}

	//// debug hook
	//if big.NewInt(0).Sub(q.ToBlock, q.FromBlock).Cmp(testChunk) > 0 {
	//	return true
	//}
	return false
}

// chunkedFilterLogs is very similar to LogFilterer FilterLogs. Both functions
// query blockchain events (logs) and return the ones matching the filter
// criteria.
//
// Note that FilterQuery is inclusive on both ends of its range. This means
// that the results include the values from FromBlock and ToBlock.
//
// In addition to the basic functionality, this version splits large
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
		// nil: FromBlock is genesis
		q.FromBlock = big.NewInt(0)
	}

	if q.ToBlock == nil {
		// nil: ToBlock is latest
		end, err := client.BlockNumber(ctx)
		if err != nil {
			return nil, err
		}
		q.ToBlock = big.NewInt(0).SetUint64(end)
	} else if q.ToBlock.Sign() < 0 {
		// < 0: ToBlock is one of the special rpc cases

		// not the cleanest, but didn't find a way to get the actual block number
		// from the 'rpc.' cases besides this one.
		hdr, err := client.HeaderByNumber(ctx, big.NewInt(int64(q.ToBlock.Int64())))
		if err != nil {
			return nil, err
		}
		q.ToBlock = hdr.Number
	}

	return func(yield func(log *types.Log, err error) bool) {
		plusOne := big.NewInt(1)
		endBlock := new(big.Int).Set(q.ToBlock)
		for q.FromBlock.Cmp(endBlock) < 0 {
			logs, err := client.FilterLogs(ctx, q)
			delta := new(big.Int).Sub(q.ToBlock, q.FromBlock)

			// hack debug
			if queryBlockRangeTooLarge(err, q) {
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

			//fmt.Println(q.FromBlock, q.ToBlock, delta, endBlock)
			q.ToBlock.Add(q.ToBlock, plusOne)
			q.FromBlock.Set(q.ToBlock)
			q.ToBlock.Add(q.ToBlock, delta)

			if q.ToBlock.Cmp(endBlock) > 0 {
				q.ToBlock.Set(endBlock)
			}
		}
	}, nil
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"encoding/json"
	"iter"
	"log/slog"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	DefaultMinChunkSize = new(big.Int).SetInt64(50)
)

type Filter struct {
	MinChunkSize *big.Int
	MaxChunkSize *big.Int
	Logger       *slog.Logger
}

type jsonRPCError struct {
	Version string `json:"jsonrpc"`
	ID      int    `jsonrpc:"id"`
	Error   struct {
		Code    int    `jsonrpc:"code"`
		Message string `jsonrpc:"message"`
	} `jsonrpc:"error"`
}

func unwrapHTTPErrorAsJSON(body []byte) (jsonRPCError, error) {
	err := jsonRPCError{}
	return err, json.Unmarshal(body, &err)
}

// read chunkedFilterLogs comment for additional information.
//
// NOTE: There is no standard reply among providers, add as needed. To handle a
// new provider add it to the table below and make queryBlockRangeTooLarge
// return true when encountering its RPC error code.
// ┌───────────────────────────────────────────────────┬───────┬────────┬────────────┐
// │          provider                                 │ limit │  code  │ checked at │
// ├───────────────────────────────────────────────────┼───────┼────────┼────────────┤
// │ https://cloudflare-eth.com/                       │   800 │ -32047 │ 2025-01-24 │
// │ https://eth-mainnet.g.alchemy.com/v2/{key} (free) │   500 │ -32600 │ 2025-05-13 │
// │ https://mainnet.infura.io/v3/{key} (free)         │ 10000 │ -32005 │ 2025-05-15 │
// │ https://site1.moralis-nodes.com/eth/{key} (free)  │   100 │    400 │ 2025-05-15 │
// └───────────────────────────────────────────────────┴───────┴────────┴────────────┘
func queryBlockRangeTooLargeCode(code int) bool {
	return (code == -32047) || // cloudflare (free)
		(code == -32600) || // alchemy (free)
		(code == -32005) || // infura (free)
		(code == 400) || // moralis (free)
		false
}

func queryBlockRangeTooLarge(err error) bool {
	if err != nil {
		switch e := err.(type) {
		case rpc.Error:
			return queryBlockRangeTooLargeCode(e.ErrorCode())

		// some providers give a HTTP bad request in addition to the jsonrpc error.
		// try to unwrap the json contents and check if the error code is for
		// a block range too large. Otherwise assume the error was something else.
		case rpc.HTTPError:
			json, err := unwrapHTTPErrorAsJSON(e.Body)
			if err != nil {
				return false
			}
			return queryBlockRangeTooLargeCode(json.Error.Code)
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
// potentially has to be adjusted to accommodate each provider.
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
	if f.MaxChunkSize == nil || f.MaxChunkSize.Uint64() == 0 {
		f.MaxChunkSize = new(big.Int).SetUint64(math.MaxUint64)
	}

	return func(yield func(log *types.Log, err error) bool) {
		one := big.NewInt(1)
		endBlock := new(big.Int).Set(q.ToBlock)

		// user defined the split point, use it
		if f.MaxChunkSize != nil {
			delta := new(big.Int).Sub(q.ToBlock, q.FromBlock)

			// split the query
			if delta.Cmp(f.MaxChunkSize) > 0 {
				q.ToBlock.Add(q.FromBlock, f.MaxChunkSize)

				// range is inclusive, so remove 1
				// e.g. a chunk size of 500 is: from: 0, to: 499
				q.ToBlock.Sub(q.ToBlock, one)
			}
		}

		for q.FromBlock.Cmp(endBlock) <= 0 {
			f.Logger.Debug("ChunkedFilterLogs for range", "from", q.FromBlock, "to", q.ToBlock, "max", f.MaxChunkSize)
			logs, err := client.FilterLogs(ctx, q)
			delta := new(big.Int).Sub(q.ToBlock, q.FromBlock)

			if queryBlockRangeTooLarge(err) {
				f.Logger.Debug("ChunkedFilterLogs range is too large", "from", q.FromBlock, "to", q.ToBlock)
				if delta.Cmp(f.MinChunkSize) < 0 {
					yield(nil, err)
					return
				}
				// ToBlock -= delta/2
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

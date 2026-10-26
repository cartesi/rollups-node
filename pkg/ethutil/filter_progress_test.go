// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const twoBlockRange = "0-1"

type progressRangeQuery struct {
	FromBlock *hexutil.Big `json:"fromBlock"`
	ToBlock   *hexutil.Big `json:"toBlock"`
}

const progressRangeLimitMessage = "provider range limit"

type progressRangeLimit struct{}

func (progressRangeLimit) Error() string  { return progressRangeLimitMessage }
func (progressRangeLimit) ErrorCode() int { return -32005 }

type progressFilterRPC struct {
	ranges []string
	fail   func(uint64, uint64) error
}

func (s *progressFilterRPC) GetLogs(_ context.Context, q progressRangeQuery) ([]types.Log, error) {
	from, to := q.FromBlock.ToInt().Uint64(), q.ToBlock.ToInt().Uint64()
	s.ranges = append(s.ranges, fmt.Sprintf("%d-%d", from, to))
	// Bound a regression in the test itself: an unchanged query must fail the
	// semantic assertions rather than hang the entire test process.
	if len(s.ranges) > 8 {
		return nil, errors.New("test RPC request budget exhausted")
	}
	if err := s.fail(from, to); err != nil {
		return nil, err
	}
	return []types.Log{{BlockNumber: from, Topics: []common.Hash{{}}, Data: []byte{}}}, nil
}

func TestChunkedFilterLogsMakesProgressToSingleBlocks(t *testing.T) {
	const singleZeroBlockRange = "0-0"
	for _, scenario := range []struct {
		name, wantError string
		end             int64
		fail            func(uint64, uint64) error
		wantRanges      []string
		wantBlocks      []uint64
	}{
		{name: "single blocks succeed", end: 1,
			fail: func(from, to uint64) error {
				if from != to {
					return progressRangeLimit{}
				}
				return nil
			}, wantRanges: []string{twoBlockRange, singleZeroBlockRange, "1-1"}, wantBlocks: []uint64{0, 1}},
		{name: "single block also rejected", end: 1,
			fail:       func(uint64, uint64) error { return progressRangeLimit{} },
			wantRanges: []string{twoBlockRange, singleZeroBlockRange}, wantError: progressRangeLimitMessage},
		{name: "single block cannot shrink", end: 0,
			fail:       func(uint64, uint64) error { return progressRangeLimit{} },
			wantRanges: []string{singleZeroBlockRange}, wantError: progressRangeLimitMessage},
		{name: "timeout splits to one block", end: 1,
			fail:       func(uint64, uint64) error { return context.DeadlineExceeded },
			wantRanges: []string{twoBlockRange, singleZeroBlockRange}, wantError: context.DeadlineExceeded.Error()},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			api := &progressFilterRPC{fail: scenario.fail}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", api))
			defer server.Stop()
			client := ethclient.NewClient(rpc.DialInProc(server))
			defer client.Close()
			filter := Filter{MaxChunkSize: big.NewInt(4),
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			logs, err := filter.ChunkedFilterLogs(t.Context(), client,
				ethereum.FilterQuery{FromBlock: new(big.Int), ToBlock: big.NewInt(scenario.end)})
			require.NoError(t, err)
			var blocks []uint64
			for log, readErr := range logs {
				if readErr != nil {
					err = readErr
					break
				}
				blocks = append(blocks, log.BlockNumber)
			}
			if scenario.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, scenario.wantError)
			}
			require.Equal(t, scenario.wantRanges, api.ranges)
			require.Equal(t, scenario.wantBlocks, blocks)
		})
	}
}

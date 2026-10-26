// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type rangeRPCError int

func (e rangeRPCError) Error() string  { return "query refused" }
func (e rangeRPCError) ErrorCode() int { return int(e) }

type logRangeRPC struct {
	mu      sync.Mutex
	queries [][2]uint64
	fail    func(context.Context, uint64, uint64) error
	hasLogs func(uint64) bool
	stop    context.CancelFunc
}

func (s *logRangeRPC) GetLogs(ctx context.Context, query progressRangeQuery) ([]types.Log, error) {
	from, to := query.FromBlock.ToInt().Uint64(), query.ToBlock.ToInt().Uint64()
	s.mu.Lock()
	s.queries = append(s.queries, [2]uint64{from, to})
	overBudget := len(s.queries) > 64
	s.mu.Unlock()
	if overBudget {
		s.stop()
		return nil, errors.New("test RPC request budget exhausted")
	}
	if s.fail != nil {
		if err := s.fail(ctx, from, to); err != nil {
			return nil, err
		}
	}
	var logs []types.Log
	for block := from; ; block++ {
		for index := range uint(2) {
			if s.hasLogs != nil && !s.hasLogs(block) {
				continue
			}
			logs = append(logs, types.Log{BlockNumber: block, Index: index,
				Topics: []common.Hash{{}}, Data: []byte{byte(index)}})
		}
		if block == to {
			return logs, nil
		}
	}
}

func (s *logRangeRPC) requestedRanges() [][2]uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][2]uint64(nil), s.queries...)
}

func runLogRangeRPC(
	t *testing.T, api *logRangeRPC, from, to uint64, maximum *big.Int, timeout time.Duration,
) ([]types.Log, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	api.stop = cancel
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", api))
	defer server.Stop()
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	rpcClient, err := rpc.DialOptions(ctx, httpServer.URL, rpc.WithHTTPClient(&http.Client{Timeout: timeout}))
	require.NoError(t, err)
	client := ethclient.NewClient(rpcClient)
	defer client.Close()
	filter := Filter{MaxChunkSize: maximum, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	stream, err := filter.ChunkedFilterLogs(ctx, client,
		ethereum.FilterQuery{FromBlock: new(big.Int).SetUint64(from), ToBlock: new(big.Int).SetUint64(to)})
	require.NoError(t, err)
	var logs []types.Log
	for log, err := range stream {
		if err != nil {
			return logs, err
		}
		logs = append(logs, *log)
	}
	return logs, nil
}

func requireLogRangeCoverage(t *testing.T, logs []types.Log, first, last uint64) {
	t.Helper()
	require.Equal(t, 2*(last-first+1), uint64(len(logs)))
	for index, log := range logs {
		require.Equal(t, first+uint64(index/2), log.BlockNumber)
		require.Equal(t, uint(index%2), log.Index)
		require.Equal(t, []byte{byte(index % 2)}, log.Data)
	}
}

func TestChunkedFilterLogsSplitsEveryRPCError(t *testing.T) {
	for _, code := range []rangeRPCError{-32047, -32600, -32005, 400, -32602, -32616, -32099} {
		t.Run(big.NewInt(int64(code)).String(), func(t *testing.T) {
			api := &logRangeRPC{fail: func(_ context.Context, from, to uint64) error {
				if from != to {
					return code
				}
				return nil
			}}
			logs, err := runLogRangeRPC(t, api, 0, 4, nil, 0)
			require.NoError(t, err)
			require.Equal(t, [][2]uint64{{0, 4}, {0, 2}, {0, 1}, {0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}},
				api.requestedRanges())
			requireLogRangeCoverage(t, logs, 0, 4)
		})
	}
}

func TestChunkedFilterLogsStopsAtRefusedSingleBlock(t *testing.T) {
	api := &logRangeRPC{fail: func(context.Context, uint64, uint64) error { return rangeRPCError(-32616) }}
	logs, err := runLogRangeRPC(t, api, 0, 39, nil, 0)
	require.ErrorContains(t, err, "query refused")
	require.Empty(t, logs)
	require.Equal(t, [][2]uint64{{0, 39}, {0, 19}, {0, 9}, {0, 4}, {0, 2}, {0, 1}, {0, 0}}, api.requestedRanges())
}

func TestChunkedFilterLogsPreservesPrefixBeforeSingleBlockError(t *testing.T) {
	api := &logRangeRPC{fail: func(_ context.Context, from, to uint64) error {
		if from != to || from == 2 {
			return rangeRPCError(-32602)
		}
		return nil
	}}
	logs, err := runLogRangeRPC(t, api, 0, 3, nil, 0)
	require.ErrorContains(t, err, "query refused")
	requireLogRangeCoverage(t, logs, 0, 1)
	require.Equal(t, [][2]uint64{{0, 3}, {0, 1}, {0, 0}, {1, 1}, {2, 2}}, api.requestedRanges())
}

func TestChunkedFilterLogsSplitsTransportTimeout(t *testing.T) {
	api := &logRangeRPC{fail: func(ctx context.Context, from, to uint64) error {
		if from == to {
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	logs, err := runLogRangeRPC(t, api, 0, 1, nil, 30*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, [][2]uint64{{0, 1}, {0, 0}, {1, 1}}, api.requestedRanges())
	requireLogRangeCoverage(t, logs, 0, 1)
}

func TestChunkedFilterLogsHonorsInclusiveMaximum(t *testing.T) {
	for _, test := range []struct {
		name     string
		from, to uint64
		maximum  *big.Int
		want     [][2]uint64
	}{
		{name: "one block", to: 0, maximum: big.NewInt(4), want: [][2]uint64{{0, 0}}},
		{name: "within cap", to: 2, maximum: big.NewInt(4), want: [][2]uint64{{0, 2}}},
		{name: "exact cap", to: 3, maximum: big.NewInt(4), want: [][2]uint64{{0, 3}}},
		{name: "one above cap", to: 4, maximum: big.NewInt(4), want: [][2]uint64{{0, 3}, {4, 4}}},
		{name: "even count", to: 5, maximum: big.NewInt(4), want: [][2]uint64{{0, 3}, {4, 5}}},
		{name: "single block cap", to: 2, maximum: big.NewInt(1), want: [][2]uint64{{0, 0}, {1, 1}, {2, 2}}},
		{name: "no cap", to: 4, want: [][2]uint64{{0, 4}}},
		{name: "zero cap", to: 4, maximum: new(big.Int), want: [][2]uint64{{0, 4}}},
		{name: "uint64 end", from: ^uint64(0) - 4, to: ^uint64(0), maximum: big.NewInt(4),
			want: [][2]uint64{{^uint64(0) - 4, ^uint64(0) - 1}, {^uint64(0), ^uint64(0)}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &logRangeRPC{}
			logs, err := runLogRangeRPC(t, api, test.from, test.to, test.maximum, 0)
			require.NoError(t, err)
			require.Equal(t, test.want, api.requestedRanges())
			requireLogRangeCoverage(t, logs, test.from, test.to)
		})
	}
}

func TestChunkedFilterLogsReturnsCancellation(t *testing.T) {
	api := &logRangeRPC{}
	api.fail = func(context.Context, uint64, uint64) error {
		api.stop()
		return context.Canceled
	}
	logs, err := runLogRangeRPC(t, api, 0, 4, nil, 0)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, logs)
	require.Equal(t, [][2]uint64{{0, 4}}, api.requestedRanges())
}

func TestChunkedFilterLogsPreservesSparseAndEmptyResults(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "sparse", true: "empty"}[empty], func(t *testing.T) {
			api := &logRangeRPC{hasLogs: func(block uint64) bool { return !empty && block%2 == 1 }}
			api.fail = func(_ context.Context, from, to uint64) error {
				if to-from+1 > 2 {
					return rangeRPCError(-32616)
				}
				return nil
			}
			logs, err := runLogRangeRPC(t, api, 0, 4, nil, 0)
			require.NoError(t, err)
			require.Equal(t, [][2]uint64{{0, 4}, {0, 2}, {0, 1}, {2, 3}, {4, 4}}, api.requestedRanges())
			if empty {
				require.Empty(t, logs)
				return
			}
			require.Len(t, logs, 4)
			for index, log := range logs {
				require.Equal(t, uint64(1+2*(index/2)), log.BlockNumber)
				require.Equal(t, uint(index%2), log.Index)
			}
		})
	}
}

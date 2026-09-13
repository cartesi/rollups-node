// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package matchadvances

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/service"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestMatchAdvancesCommandRequiresScopeAndCompleteEventIdentity(t *testing.T) {
	for count := range 8 {
		err := Cmd.Args(Cmd, make([]string, count))
		if count == 4 || count == 6 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	require.Contains(t, Cmd.Use, "transaction hash log index")
	require.NotContains(t, Cmd.Use, "parent")
	require.NotContains(t, Cmd.Example, "[parent]")
}

func TestMatchAdvancesGetExampleHasValidEventIdentity(t *testing.T) {
	_, example, found := strings.Cut(Cmd.Example, "cartesi-rollups-cli read match_advances ")
	require.True(t, found)
	line, _, _ := strings.Cut(example, "\n")
	args := strings.Fields(line)
	require.Len(t, args, getArgCount)
	require.NoError(t, Cmd.Args(Cmd, args))
	_, err := config.ToHashFromString(args[listArgCount])
	require.NoError(t, err)
	_, err = config.AsHexString(args[getArgCount-1])
	require.NoError(t, err)
}

type matchAdvanceReader struct {
	service.ReadService
	get  func(context.Context, api.GetMatchAdvanceParams) (json.RawMessage, error)
	list func(context.Context, api.ListMatchAdvancesParams) (json.RawMessage, error)
}

func (reader *matchAdvanceReader) GetMatchAdvanced(ctx context.Context, params api.GetMatchAdvanceParams) (json.RawMessage, error) {
	return reader.get(ctx, params)
}

func (reader *matchAdvanceReader) ListMatchAdvances(ctx context.Context, params api.ListMatchAdvancesParams) (json.RawMessage, error) {
	return reader.list(ctx, params)
}

func matchAdvanceCommandArgs() []string {
	return []string{"echo-dapp", "16", common.HexToAddress("0xabc").Hex(), common.HexToHash("0xdef").Hex()}
}

func TestMatchAdvancesCommandMapsGetEventIdentity(t *testing.T) {
	for _, logIndex := range []string{"0", "0X2A"} {
		t.Run(logIndex, func(t *testing.T) {
			args := append(matchAdvanceCommandArgs(), common.HexToHash("0x123").Hex(), logIndex)
			require.NoError(t, Cmd.Args(Cmd, args))
			expectedIndex := "0x0"
			if logIndex == "0X2A" {
				expectedIndex = "0x2a"
			}
			called := false
			reader := &matchAdvanceReader{get: func(ctx context.Context, params api.GetMatchAdvanceParams) (json.RawMessage, error) {
				called = true
				require.Equal(t, t.Context(), ctx)
				require.Equal(t, api.GetMatchAdvanceParams{Application: args[0], EpochIndex: "0x10",
					TournamentAddress: args[2], IDHash: args[3], TxHash: args[4], LogIndex: expectedIndex}, params)
				return json.RawMessage(`{"data":{}}`), nil
			}}
			result, err := readMatchAdvances(t.Context(), reader, args)
			require.NoError(t, err)
			require.True(t, called)
			require.JSONEq(t, `{"data":{}}`, string(result))
		})
	}
}

func TestMatchAdvancesCommandKeepsListPaginationFlags(t *testing.T) {
	previousLimit, previousOffset, previousDescending := limit, offset, descending
	t.Cleanup(func() { limit, offset, descending = previousLimit, previousOffset, previousDescending })
	require.NoError(t, Cmd.ParseFlags([]string{"--limit", "3", "--offset", "2", "--descending"}))
	args := matchAdvanceCommandArgs()
	require.NoError(t, Cmd.Args(Cmd, args))
	require.NoError(t, Cmd.PreRunE(Cmd, args))
	called := false
	reader := &matchAdvanceReader{list: func(_ context.Context, params api.ListMatchAdvancesParams) (json.RawMessage, error) {
		called = true
		require.Equal(t, api.ListMatchAdvancesParams{Application: args[0], EpochIndex: "0x10",
			TournamentAddress: args[2], IDHash: args[3], Limit: 3, Offset: 2, Descending: true}, params)
		return json.RawMessage(`{"data":[]}`), nil
	}}
	_, err := readMatchAdvances(t.Context(), reader, args)
	require.NoError(t, err)
	require.True(t, called)
	limit = 0
	require.NoError(t, Cmd.PreRunE(Cmd, args))
	require.Equal(t, uint64(jsonrpc.LIST_ITEM_LIMIT), limit)
	limit = jsonrpc.LIST_ITEM_LIMIT + 1
	require.Error(t, Cmd.PreRunE(Cmd, args))
}

func TestMatchAdvancesCommandRejectsInvalidNumericIndices(t *testing.T) {
	for _, epochIndex := range []string{"-1", "0x10000000000000000"} {
		args := matchAdvanceCommandArgs()
		args[1] = epochIndex
		_, err := readMatchAdvances(t.Context(), nil, args)
		require.Error(t, err)
	}
	for _, logIndex := range []string{"-1", "0x10000000000000000"} {
		args := append(matchAdvanceCommandArgs(), common.HexToHash("0x123").Hex(), logIndex)
		_, err := readMatchAdvances(t.Context(), nil, args)
		require.Error(t, err)
	}
}

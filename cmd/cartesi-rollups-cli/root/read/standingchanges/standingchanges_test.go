// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package standingchanges

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

func TestStandingChangesCommandRequiresScopeAndCompleteEventIdentity(t *testing.T) {
	for count := range 8 {
		err := Cmd.Args(Cmd, make([]string, count))
		if count == 3 || count == 5 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestStandingChangesGetExampleHasValidEventIdentity(t *testing.T) {
	_, example, found := strings.Cut(Cmd.Example, "cartesi-rollups-cli read standing_changes ")
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

type standingChangeReader struct {
	service.ReadService
	get  func(context.Context, api.GetStandingChangeParams) (json.RawMessage, error)
	list func(context.Context, api.ListStandingChangesParams) (json.RawMessage, error)
}

func (reader *standingChangeReader) GetStandingChange(ctx context.Context, params api.GetStandingChangeParams) (json.RawMessage, error) {
	return reader.get(ctx, params)
}

func (reader *standingChangeReader) ListStandingChanges(
	ctx context.Context, params api.ListStandingChangesParams,
) (json.RawMessage, error) {
	return reader.list(ctx, params)
}

func standingChangeCommandArgs() []string {
	return []string{"echo-dapp", "16", common.HexToAddress("0xabc").Hex()}
}

func TestStandingChangesCommandMapsGetEventIdentity(t *testing.T) {
	for _, test := range []struct {
		epoch, logIndex, wantEpoch, wantLog string
	}{
		{epoch: "16", logIndex: "0", wantEpoch: "0x10", wantLog: "0x0"},
		{epoch: "18446744073709551615", logIndex: "0XFFFFFFFFFFFFFFFF",
			wantEpoch: "0xffffffffffffffff", wantLog: "0xffffffffffffffff"},
	} {
		t.Run(test.logIndex, func(t *testing.T) {
			args := []string{"echo-dapp", test.epoch, common.HexToAddress("0xabc").Hex(),
				common.HexToHash("0x123").Hex(), test.logIndex}
			require.NoError(t, Cmd.Args(Cmd, args))
			reader := &standingChangeReader{get: func(_ context.Context, params api.GetStandingChangeParams) (json.RawMessage, error) {
				require.Equal(t, api.GetStandingChangeParams{Application: args[0], EpochIndex: test.wantEpoch,
					TournamentAddress: args[2], TxHash: args[3], LogIndex: test.wantLog}, params)
				return json.RawMessage(`{"data":{}}`), nil
			}}
			result, err := readStandingChanges(t.Context(), reader, args)
			require.NoError(t, err)
			require.JSONEq(t, `{"data":{}}`, string(result))
		})
	}
}

func TestStandingChangesCommandKeepsListPaginationFlags(t *testing.T) {
	previousLimit, previousOffset, previousDescending := limit, offset, descending
	t.Cleanup(func() { limit, offset, descending = previousLimit, previousOffset, previousDescending })
	require.NoError(t, Cmd.ParseFlags([]string{"--limit", "3", "--offset", "2", "--descending"}))
	args := standingChangeCommandArgs()
	require.NoError(t, Cmd.Args(Cmd, args))
	require.NoError(t, Cmd.PreRunE(Cmd, args))
	called := false
	reader := &standingChangeReader{list: func(_ context.Context, params api.ListStandingChangesParams) (json.RawMessage, error) {
		called = true
		require.Equal(t, api.ListStandingChangesParams{Application: args[0], EpochIndex: "0x10",
			TournamentAddress: args[2], Limit: 3, Offset: 2, Descending: true}, params)
		return json.RawMessage(`{"data":[]}`), nil
	}}
	_, err := readStandingChanges(t.Context(), reader, args)
	require.NoError(t, err)
	require.True(t, called)
	limit = 0
	require.NoError(t, Cmd.PreRunE(Cmd, args))
	require.Equal(t, uint64(jsonrpc.LIST_ITEM_LIMIT), limit)
	limit = jsonrpc.LIST_ITEM_LIMIT + 1
	require.Error(t, Cmd.PreRunE(Cmd, args))
}

func TestStandingChangesCommandRejectsInvalidNumericIndices(t *testing.T) {
	for _, epochIndex := range []string{"-1", "0x10000000000000000"} {
		args := standingChangeCommandArgs()
		args[1] = epochIndex
		_, err := readStandingChanges(t.Context(), nil, args)
		require.Error(t, err)
	}
	for _, logIndex := range []string{"-1", "0x10000000000000000"} {
		args := append(standingChangeCommandArgs(), common.HexToHash("0x123").Hex(), logIndex)
		_, err := readStandingChanges(t.Context(), nil, args)
		require.Error(t, err)
	}
}

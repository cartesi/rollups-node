// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
)

const getSentryByIDMethod = "getSentryById"

type sentryInspectionRPC struct{ mock.Mock }

func (m *sentryInspectionRPC) GetCode(_ context.Context, address common.Address, block rpc.BlockNumber) hexutil.Bytes {
	args := m.Called(address, block)
	return args.Get(0).([]byte)
}

func (m *sentryInspectionRPC) Call(
	_ context.Context, call map[string]json.RawMessage, block rpc.BlockNumber,
) (hexutil.Bytes, error) {
	var address common.Address
	var input hexutil.Bytes
	if err := json.Unmarshal(call["to"], &address); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(call["input"], &input); err != nil {
		return nil, err
	}
	args := m.Called(address, []byte(input), block)
	return args.Get(0).([]byte), args.Error(1)
}

type sentryInspectionFixture struct {
	manager    common.Address
	sentries   []common.Address
	count      *big.Int
	failMethod string
	failID     uint64
}

func newSentryInspectionClient(t *testing.T, fixture sentryInspectionFixture) (*chainClient, common.Address) {
	t.Helper()
	contractABI, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	require.NoError(t, err)
	address := common.HexToAddress("0x100")
	block := rpc.BlockNumber(1234)
	backend := &sentryInspectionRPC{}
	backend.On("GetCode", address, block).Return([]byte{0x01}).Once()
	expectCall := func(name string, inputs, outputs []any) {
		t.Helper()
		input, packErr := contractABI.Pack(name, inputs...)
		require.NoError(t, packErr)
		var encoded []byte
		var callErr error
		if name == fixture.failMethod && (name != getSentryByIDMethod || inputs[0].(*big.Int).Uint64() == fixture.failID) {
			callErr = errors.New("RPC read failed")
		} else {
			encoded, packErr = contractABI.Methods[name].Outputs.Pack(outputs...)
			require.NoError(t, packErr)
		}
		backend.On("Call", address, input, block).Return(encoded, callErr).Once()
	}
	if fixture.count == nil {
		fixture.count = big.NewInt(int64(len(fixture.sentries)))
	}
	for _, call := range []struct {
		name   string
		values []any
	}{
		{"canStageTournamentResult", []any{false, false, false, big.NewInt(1), [32]byte{}, [32]byte{}}},
		{"getCurrentSealedEpoch", []any{
			big.NewInt(1), big.NewInt(2), big.NewInt(3), common.HexToAddress("0x200"), false, big.NewInt(0), [32]byte{}, [32]byte{},
		}},
		{"getDeploymentBlockNumber", []any{big.NewInt(100)}},
		{"getInputBox", []any{common.HexToAddress("0x300")}},
		{"getTournamentFactory", []any{common.HexToAddress("0x400")}},
		{"getClaimStagingPeriod", []any{big.NewInt(300)}},
		{"getSentryManager", []any{fixture.manager}},
		{"getNumberOfSentries", []any{fixture.count}},
	} {
		expectCall(call.name, nil, call.values)
		if call.name == fixture.failMethod {
			break
		}
	}
	if fixture.failMethod == "" || fixture.failMethod == getSentryByIDMethod {
		for index, sentry := range fixture.sentries {
			id := uint64(index + 1)
			expectCall(getSentryByIDMethod, []any{new(big.Int).SetUint64(id)}, []any{sentry})
			if sentry == (common.Address{}) || (fixture.failMethod == getSentryByIDMethod && fixture.failID == id) {
				break
			}
		}
	}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	t.Cleanup(func() { backend.AssertExpectations(t) })
	return &chainClient{
		eth:      client,
		callOpts: &bind.CallOpts{Context: t.Context(), BlockNumber: big.NewInt(int64(block))},
		blockNum: uint64(block),
		chainID:  31337,
	}, address
}

func TestQueryDaveSentriesAtPinnedBlock(t *testing.T) {
	manager := common.HexToAddress("0x500")
	first := common.HexToAddress("0x501")
	second := common.HexToAddress("0x502")
	rotated := common.HexToAddress("0x503")
	for _, test := range []struct {
		name     string
		manager  common.Address
		sentries []common.Address
	}{
		{name: "no sentries"},
		{name: "two sentries", manager: manager, sentries: []common.Address{first, second}},
		{name: "rotated first slot", manager: manager, sentries: []common.Address{rotated, second}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, address := newSentryInspectionClient(t, sentryInspectionFixture{manager: test.manager, sentries: test.sentries})
			result, err := client.queryDave(address)
			require.NoError(t, err)
			require.Equal(t, test.manager.Hex(), result.SentryManager)
			require.EqualValues(t, len(test.sentries), result.NumSentries)
			require.NotNil(t, result.Sentries)
			require.Len(t, result.Sentries, len(test.sentries))
			for index, sentry := range test.sentries {
				require.Equal(t, SentryResult{ID: uint64(index + 1), Address: sentry.Hex()}, result.Sentries[index])
			}
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &fields))
			require.JSONEq(t, fmt.Sprintf("%q", test.manager.Hex()), string(fields["sentry_manager"]))
			require.Equal(t, fmt.Sprint(len(test.sentries)), string(fields["num_sentries"]))
			if len(test.sentries) == 0 {
				require.Equal(t, "[]", string(fields["sentries"]))
			} else {
				var sentries []SentryResult
				require.NoError(t, json.Unmarshal(fields["sentries"], &sentries))
				require.Equal(t, result.Sentries, sentries)
			}
			var text bytes.Buffer
			printConsensusSummary(&printer{w: &text}, consensusResult{result: result})
			require.Contains(t, text.String(), "Sentry Manager          "+test.manager.Hex())
			require.Contains(t, text.String(), "Sentries                "+fmt.Sprint(len(test.sentries)))
			for index, sentry := range test.sentries {
				require.Contains(t, text.String(), fmt.Sprintf("  Sentry #%d             %s", index+1, sentry.Hex()))
			}
		})
	}
}

func TestQueryDaveRejectsIncompleteSentryRoster(t *testing.T) {
	for _, test := range []struct {
		name    string
		fixture sentryInspectionFixture
		wantErr string
	}{
		{name: "manager read fails", fixture: sentryInspectionFixture{failMethod: "getSentryManager"}, wantErr: "GetSentryManager"},
		{name: "count read fails", fixture: sentryInspectionFixture{failMethod: "getNumberOfSentries"}, wantErr: "GetNumberOfSentries"},
		{
			name: "second member read fails",
			fixture: sentryInspectionFixture{
				sentries:   []common.Address{common.HexToAddress("0x501"), common.HexToAddress("0x502")},
				failMethod: getSentryByIDMethod, failID: 2,
			},
			wantErr: "GetSentryById(2)",
		},
		{
			name:    "missing member",
			fixture: sentryInspectionFixture{sentries: []common.Address{common.HexToAddress("0x501"), {}}},
			wantErr: "GetSentryById(2): registered sentry has zero address",
		},
		{
			name:    "count exceeds inspection limit",
			fixture: sentryInspectionFixture{count: big.NewInt(10001)},
			wantErr: "number of sentries 10001 exceeds inspection limit 10000",
		},
		{
			name:    "count exceeds uint64",
			fixture: sentryInspectionFixture{count: new(big.Int).Lsh(big.NewInt(1), 64)},
			wantErr: "number of sentries",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, address := newSentryInspectionClient(t, test.fixture)
			result, err := client.queryDave(address)
			require.ErrorContains(t, err, test.wantErr)
			require.Nil(t, result, "a failed member read must not expose a partial roster")
		})
	}
}

func TestPrintDaveIncludesSentryRoster(t *testing.T) {
	previousJSON := jsonParam
	t.Cleanup(func() { jsonParam = previousJSON })
	manager := common.HexToAddress("0x500")
	sentry := common.HexToAddress("0x501")
	for _, asJSON := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%t", asJSON), func(t *testing.T) {
			client, address := newSentryInspectionClient(t, sentryInspectionFixture{manager: manager, sentries: []common.Address{sentry}})
			jsonParam = asJSON
			output := captureSentryInspectionOutput(t, func() { require.NoError(t, client.printDave(address)) })
			if asJSON {
				var result DaveConsensusResult
				require.NoError(t, json.Unmarshal([]byte(output), &result))
				require.Equal(t, manager.Hex(), result.SentryManager)
				require.EqualValues(t, 1, result.NumSentries)
				require.Equal(t, []SentryResult{{ID: 1, Address: sentry.Hex()}}, result.Sentries)
				return
			}
			require.Contains(t, output, "Sentry Manager          "+manager.Hex())
			require.Contains(t, output, "Sentries                1")
			require.Contains(t, output, "  Sentry #1             "+sentry.Hex())
		})
	}
}

func captureSentryInspectionOutput(t *testing.T, fn func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	defer file.Close()
	previous := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = previous }()
	fn()
	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)
	output, err := io.ReadAll(file)
	require.NoError(t, err)
	return string(output)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package refund

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
)

const (
	testApplication = "0x0000000000000000000000000000000000000001"
	testChainID     = 31337
	testGasLimit    = 90000
	inputFileFlag   = "--input-file"
	yesFlag         = "--yes"
	jsonFlag        = "--json"

	maxUint256Decimal = "115792089237316195423570985008687907853269984665640564039457584007913129639935"
)

func TestParseInputIndex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0", "0"},
		{"0x0", "0"},
		{"007", "7"},
		{"0xFF", "255"},
		{"18446744073709551616", "18446744073709551616"},
		{maxUint256Decimal, maxUint256Decimal},
		{"0x" + strings.Repeat("f", 64), maxUint256Decimal},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			index, err := parseInputIndex(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, index.String())
		})
	}
	for _, input := range []string{
		"", "-1", "+1", "-0", "0x", "0x-1", "0x+1", "0xgg", "0b10", "1_0", " 7", "7 ",
		"0x1" + strings.Repeat("0", 64),
		"115792089237316195423570985008687907853269984665640564039457584007913129639936",
	} {
		t.Run("reject_"+input, func(t *testing.T) {
			index, err := parseInputIndex(input)
			require.ErrorContains(t, err, "invalid input index")
			require.Nil(t, index)
		})
	}
}

func writeInputFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.hex")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func originalInput(t *testing.T) []byte {
	t.Helper()
	inputABI, err := inputs.InputsMetaData.GetAbi()
	require.NoError(t, err)
	input, err := inputABI.Pack("EvmAdvance", big.NewInt(testChainID), common.HexToAddress(testApplication),
		common.HexToAddress("0x2"), big.NewInt(10), big.NewInt(20), big.NewInt(0), big.NewInt(7), []byte{0xaa, 0xbb})
	require.NoError(t, err)
	return input
}

func TestLoadInput(t *testing.T) {
	input := originalInput(t)
	loaded, err := loadInput(writeInputFile(t, " \t\n"+hexutil.Encode(input)+"\r\n "))
	require.NoError(t, err)
	require.Equal(t, input, loaded)
	for _, contents := range []string{"", " \n", "0x", "aabb", "0Xaa", "0xabc", "0xgg", "0xaa bb", `"0xaa"`, `{}`} {
		t.Run("reject_"+contents, func(t *testing.T) {
			loaded, err := loadInput(writeInputFile(t, contents))
			require.ErrorContains(t, err, "invalid input file")
			require.Nil(t, loaded)
		})
	}
	_, err = loadInput(filepath.Join(t.TempDir(), "missing.hex"))
	require.ErrorContains(t, err, "read input file")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCommandRejectsArgumentsBeforeEthereum(t *testing.T) {
	t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, "invalid-endpoint")
	validFile := writeInputFile(t, hexutil.Encode(originalInput(t)))
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing arguments", nil, "accepts 2 arg(s)"},
		{"missing file flag", []string{testApplication, "7"}, `required flag(s) "input-file" not set`},
		{"invalid application", []string{"0x1234", "7", inputFileFlag, validFile}, "invalid Ethereum address"},
		{"invalid index", []string{testApplication, "bad", inputFileFlag, validFile}, "invalid input index"},
		{"missing file", []string{testApplication, "7", inputFileFlag, filepath.Join(t.TempDir(), "missing")}, "read input file"},
		{"invalid file", []string{testApplication, "7", inputFileFlag, writeInputFile(t, "{}")}, "invalid input file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newCommand()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(append([]string{}, tt.args...))
			require.ErrorContains(t, cmd.ExecuteContext(t.Context()), tt.want)
		})
	}
}

// refundRPC serves only the Ethereum methods used by the real generated binding.
// It never connects to a chain or sends a transaction outside this test server.
type refundRPC struct {
	estimateErr error
	sendErr     error
	sent        chan *types.Transaction
	estimates   atomic.Int32
	receipts    atomic.Int32
	failed      bool
	pending     bool
	logs        []*types.Log
	emptyCode   bool
}

func (*refundRPC) ChainId(context.Context) (*hexutil.Big, error) { //nolint:revive // RPC method eth_chainId.
	return (*hexutil.Big)(big.NewInt(testChainID)), nil
}

func (*refundRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), GasLimit: testGasLimit}, nil
}

func (*refundRPC) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1)), nil
}

func (r *refundRPC) GetCode(context.Context, common.Address, string) (hexutil.Bytes, error) {
	if r.emptyCode {
		return hexutil.Bytes{}, nil
	}
	return hexutil.Bytes{0x01}, nil
}

func (r *refundRPC) EstimateGas(context.Context, map[string]json.RawMessage) (hexutil.Uint64, error) {
	r.estimates.Add(1)
	return testGasLimit, r.estimateErr
}

func (r *refundRPC) GetTransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	r.receipts.Add(1)
	if r.pending {
		return nil, nil
	}
	status := uint64(types.ReceiptStatusSuccessful)
	if r.failed {
		status = types.ReceiptStatusFailed
	}
	return &types.Receipt{
		TxHash: hash, Status: status, BlockNumber: big.NewInt(10),
		Logs: r.logs, GasUsed: testGasLimit, CumulativeGasUsed: testGasLimit,
	}, nil
}

func (*refundRPC) GetTransactionCount(context.Context, common.Address, string) (hexutil.Uint64, error) {
	return 0, nil
}

func (r *refundRPC) SendRawTransaction(_ context.Context, raw hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	r.sent <- &tx
	return tx.Hash(), r.sendErr
}

type refundRevert struct {
	data string
}

func (*refundRevert) Error() string    { return "execution reverted" }
func (*refundRevert) ErrorCode() int   { return 3 }
func (e *refundRevert) ErrorData() any { return e.data }

func setupRPC(t *testing.T, service *refundRPC) common.Address {
	t.Helper()
	if service.logs == nil {
		service.logs = []*types.Log{refundEventLog(t, common.HexToAddress(testApplication), big.NewInt(7), originalInput(t))}
	}
	rpcServer := rpc.NewServer()
	require.NoError(t, rpcServer.RegisterName("eth", service))
	server := httptest.NewServer(rpcServer)
	t.Cleanup(server.Close)
	t.Cleanup(rpcServer.Stop)
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, server.URL)
	t.Setenv(config.AUTH_KIND, "private_key")
	t.Setenv(config.AUTH_PRIVATE_KEY, hexutil.Encode(crypto.FromECDSA(key)))
	t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "0")
	return crypto.PubkeyToAddress(key.PublicKey)
}

func refundEventLog(t *testing.T, address common.Address, index *big.Int, input []byte) *types.Log {
	t.Helper()
	parsed, err := iapplication.IApplicationMetaData.GetAbi()
	require.NoError(t, err)
	event := parsed.Events["RefundIssued"]
	data, err := event.Inputs.NonIndexed().Pack(input, []byte{0xab})
	require.NoError(t, err)
	return &types.Log{Address: address, Topics: []common.Hash{event.ID, common.BigToHash(index)}, Data: data}
}

func TestCommandRequiresMatchingRefundEvent(t *testing.T) {
	address := common.HexToAddress(testApplication)
	input := originalInput(t)
	matching := refundEventLog(t, address, big.NewInt(7), input)
	wrongEmitter := refundEventLog(t, common.HexToAddress("0x1234"), big.NewInt(7), input)
	wrongIndex := refundEventLog(t, address, big.NewInt(8), input)
	wrongInput := refundEventLog(t, address, big.NewInt(7), []byte{0x12})
	malformed := &types.Log{Address: address, Topics: matching.Topics, Data: []byte{0x01}}
	wrongTopic := &types.Log{Address: address, Topics: []common.Hash{common.HexToHash("0x12")}, Data: matching.Data}
	missingIndex := &types.Log{Address: address, Topics: matching.Topics[:1], Data: matching.Data}
	for _, test := range []struct {
		name    string
		logs    []*types.Log
		noWait  bool
		wantErr bool
	}{
		{name: "matching", logs: []*types.Log{matching}},
		{name: "empty receipt at EOA", logs: []*types.Log{}, wantErr: true},
		{name: "wrong emitter", logs: []*types.Log{wrongEmitter}, wantErr: true},
		{name: "wrong index", logs: []*types.Log{wrongIndex}, wantErr: true},
		{name: "wrong input", logs: []*types.Log{wrongInput}, wantErr: true},
		{name: "wrong event", logs: []*types.Log{wrongTopic}, wantErr: true},
		{name: "malformed", logs: []*types.Log{malformed}, wantErr: true},
		{name: "missing index", logs: []*types.Log{missingIndex}, wantErr: true},
		{name: "later match", logs: []*types.Log{nil, wrongEmitter, wrongIndex, wrongInput, malformed, wrongTopic, missingIndex, matching}},
		{name: "no wait with empty receipt", logs: []*types.Log{}, noWait: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &refundRPC{sent: make(chan *types.Transaction, 1), logs: test.logs, emptyCode: len(test.logs) == 0}
			setupRPC(t, service)
			t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "123456")
			t.Setenv(config.DATABASE_CONNECTION, "invalid")
			cmd := newCommand()
			cmd.SilenceUsage = true
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			args := []string{testApplication, "7", inputFileFlag, writeInputFile(t, hexutil.Encode(input)), yesFlag, jsonFlag}
			if test.noWait {
				args = append(args, "--no-wait")
			}
			cmd.SetArgs(args)
			err := cmd.ExecuteContext(t.Context())
			require.Len(t, service.sent, 1)
			tx := <-service.sent
			require.EqualValues(t, 123456, tx.Gas())
			require.Zero(t, service.estimates.Load())
			require.Contains(t, stderr.String(), tx.Hash().Hex())
			if test.noWait {
				require.Zero(t, service.receipts.Load())
			} else {
				require.EqualValues(t, 1, service.receipts.Load())
			}
			if test.wantErr {
				require.ErrorContains(t, err, "no matching RefundIssued event")
				require.ErrorContains(t, err, tx.Hash().Hex())
				require.Empty(t, stdout.String())
				return
			}
			require.NoError(t, err)
			var result map[string]string
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
			status := "mined"
			if test.noWait {
				status = "broadcast"
			}
			require.Equal(t, status, result["status"])
		})
	}
}

func TestCommandSignsOriginalInput(t *testing.T) {
	service := &refundRPC{sent: make(chan *types.Transaction, 1)}
	signer := setupRPC(t, service)
	input := originalInput(t)
	cmd := newCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{testApplication, "0x7", inputFileFlag, writeInputFile(t, hexutil.Encode(input)), yesFlag, jsonFlag})
	require.NoError(t, cmd.ExecuteContext(t.Context()))
	require.Len(t, service.sent, 1)
	tx := <-service.sent
	require.Equal(t, common.HexToAddress(testApplication), *tx.To())
	require.Zero(t, tx.Value().Sign())
	require.Equal(t, uint64(testGasLimit), tx.Gas())
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(testChainID)), tx)
	require.NoError(t, err)
	require.Equal(t, signer, from)
	appABI, err := iapplication.IApplicationMetaData.GetAbi()
	require.NoError(t, err)
	method, err := appABI.MethodById(tx.Data()[:4])
	require.NoError(t, err)
	require.Equal(t, "issueRefund", method.Name)
	args, err := method.Inputs.Unpack(tx.Data()[4:])
	require.NoError(t, err)
	require.Equal(t, big.NewInt(7), args[0])
	require.Equal(t, input, args[1])
	var result map[string]string
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.Equal(t, map[string]string{
		"transaction_hash": tx.Hash().Hex(), "application_address": testApplication, "input_index": "7",
		"status": "mined", "block_number": "0xa",
	}, result)
	require.EqualValues(t, 1, service.estimates.Load())
	require.EqualValues(t, 1, service.receipts.Load())
	require.Contains(t, stderr.String(), tx.Hash().Hex())
}

func TestCommandReceiptPolicy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		manualGas bool
		noWait    bool
		failed    bool
		pending   bool
		wantErr   string
	}{
		{name: "estimated mined"},
		{name: "manual gas mined", manualGas: true},
		{name: "manual gas receipt failure", manualGas: true, failed: true, wantErr: "failed in block"},
		{name: "estimated receipt failure", failed: true, wantErr: "failed in block"},
		{name: "estimated broadcast only", noWait: true, pending: true},
		{name: "manual gas broadcast only", manualGas: true, noWait: true, pending: true},
		{name: "pending timeout", pending: true, wantErr: "outcome unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &refundRPC{sent: make(chan *types.Transaction, 1), failed: tt.failed, pending: tt.pending}
			setupRPC(t, service)
			if tt.manualGas {
				t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "123456")
			}
			cmd := newCommand()
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			args := []string{testApplication, "7", inputFileFlag, writeInputFile(t, hexutil.Encode(originalInput(t))), yesFlag, jsonFlag}
			if tt.noWait {
				args = append(args, "--no-wait")
			}
			if tt.pending && !tt.noWait {
				args = append(args, "--wait-timeout", "20ms")
			}
			cmd.SetArgs(args)
			err := cmd.ExecuteContext(t.Context())
			require.Len(t, service.sent, 1)
			tx := <-service.sent
			if tt.manualGas {
				require.EqualValues(t, 123456, tx.Gas())
				require.Zero(t, service.estimates.Load())
			} else {
				require.EqualValues(t, testGasLimit, tx.Gas())
				require.EqualValues(t, 1, service.estimates.Load())
			}
			if tt.noWait {
				require.Zero(t, service.receipts.Load())
			} else {
				require.Positive(t, service.receipts.Load())
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.ErrorContains(t, err, tx.Hash().Hex())
				require.NotContains(t, stdout.String(), `"transaction_hash"`)
				return
			}
			require.NoError(t, err)
			var result map[string]string
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
			if tt.noWait {
				require.Equal(t, "broadcast", result["status"])
				require.NotContains(t, result, "block_number")
			} else {
				require.Equal(t, "mined", result["status"])
			}
		})
	}
}

// This subprocess keeps Cobra's default writers. SetOut would hide a regression
// from OutOrStdout back to Printf, because Cobra Printf defaults to stderr.
func TestCommandDefaultOutputProcess(t *testing.T) {
	if os.Getenv("CARTESI_TEST_REFUND_OUTPUT_PROCESS") != "1" {
		return
	}
	cmd := newCommand()
	for i, arg := range os.Args {
		if arg == "--" {
			cmd.SetArgs(os.Args[i+1:])
			break
		}
	}
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestCommandPlainResultUsesDefaultStdout(t *testing.T) {
	service := &refundRPC{sent: make(chan *types.Transaction, 1)}
	setupRPC(t, service)
	t.Setenv("CARTESI_TEST_REFUND_OUTPUT_PROCESS", "1")
	//nolint:gosec // Runs this test binary with fixed arguments and a local fixture path, without a shell.
	process := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCommandDefaultOutputProcess$", "--",
		testApplication, "7", inputFileFlag, writeInputFile(t, hexutil.Encode(originalInput(t))), yesFlag)
	var stdout, stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	require.NoError(t, process.Run(), stderr.String())
	require.Len(t, service.sent, 1)
	tx := <-service.sent
	require.Equal(t, "Transaction mined: "+tx.Hash().Hex()+"\n", stdout.String())
	require.Contains(t, stderr.String(), tx.Hash().Hex())
	require.NotContains(t, stderr.String(), "Transaction mined:")
}

func TestCommandReportsRefundReverts(t *testing.T) {
	appABI, err := iapplication.IApplicationMetaData.GetAbi()
	require.NoError(t, err)
	for _, name := range []string{"NotForeclosed", "CannotRefundFinalizedInput", "RefundAlreadyIssued"} {
		t.Run(name, func(t *testing.T) {
			contractError := appABI.Errors[name]
			data := append([]byte(nil), contractError.ID[:4]...)
			if len(contractError.Inputs) > 0 {
				args, err := contractError.Inputs.Pack(big.NewInt(7))
				require.NoError(t, err)
				data = append(data, args...)
			}
			service := &refundRPC{sent: make(chan *types.Transaction, 1), estimateErr: &refundRevert{data: hexutil.Encode(data)}}
			setupRPC(t, service)
			cmd := newCommand()
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{testApplication, "7", inputFileFlag, writeInputFile(t, hexutil.Encode(originalInput(t))), yesFlag})
			require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "decoded revert: "+name)
			require.Empty(t, service.sent)
			require.NotContains(t, stdout.String(), "Refund tx-hash")
		})
	}
}

func TestCommandKeepsHashOnBroadcastError(t *testing.T) {
	service := &refundRPC{sent: make(chan *types.Transaction, 1), sendErr: errors.New("response unavailable")}
	setupRPC(t, service)
	cmd := newCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{testApplication, "7", inputFileFlag, writeInputFile(t, hexutil.Encode(originalInput(t))), yesFlag, jsonFlag})
	err := cmd.ExecuteContext(t.Context())
	require.ErrorContains(t, err, "response unavailable")
	require.Len(t, service.sent, 1)
	tx := <-service.sent
	require.ErrorContains(t, err, tx.Hash().Hex())
	require.Contains(t, stderr.String(), tx.Hash().Hex())
	require.NotContains(t, stdout.String(), `"transaction_hash"`)
}

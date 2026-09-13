// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package execute

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
)

const (
	testApplication = "0x0000000000000000000000000000000000000001"
	testChainID     = 31337
	testGasLimit    = 90000
	proofFileFlag   = "--proof-file"
	jsonFlag        = "--json"
	yesFlag         = "--yes"
)

func writeProofFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "output-proof.json")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func proofDocument(t *testing.T, output []byte, siblings []common.Hash) string {
	t.Helper()
	data, err := json.Marshal(struct {
		RawData              string        `json:"raw_data"`
		OutputHashesSiblings []common.Hash `json:"output_hashes_siblings"`
	}{hexutil.Encode(output), siblings})
	require.NoError(t, err)
	return string(data)
}

func TestLoadOutputProofPreservesBytesAndOrder(t *testing.T) {
	t.Parallel()
	output := []byte{0, 1, 2, 3, 0xfe, 0xff}
	siblings := []common.Hash{common.HexToHash("0x1234"), common.HexToHash("0xabcd")}
	path := writeProofFile(t, " \n"+proofDocument(t, output, siblings)+"\n\t")
	gotOutput, gotSiblings, err := loadOutputProof(path)
	require.NoError(t, err)
	require.Equal(t, output, gotOutput)
	require.Equal(t, [][32]byte{siblings[0], siblings[1]}, gotSiblings)
}

func TestLoadOutputProofRejectsInvalidFiles(t *testing.T) {
	t.Parallel()
	hash := common.HexToHash("0x01").Hex()
	for name, contents := range map[string]string{
		"empty file":             "",
		"null object":            "null",
		"array object":           "[]",
		"missing fields":         "{}",
		"missing output":         `{"output_hashes_siblings":["` + hash + `"]}`,
		"empty output":           `{"raw_data":"0x","output_hashes_siblings":["` + hash + `"]}`,
		"null output":            `{"raw_data":null,"output_hashes_siblings":["` + hash + `"]}`,
		"missing output prefix":  `{"raw_data":"aabb","output_hashes_siblings":["` + hash + `"]}`,
		"invalid output hex":     `{"raw_data":"0xgg","output_hashes_siblings":["` + hash + `"]}`,
		"odd output hex":         `{"raw_data":"0xabc","output_hashes_siblings":["` + hash + `"]}`,
		"missing siblings":       `{"raw_data":"0xaa"}`,
		"null siblings":          `{"raw_data":"0xaa","output_hashes_siblings":null}`,
		"empty siblings":         `{"raw_data":"0xaa","output_hashes_siblings":[]}`,
		"null sibling":           `{"raw_data":"0xaa","output_hashes_siblings":[null]}`,
		"non-string sibling":     `{"raw_data":"0xaa","output_hashes_siblings":[1]}`,
		"empty sibling":          `{"raw_data":"0xaa","output_hashes_siblings":["0x"]}`,
		"missing sibling prefix": `{"raw_data":"0xaa","output_hashes_siblings":["` + hash[2:] + `"]}`,
		"short sibling":          `{"raw_data":"0xaa","output_hashes_siblings":["0xab"]}`,
		"long sibling":           `{"raw_data":"0xaa","output_hashes_siblings":["0x` + strings.Repeat("ab", 33) + `"]}`,
		"invalid sibling hex":    `{"raw_data":"0xaa","output_hashes_siblings":["0x` + strings.Repeat("gg", 32) + `"]}`,
		"unknown index":          `{"raw_data":"0xaa","output_hashes_siblings":["` + hash + `"],"output_index":7}`,
		"trailing object":        `{"raw_data":"0xaa","output_hashes_siblings":["` + hash + `"]}{}`,
		"trailing null":          `{"raw_data":"0xaa","output_hashes_siblings":["` + hash + `"]}null`,
		"trailing garbage":       `{"raw_data":"0xaa","output_hashes_siblings":["` + hash + `"]}garbage`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			output, siblings, err := loadOutputProof(writeProofFile(t, contents))
			require.Error(t, err)
			require.Nil(t, output)
			require.Nil(t, siblings)
		})
	}
	_, _, err := loadOutputProof(filepath.Join(t.TempDir(), "missing.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// executeRPC is a local Ethereum JSON-RPC server for the real generated binding.
// It does not implement a node JSON-RPC API or connect to a blockchain.
type executeRPC struct {
	sent       chan *types.Transaction
	chainReads atomic.Int32
	estimates  atomic.Int32
	receipts   atomic.Int32
	failed     bool
	logs       []*types.Log
}

func (r *executeRPC) ChainId(context.Context) (*hexutil.Big, error) { //nolint:revive // Ethereum RPC method eth_chainId.
	r.chainReads.Add(1)
	return (*hexutil.Big)(big.NewInt(testChainID)), nil
}

func (*executeRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), GasLimit: testGasLimit}, nil
}

func (*executeRPC) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1)), nil
}

func (*executeRPC) GetCode(context.Context, common.Address, string) (hexutil.Bytes, error) {
	return hexutil.Bytes{1}, nil
}

func (*executeRPC) GetTransactionCount(context.Context, common.Address, string) (hexutil.Uint64, error) {
	return 0, nil
}

func (r *executeRPC) EstimateGas(context.Context, map[string]json.RawMessage) (hexutil.Uint64, error) {
	r.estimates.Add(1)
	return testGasLimit, nil
}

func (r *executeRPC) SendRawTransaction(_ context.Context, raw hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	r.sent <- &tx
	return tx.Hash(), nil
}

func (r *executeRPC) GetTransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	r.receipts.Add(1)
	status := uint64(types.ReceiptStatusSuccessful)
	if r.failed {
		status = types.ReceiptStatusFailed
	}
	return &types.Receipt{TxHash: hash, Status: status, BlockNumber: big.NewInt(10),
		Logs: r.logs, GasUsed: testGasLimit, CumulativeGasUsed: testGasLimit}, nil
}

func outputExecutedLog(t *testing.T, application common.Address, index uint64, output []byte) *types.Log {
	t.Helper()
	parsed, err := iapplication.IApplicationMetaData.GetAbi()
	require.NoError(t, err)
	event := parsed.Events["OutputExecuted"]
	data, err := event.Inputs.NonIndexed().Pack(output)
	require.NoError(t, err)
	return &types.Log{
		Address: application,
		Topics:  []common.Hash{event.ID, common.BigToHash(new(big.Int).SetUint64(index))},
		Data:    data,
	}
}

func setupExecuteRPC(t *testing.T, service *executeRPC) common.Address {
	t.Helper()
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
	// This value fails configuration parsing. A successful address/file command
	// therefore proves that it did not read the database setting or open a DB.
	t.Setenv(config.DATABASE_CONNECTION, "://invalid-database")
	return crypto.PubkeyToAddress(key.PublicKey)
}

func TestCommandExecutesProofFileWithoutDatabase(t *testing.T) {
	outputABI, err := outputs.OutputsMetaData.GetAbi()
	require.NoError(t, err)
	output, err := outputABI.Pack("Voucher", common.HexToAddress("0x02"), big.NewInt(3), []byte{0xaa, 0xbb})
	require.NoError(t, err)
	// The CLI must submit complete proven bytes, without decoding and rebuilding
	// the output or dropping any committed suffix.
	output = append(output, 0xcc, 0xdd)
	siblings := []common.Hash{common.HexToHash("0x11"), common.HexToHash("0x22")}
	file := writeProofFile(t, proofDocument(t, output, siblings))
	for _, tt := range []struct {
		name      string
		manualGas bool
		noWait    bool
		failed    bool
	}{
		{name: "estimate and wait"},
		{name: "manual gas and wait", manualGas: true},
		{name: "manual gas failed receipt", manualGas: true, failed: true},
		{name: "estimated no wait", noWait: true},
		{name: "manual gas no wait", manualGas: true, noWait: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &executeRPC{
				sent: make(chan *types.Transaction, 1), failed: tt.failed,
				logs: []*types.Log{outputExecutedLog(t, common.HexToAddress(testApplication), 7, output)},
			}
			signer := setupExecuteRPC(t, service)
			if tt.manualGas {
				t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "123456")
			}
			cmd := newCommand()
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			args := []string{testApplication, "0x7", proofFileFlag, file, yesFlag, jsonFlag}
			if tt.noWait {
				args = append(args, "--no-wait")
			}
			cmd.SetArgs(args)
			err := cmd.ExecuteContext(t.Context())
			require.Len(t, service.sent, 1, "command error: %v", err)
			tx := <-service.sent
			if tt.failed {
				require.ErrorContains(t, err, "receipt status 0")
				require.ErrorContains(t, err, tx.Hash().Hex())
				require.NotContains(t, stdout.String(), `"transaction_hash"`)
			} else {
				require.NoError(t, err)
				var result map[string]string
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
				require.Equal(t, tx.Hash().Hex(), result["transaction_hash"])
				if tt.noWait {
					require.Equal(t, "broadcast", result["status"])
					require.NotContains(t, result, "block_number")
				} else {
					require.Equal(t, "mined", result["status"])
				}
			}
			if tt.noWait {
				require.Zero(t, service.receipts.Load())
			} else {
				require.EqualValues(t, 1, service.receipts.Load())
			}
			if tt.manualGas {
				require.Zero(t, service.estimates.Load())
				require.EqualValues(t, 123456, tx.Gas())
			} else {
				require.EqualValues(t, 1, service.estimates.Load())
			}
			require.Equal(t, common.HexToAddress(testApplication), *tx.To())
			from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(testChainID)), tx)
			require.NoError(t, err)
			require.Equal(t, signer, from)
			appABI, err := iapplication.IApplicationMetaData.GetAbi()
			require.NoError(t, err)
			method, err := appABI.MethodById(tx.Data()[:4])
			require.NoError(t, err)
			require.Equal(t, "executeOutput", method.Name)
			callArgs, err := method.Inputs.Unpack(tx.Data()[4:])
			require.NoError(t, err)
			require.Equal(t, output, callArgs[0])
			proof := abi.ConvertType(callArgs[1], new(iapplication.OutputValidityProof)).(*iapplication.OutputValidityProof)
			require.Equal(t, uint64(7), proof.OutputIndex)
			require.Equal(t, [][32]byte{siblings[0], siblings[1]}, proof.OutputHashesSiblings)
		})
	}
}

func TestCommandRequiresMatchingOutputExecuted(t *testing.T) {
	application := common.HexToAddress(testApplication)
	output := []byte{0xaa, 0xbb, 0xcc}
	file := writeProofFile(t, proofDocument(t, output, []common.Hash{common.HexToHash("0x11")}))
	valid := outputExecutedLog(t, application, 7, output)
	wrongEmitter := outputExecutedLog(t, common.HexToAddress("0x02"), 7, output)
	wrongIndex := outputExecutedLog(t, application, 8, output)
	wrongBytes := outputExecutedLog(t, application, 7, []byte{0xaa, 0xbb})
	malformed := &types.Log{Address: application, Topics: valid.Topics, Data: []byte{1}}
	missingIndex := &types.Log{Address: application, Topics: valid.Topics[:1], Data: valid.Data}
	unrelated := &types.Log{Address: application, Topics: []common.Hash{common.HexToHash("0x03")}}
	for _, test := range []struct {
		name      string
		logs      []*types.Log
		noWait    bool
		wantError bool
	}{
		{name: "empty receipt", logs: []*types.Log{}, wantError: true},
		{name: "wrong emitter", logs: []*types.Log{wrongEmitter}, wantError: true},
		{name: "wrong index", logs: []*types.Log{wrongIndex}, wantError: true},
		{name: "wrong bytes", logs: []*types.Log{wrongBytes}, wantError: true},
		{name: "malformed data", logs: []*types.Log{malformed}, wantError: true},
		{name: "missing index topic", logs: []*types.Log{missingIndex}, wantError: true},
		{name: "unrelated event", logs: []*types.Log{unrelated}, wantError: true},
		{name: "nil log", logs: []*types.Log{nil}, wantError: true},
		{name: "later matching event",
			logs: []*types.Log{wrongEmitter, wrongIndex, wrongBytes, malformed, missingIndex, unrelated, nil, valid}},
		{name: "no wait does not require event", logs: []*types.Log{}, noWait: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &executeRPC{sent: make(chan *types.Transaction, 1), logs: test.logs}
			setupExecuteRPC(t, service)
			// A manual limit permits broadcast even when estimation would have
			// rejected a wrong destination. Receipt evidence must still be checked.
			t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "123456")
			cmd := newCommand()
			cmd.SilenceUsage = true // Match the root CLI's error-output policy.
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			args := []string{testApplication, "7", proofFileFlag, file, yesFlag, jsonFlag}
			if test.noWait {
				args = append(args, "--no-wait")
			}
			cmd.SetArgs(args)
			err := cmd.ExecuteContext(t.Context())
			require.Len(t, service.sent, 1)
			tx := <-service.sent
			require.Zero(t, service.estimates.Load())
			require.EqualValues(t, 123456, tx.Gas())
			if test.noWait {
				require.Zero(t, service.receipts.Load())
			} else {
				require.EqualValues(t, 1, service.receipts.Load())
			}
			if test.wantError {
				require.ErrorContains(t, err, "no matching OutputExecuted event")
				require.ErrorContains(t, err, tx.Hash().Hex())
				require.Empty(t, stdout.String())
				return
			}
			require.NoError(t, err)
			var result map[string]string
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
			require.Equal(t, tx.Hash().Hex(), result["transaction_hash"])
			if test.noWait {
				require.Equal(t, "broadcast", result["status"])
			} else {
				require.Equal(t, "mined", result["status"])
			}
		})
	}
}

func TestCommandRejectsInvalidProofBeforeEthereum(t *testing.T) {
	service := &executeRPC{sent: make(chan *types.Transaction, 1)}
	setupExecuteRPC(t, service)
	for _, application := range []string{testApplication, "echo-dapp"} {
		t.Run(application, func(t *testing.T) {
			cmd := newCommand()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{application, "7", proofFileFlag, writeProofFile(t, `{"raw_data":"0xaa","unknown":1}`)})
			require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "unknown field")
			require.Zero(t, service.chainReads.Load())
			require.Empty(t, service.sent)
		})
	}
}

func TestCommandWithoutFileStillRequiresDatabase(t *testing.T) {
	service := &executeRPC{sent: make(chan *types.Transaction, 1)}
	setupExecuteRPC(t, service)
	cmd := newCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{testApplication, "7", yesFlag})
	require.ErrorContains(t, cmd.ExecuteContext(t.Context()), config.DATABASE_CONNECTION)
	require.Zero(t, service.chainReads.Load())
	require.Empty(t, service.sent)
}

func TestCommandRejectsEmptyProofPathBeforeLookup(t *testing.T) {
	service := &executeRPC{sent: make(chan *types.Transaction, 1)}
	setupExecuteRPC(t, service)
	cmd := newCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{testApplication, "7", proofFileFlag, ""})
	require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "--proof-file cannot be empty")
	require.Zero(t, service.chainReads.Load())
	require.Empty(t, service.sent)
}

func TestCommandProofFileNameStillRequiresDatabase(t *testing.T) {
	service := &executeRPC{sent: make(chan *types.Transaction, 1)}
	setupExecuteRPC(t, service)
	cmd := newCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	file := writeProofFile(t, proofDocument(t, []byte{0xaa}, []common.Hash{common.HexToHash("0x01")}))
	cmd.SetArgs([]string{"echo-dapp", "7", proofFileFlag, file, yesFlag})
	require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "resolving application")
	require.Zero(t, service.chainReads.Load())
	require.Empty(t, service.sent)
}

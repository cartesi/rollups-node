// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package provedriveroot

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
)

func TestCommandRequiresMatchingDriveRootEvent(t *testing.T) {
	address := common.HexToAddress("0x1234")
	root := common.HexToHash("0x42")
	parsed, err := iapplication.IApplicationMetaData.GetAbi()
	require.NoError(t, err)
	event := parsed.Events["AccountsDriveMerkleRootProved"]
	matching := &types.Log{Address: address, Topics: []common.Hash{event.ID}, Data: root.Bytes()}
	wrongEmitter := &types.Log{Address: common.HexToAddress("0x5678"), Topics: matching.Topics, Data: matching.Data}
	wrongRoot := &types.Log{Address: address, Topics: matching.Topics, Data: common.HexToHash("0x43").Bytes()}
	wrongTopic := &types.Log{Address: address, Topics: []common.Hash{common.HexToHash("0x01")}, Data: matching.Data}
	malformed := &types.Log{Address: address, Topics: matching.Topics, Data: []byte{0x42}}
	emptyData := &types.Log{Address: address, Topics: matching.Topics}
	extraData := &types.Log{Address: address, Topics: matching.Topics, Data: append(root.Bytes(), root.Bytes()...)}
	for _, test := range []struct {
		name      string
		logs      []*types.Log
		estimated bool
		noWait    bool
		zeroRoot  bool
		wantErr   bool
	}{
		{name: "matching manual gas", logs: []*types.Log{matching}},
		{name: "matching estimated gas", logs: []*types.Log{matching}, estimated: true},
		{name: "empty receipt at EOA", logs: []*types.Log{}, wantErr: true},
		{name: "wrong emitter", logs: []*types.Log{wrongEmitter}, wantErr: true},
		{name: "wrong root", logs: []*types.Log{wrongRoot}, wantErr: true},
		{name: "wrong event", logs: []*types.Log{wrongTopic}, wantErr: true},
		{name: "malformed data", logs: []*types.Log{malformed}, wantErr: true},
		{name: "empty data for zero root", logs: []*types.Log{emptyData}, zeroRoot: true, wantErr: true},
		{name: "extra data", logs: []*types.Log{extraData}, wantErr: true},
		{name: "later match", logs: []*types.Log{nil, wrongEmitter, wrongRoot, wrongTopic, malformed, matching}},
		{name: "no wait with empty receipt", logs: []*types.Log{}, noWait: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &driveRootRPC{logs: test.logs, sent: make(chan *types.Transaction, 1), emptyCode: len(test.logs) == 0}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", service))
			defer server.Stop()
			httpServer := httptest.NewServer(server)
			defer httpServer.Close()
			key, err := crypto.GenerateKey()
			require.NoError(t, err)
			t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, httpServer.URL)
			t.Setenv(config.AUTH_KIND, "private_key")
			t.Setenv(config.AUTH_PRIVATE_KEY, hexutil.Encode(crypto.FromECDSA(key)))
			t.Setenv(config.DATABASE_CONNECTION, "invalid")
			t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "123456")
			if test.estimated {
				t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "0")
			}
			Cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				require.NoError(t, flag.Value.Set(flag.DefValue))
				flag.Changed = false
			})
			Cmd.SilenceUsage = true
			var stdout, stderr bytes.Buffer
			Cmd.SetOut(&stdout)
			Cmd.SetErr(&stderr)
			proofRoot := root
			if test.zeroRoot {
				proofRoot = common.Hash{}
			}
			proof, err := json.Marshal(proveDriveRootJSON{AccountsDriveMerkleRoot: proofRoot.Hex(), Proof: []string{root.Hex()}})
			require.NoError(t, err)
			args := []string{address.Hex(), "--proof-file", writeProofFile(t, string(proof)), "--yes", "--json"}
			if test.noWait {
				args = append(args, "--no-wait")
			}
			Cmd.SetArgs(args)
			err = Cmd.ExecuteContext(t.Context())
			require.Len(t, service.sent, 1)
			tx := <-service.sent
			require.EqualValues(t, 123456, tx.Gas())
			if test.estimated {
				require.EqualValues(t, 1, service.estimates.Load())
			} else {
				require.Zero(t, service.estimates.Load())
			}
			require.Contains(t, stderr.String(), tx.Hash().Hex())
			if test.noWait {
				require.Zero(t, service.receipts.Load())
			} else {
				require.EqualValues(t, 1, service.receipts.Load())
			}
			if test.wantErr {
				require.ErrorContains(t, err, "no matching AccountsDriveMerkleRootProved event")
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

type driveRootRPC struct {
	logs      []*types.Log
	sent      chan *types.Transaction
	emptyCode bool
	estimates atomic.Int32
	receipts  atomic.Int32
}

func (*driveRootRPC) ChainId(context.Context) *hexutil.Big { //nolint:revive // Ethereum RPC method eth_chainId.
	return (*hexutil.Big)(big.NewInt(31337))
}

func (*driveRootRPC) GetBlockByNumber(context.Context, string, bool) *types.Header {
	return &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), GasLimit: 1_000_000}
}

func (*driveRootRPC) GasPrice(context.Context) *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(1))
}

func (r *driveRootRPC) GetCode(context.Context, common.Address, string) hexutil.Bytes {
	if r.emptyCode {
		return hexutil.Bytes{}
	}
	return hexutil.Bytes{0x01}
}

func (*driveRootRPC) GetTransactionCount(context.Context, common.Address, string) hexutil.Uint64 {
	return 0
}

func (r *driveRootRPC) EstimateGas(context.Context, map[string]json.RawMessage) hexutil.Uint64 {
	r.estimates.Add(1)
	return 123456
}

func (r *driveRootRPC) SendRawTransaction(_ context.Context, raw hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	r.sent <- &tx
	return tx.Hash(), nil
}

func (r *driveRootRPC) GetTransactionReceipt(_ context.Context, hash common.Hash) *types.Receipt {
	r.receipts.Add(1)
	return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(10), Logs: r.logs}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deposit

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/ierc20portal"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
)

func TestDepositReceiptEvidence(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*testing.T, *depositRPC)
		valid  bool
	}{
		{name: "matching input", valid: true},
		{name: "no events", change: func(_ *testing.T, r *depositRPC) { r.depositLogs = nil }},
		{name: "wrong input box", change: func(_ *testing.T, r *depositRPC) { r.depositLogs[0].Address = common.Address{} }},
		{name: "wrong outer application", change: func(_ *testing.T, r *depositRPC) { r.depositLogs[0].Topics[1] = common.Hash{} }},
		{name: "wrong outer index", change: func(_ *testing.T, r *depositRPC) { r.depositLogs[0].Topics[2] = common.Hash{} }},
		{name: "wrong event", change: func(_ *testing.T, r *depositRPC) { r.depositLogs[0].Topics[0] = common.Hash{} }},
		{name: "malformed event", change: func(_ *testing.T, r *depositRPC) { r.depositLogs[0].Data = []byte{1} }},
		{name: "zero input box", change: func(_ *testing.T, r *depositRPC) { r.inputBoxResponse = make([]byte, common.HashLength) }},
		{name: "bad input box response", change: func(_ *testing.T, r *depositRPC) { r.inputBoxResponse = []byte{1} }},
		{name: "wrong input selector", change: func(t *testing.T, r *depositRPC) {
			changeDepositInput(t, r.depositLogs[0], func(input []byte) []byte { input[0] ^= 1; return input })
		}},
		{name: "malformed input", change: func(t *testing.T, r *depositRPC) {
			changeDepositInput(t, r.depositLogs[0], func(input []byte) []byte { return input[:5] })
		}},
		{name: "wrong inner application", change: changeAdvanceField(1, common.Address{})},
		{name: "wrong portal sender", change: changeAdvanceField(2, common.Address{})},
		{name: "wrong inner index", change: changeAdvanceField(6, big.NewInt(8))},
		{name: "wrong token", change: changePayloadByte(0)},
		{name: "wrong depositor", change: changePayloadByte(20)},
		{name: "wrong amount", change: changePayloadByte(71)},
		{name: "wrong execution data", change: func(t *testing.T, r *depositRPC) {
			changeAdvance(t, r.depositLogs[0], func(args []any) { args[7] = append(args[7].([]byte), 1) })
		}},
		{name: "matching event after unrelated event", valid: true, change: func(_ *testing.T, r *depositRPC) {
			wrong := *r.depositLogs[0]
			wrong.Data = []byte{1}
			r.depositLogs = append([]*types.Log{&wrong}, r.depositLogs...)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rpc := &depositRPC{sent: make(chan *types.Transaction, 2)}
			setupDepositRPC(t, rpc)
			if tt.change != nil {
				tt.change(t, rpc)
			}
			t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "90000")
			output, err := runDeposit(t)
			require.Len(t, rpc.sent, 1)
			tx := <-rpc.sent
			require.Zero(t, rpc.estimates.Load())
			require.EqualValues(t, 1, rpc.receipts.Load())
			require.Equal(t, "0xa", rpc.inputBoxBlock.Load())
			if tt.valid {
				require.NoError(t, err)
				require.Contains(t, output, `"status": "mined"`)
			} else {
				require.ErrorContains(t, err, "deposit could not be confirmed")
				require.ErrorContains(t, err, tx.Hash().Hex())
				require.NotContains(t, output, `"status": "mined"`)
			}
		})
	}
}

func TestApprovalReceiptEvidence(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*depositRPC)
		valid  bool
	}{
		{name: "matching approval", valid: true},
		{name: "no events", change: func(r *depositRPC) { r.approvalLogs = nil }},
		{name: "wrong token", change: func(r *depositRPC) { r.approvalLogs[0].Address = common.Address{} }},
		{name: "wrong event", change: func(r *depositRPC) { r.approvalLogs[0].Topics[0] = common.Hash{} }},
		{name: "wrong owner", change: func(r *depositRPC) { r.approvalLogs[0].Topics[1] = common.Hash{} }},
		{name: "wrong spender", change: func(r *depositRPC) { r.approvalLogs[0].Topics[2] = common.Hash{} }},
		{name: "wrong amount", change: func(r *depositRPC) { r.approvalLogs[0].Data[31]++ }},
		{name: "missing amount", change: func(r *depositRPC) { r.approvalLogs[0].Data = nil }},
		{name: "malformed data", change: func(r *depositRPC) { r.approvalLogs[0].Data = []byte{1} }},
		{name: "matching event after unrelated event", valid: true, change: func(r *depositRPC) {
			wrong := *r.approvalLogs[0]
			wrong.Data = nil
			r.approvalLogs = append([]*types.Log{&wrong}, r.approvalLogs...)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rpc := &depositRPC{sent: make(chan *types.Transaction, 2)}
			setupDepositRPC(t, rpc)
			if tt.change != nil {
				tt.change(rpc)
			}
			t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "90000")
			output, err := runDeposit(t, approveFlag)
			require.Zero(t, rpc.estimates.Load())
			if tt.valid {
				require.NoError(t, err)
				require.Len(t, rpc.sent, 2)
				require.Contains(t, output, `"approve_transaction_hash"`)
			} else {
				require.Len(t, rpc.sent, 1, "a deposit must not follow an unconfirmed approval")
				require.ErrorContains(t, err, (<-rpc.sent).Hash().Hex())
				require.ErrorContains(t, err, "no matching Approval event")
				require.NotContains(t, output, `"status": "mined"`)
				require.Zero(t, rpc.inputBoxReads.Load())
			}
		})
	}
}

func TestDepositNoWaitSkipsReceiptEvidence(t *testing.T) {
	rpc := &depositRPC{sent: make(chan *types.Transaction, 2)}
	setupDepositRPC(t, rpc)
	rpc.depositLogs = nil
	rpc.inputBoxResponse = nil
	t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "90000")
	output, err := runDeposit(t, "--no-wait")
	require.NoError(t, err)
	require.Contains(t, output, `"status": "broadcast"`)
	require.Zero(t, rpc.receipts.Load())
	require.Zero(t, rpc.estimates.Load())
	require.Zero(t, rpc.inputBoxReads.Load())
}

func TestDepositReceiptWithExecutionData(t *testing.T) {
	rpc := &depositRPC{sent: make(chan *types.Transaction, 2)}
	setupDepositRPC(t, rpc)
	execData := []byte{0xde, 0xad, 0xbe, 0xef}
	changeAdvance(t, rpc.depositLogs[0], func(args []any) { args[7] = append(args[7].([]byte), execData...) })
	output, err := runDeposit(t, "--exec-data", "0xdeadbeef")
	require.NoError(t, err)
	require.Contains(t, output, `"status": "mined"`)
	require.Len(t, rpc.sent, 1)
	portalABI, err := ierc20portal.IErc20PortalMetaData.GetAbi()
	require.NoError(t, err)
	method := portalABI.Methods["depositErc20Tokens"]
	tx := <-rpc.sent
	require.Equal(t, method.ID, tx.Data()[:len(method.ID)])
	args, err := method.Inputs.Unpack(tx.Data()[len(method.ID):])
	require.NoError(t, err)
	require.Equal(t, execData, args[3])
}

func TestParseAmountUint256(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	maxAmount := new(big.Int).Sub(new(big.Int).Set(limit), big.NewInt(1))
	amount, err := parseAmount(maxAmount.String())
	require.NoError(t, err)
	require.Equal(t, maxAmount, amount)
	_, err = parseAmount(limit.String())
	require.ErrorContains(t, err, "uint256")
}

func runDeposit(t *testing.T, extra ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	Cmd.SetOut(&stdout)
	Cmd.SetErr(&stderr)
	args := append([]string{"erc20", testApplication, "--portal", testPortal, "--token", testToken,
		"--amount", "100", "--yes", "--json"}, extra...)
	Cmd.SetArgs(args)
	err := Cmd.ExecuteContext(t.Context())
	return stdout.String(), err
}

func changeDepositInput(t *testing.T, log *types.Log, change func([]byte) []byte) {
	t.Helper()
	parsed, err := iinputbox.IInputBoxMetaData.GetAbi()
	require.NoError(t, err)
	args := parsed.Events["InputAdded"].Inputs.NonIndexed()
	values, err := args.Unpack(log.Data)
	require.NoError(t, err)
	log.Data, err = args.Pack(change(values[0].([]byte)))
	require.NoError(t, err)
}

func changeAdvance(t *testing.T, log *types.Log, change func([]any)) {
	t.Helper()
	parsed, err := inputs.InputsMetaData.GetAbi()
	require.NoError(t, err)
	method := parsed.Methods["EvmAdvance"]
	changeDepositInput(t, log, func(input []byte) []byte {
		args, err := method.Inputs.Unpack(input[len(method.ID):])
		require.NoError(t, err)
		change(args)
		input, err = parsed.Pack("EvmAdvance", args...)
		require.NoError(t, err)
		return input
	})
}

func changeAdvanceField(field int, value any) func(*testing.T, *depositRPC) {
	return func(t *testing.T, r *depositRPC) {
		changeAdvance(t, r.depositLogs[0], func(args []any) { args[field] = value })
	}
}

func changePayloadByte(offset int) func(*testing.T, *depositRPC) {
	return func(t *testing.T, r *depositRPC) {
		changeAdvance(t, r.depositLogs[0], func(args []any) { args[7].([]byte)[offset] ^= 1 })
	}
}

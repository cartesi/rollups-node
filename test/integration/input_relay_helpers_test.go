// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

// Generated from testdata/InputRelay.sol with solc 0.8.30, no optimizer:
//
//	solc --abi --bin testdata/InputRelay.sol
//
// The static artifacts keep the integration tests independent of a local
// Solidity toolchain. Regenerate both files when InputRelay.sol changes.
//
//go:embed testdata/input_relay_abi.json
var inputRelayABIJSON string

//go:embed testdata/input_relay_bytecode.hex
var inputRelayBytecode string

// sendInputThroughRelay deploys a contract that forwards one input to the
// InputBox. The echo DApp then addresses its voucher to this contract. The
// relay's payable fallback accepts the non-empty voucher payload under the v3
// application target-code rule.
func sendInputThroughRelay(
	ctx context.Context,
	t testing.TB,
	appAddress common.Address,
	payload string,
) (uint64, uint64, common.Address) {
	t.Helper()
	r := require.New(t)
	r.NotEqual(common.Address{}, appAddress, "application address must be non-zero")
	r.NotEmpty(payload, "relay input payload must be non-empty")

	client := newIntegrationEthClient(ctx, t)
	defer client.Close()
	inputBoxAddr := inputBoxAddress(t)
	inputBox, err := iinputbox.NewIInputBox(inputBoxAddr, client)
	r.NoError(err, "bind InputBox")

	parsed, err := abi.JSON(strings.NewReader(inputRelayABIJSON))
	r.NoError(err, "parse InputRelay ABI")
	deployOpts := transactorForMnemonicIndex(ctx, t, client, integrationCLIAccountIndex)
	deployOpts.GasLimit = 3_000_000
	relayAddress, deployTx, relay, err := bind.DeployContract(
		deployOpts,
		parsed,
		common.FromHex(strings.TrimSpace(inputRelayBytecode)),
		client,
		inputBoxAddr,
	)
	r.NoError(err, "deploy InputRelay")

	receiptCtx, receiptCancel := context.WithTimeout(ctx, 30*time.Second)
	deployReceipt := waitReceipt(receiptCtx, t, client, deployTx)
	receiptCancel()
	r.Equal(types.ReceiptStatusSuccessful, deployReceipt.Status,
		"InputRelay deployment reverted in tx %s", deployTx.Hash())

	submitOpts := transactorForMnemonicIndex(ctx, t, client, integrationCLIAccountIndex)
	submitOpts.GasLimit = 2_000_000
	submitTx, err := relay.Transact(submitOpts, "addInput", appAddress, []byte(payload))
	r.NoError(err, "submit input through InputRelay")
	receiptCtx, receiptCancel = context.WithTimeout(ctx, 30*time.Second)
	submitReceipt := waitReceipt(receiptCtx, t, client, submitTx)
	receiptCancel()
	r.Equal(types.ReceiptStatusSuccessful, submitReceipt.Status,
		"InputRelay submission reverted in tx %s", submitTx.Hash())

	var matchingInputs []*iinputbox.IInputBoxInputAdded
	for _, rawLog := range submitReceipt.Logs {
		if rawLog.Address != inputBoxAddr {
			continue
		}
		event, err := inputBox.ParseInputAdded(*rawLog)
		if err == nil && event.AppContract == appAddress {
			matchingInputs = append(matchingInputs, event)
		}
	}
	r.Len(matchingInputs, 1, "relay transaction must emit one InputAdded event for the application")
	event := matchingInputs[0]
	r.Equal(submitTx.Hash(), event.Raw.TxHash, "InputAdded event must belong to the relay transaction")
	r.True(event.Index.IsUint64(), "input index must fit uint64")

	return event.Index.Uint64(), submitReceipt.BlockNumber.Uint64(), relayAddress
}

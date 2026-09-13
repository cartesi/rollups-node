// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/ierc20metadata"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
)

const (
	refundPayerIndex    uint32 = 9
	refundDepositAmount uint64 = 37
	refundSelectorBytes        = 4
)

// TestRefundLifecycle stops every submitter before the second deposit exists.
// No claim prepared before shutdown can finalize that deposit. Foreclosure is
// mined before restarting, so the finalization boundary does not depend on a
// race against the background claimer, epoch timing, or local input status.
func TestRefundLifecycle(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("refund finalization control requires a test-managed node")
	}
	for _, consensus := range []withdrawalConsensus{
		withdrawalConsensusAuthority, withdrawalConsensusQuorum, withdrawalConsensusPRT,
	} {
		t.Run(string(consensus), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			t.Cleanup(cancel)
			client := newIntegrationEthClient(ctx, t)
			t.Cleanup(client.Close)
			chainID, err := client.ChainID(ctx)
			require.NoError(t, err)

			// Reuse the existing lifecycle helpers without running its suite.
			s := &WithdrawalLifecycleSuite{ctx: ctx, client: client, chainID: chainID}
			s.SetT(t)
			s.StartLogCapture()
			if consensus == withdrawalConsensusPRT {
				// PRT settlement mines blocks rapidly while the node is running.
				s.SetExpectedLogs(t, prtBlockOutOfRangeAllowlist)
			}
			t.Cleanup(func() { s.CheckLogs(t) })
			// Keep recovery separate: a fatal cleanup assertion must not leave
			// the shared node stopped for subsequent integration tests.
			t.Cleanup(func() {
				if sharedNode == nil {
					startSharedNode(t)
				}
			})
			t.Cleanup(func() {
				if s.appName != "" {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
					defer cleanupCancel()
					require.NoError(t, disableApplication(cleanupCtx, s.appName))
				}
			})
			runRefundLifecycle(s, consensus)
		})
	}
}

type refundLifecycleFixture struct {
	s        *WithdrawalLifecycleSuite
	app      withdrawalAppDeployment
	contract *iapplication.IApplication
	portal   common.Address
	token    common.Address
	first    *iinputbox.IInputBoxInputAdded
	second   *iinputbox.IInputBoxInputAdded
}

func runRefundLifecycle(s *WithdrawalLifecycleSuite, consensus withdrawalConsensus) {
	r := s.Require()
	dappPath := envOrDefault("CARTESI_TEST_ERC20_WITHDRAWAL_DAPP_PATH", "applications/erc20-withdrawal-dapp")
	f := refundLifecycleFixture{
		s:      s,
		portal: devnetAddress(s.T(), "CARTESI_DEVNET_ERC20_PORTAL_ADDRESS", defaultDevnetERC20PortalAddress),
		token:  devnetAddress(s.T(), "CARTESI_DEVNET_TEST_ERC20_ADDRESS", defaultDevnetTestERC20Address),
	}
	depositor := mnemonicAddress(s.T(), withdrawalUserIndex)
	payer := mnemonicAddress(s.T(), refundPayerIndex)
	guardian := mnemonicAddress(s.T(), withdrawalGuardianIndex)
	r.NotEqual(depositor, payer)
	r.NotEqual(guardian, payer)
	r.NotEqual(guardian, depositor)
	r.NoError(anvilSetBalance(s.ctx, depositor.Hex(), oneEtherWei))
	r.NoError(anvilSetBalance(s.ctx, payer.Hex(), oneEtherWei))
	initialDepositorBalance := s.tokenBalance(f.token, depositor)
	s.mintTestToken(f.token, withdrawalUserIndex, tokenAmount(withdrawalDepositAmount+refundDepositAmount))
	f.app = s.deployWithdrawalApp(consensus, dappPath)
	s.appName = f.app.appName
	var err error
	f.contract, err = iapplication.NewIApplication(f.app.appAddress, s.client)
	r.NoError(err)
	if consensus != withdrawalConsensusPRT {
		s.waitForIConsensusInputCursor(s.appName)
	}

	f.first = f.deposit(withdrawalDepositAmount)
	firstInput := s.waitForAcceptedInput(s.appName, f.first.Index.Uint64())
	s.finalizeWithdrawalEpoch(consensus, f.app, firstInput.EpochIndex)
	f.requireFinalized(f.first, true)
	firstFile := f.exportInput(f.first)

	stopSharedNode(s.T())
	f.second = f.deposit(refundDepositAmount)
	f.requireFinalized(f.first, true)
	f.requireFinalized(f.second, false)
	s.requireTokenBalance(f.token, depositor, initialDepositorBalance, "both deposits left the depositor")
	s.requireTokenBalance(f.token, f.app.appAddress,
		tokenAmount(withdrawalDepositAmount+refundDepositAmount), "application holds both deposits")
	before := f.state()
	r.Zero(before.RefundCount.Sign())
	r.False(before.FirstRefunded)
	r.False(before.SecondRefunded)

	// The node has not indexed this deposit yet. Use the actual InputAdded
	// bytes for the pre-foreclosure rejection, then export them through the
	// existing node API after restart for the successful CLI invocation.
	eventFile := f.inputFile("event-input.hex", hexutil.Encode(f.second.Input))
	f.reject(s.appName, f.second.Index.String(), eventFile, "NotForeclosed", before)
	forecloseJSON, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", withdrawalGuardianIndex)},
		"foreclose", s.appName, "--yes", "--json")
	r.NoError(err)
	forecloseReceipt := f.cliReceipt(forecloseJSON)
	foreclosed, err := f.contract.IsForeclosed(&bind.CallOpts{Context: s.ctx, BlockNumber: forecloseReceipt.BlockNumber})
	r.NoError(err)
	r.True(foreclosed)
	f.requireFinalized(f.first, true)
	f.requireFinalized(f.second, false)
	startSharedNode(s.T())
	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName))
	forecloseCancel()
	secondInput := s.waitForAcceptedInput(s.appName, f.second.Index.Uint64())
	r.Equal(model.InputCompletionStatus_Accepted, secondInput.Status,
		"local execution acceptance does not make an input finalized by consensus")
	secondFile := f.exportInput(f.second)
	f.requireFinalized(f.second, false)
	r.Equal(before, f.state(), "foreclosure and node replay do not transfer tokens or issue refunds")

	// Tampering must be checked before a successful refund sets the replay
	// bitmap; afterwards even bad bytes would hit RefundAlreadyIssued first.
	tampered := bytes.Clone(f.second.Input)
	tampered[len(tampered)-1] ^= 1
	tamperedFile := f.inputFile("tampered-input.hex", hexutil.Encode(tampered))
	f.reject(s.appName, f.second.Index.String(), tamperedFile, "InvalidInputHash", before)
	f.reject(f.app.appAddress.Hex(), f.first.Index.String(), firstFile, "CannotRefundFinalizedInput", before)

	// A manual gas limit bypasses estimation, not receipt checks. The invalid
	// refund must mine, fail, and leave the contract and token state unchanged.
	nonceBefore, err := s.client.NonceAt(s.ctx, payer, nil)
	r.NoError(err)
	_, err = f.refund(f.app.appAddress.Hex(), f.first.Index.String(), firstFile, "--gas-limit", "500000")
	r.ErrorContains(err, "receipt status 0")
	nonceAfter, err := s.client.NonceAt(s.ctx, payer, nil)
	r.NoError(err)
	r.Equal(nonceBefore+1, nonceAfter, "the failed refund was mined, not rejected by estimation")
	r.Equal(before, f.state(), "a mined failing refund must preserve token balances and refund state")

	appReference := s.appName
	var transactionFlags []string
	if consensus == withdrawalConsensusQuorum {
		appReference = f.app.appAddress.Hex()
		transactionFlags = []string{"--no-wait", "--gas-limit", "500000"}
	}
	result, err := f.refund(appReference, hexutil.EncodeBig(f.second.Index), secondFile, transactionFlags...)
	r.NoError(err, "refund CLI must submit the original exported input")
	var submitted struct {
		Status string `json:"status"`
	}
	r.NoError(json.Unmarshal([]byte(result), &submitted))
	if consensus == withdrawalConsensusQuorum {
		r.Equal("broadcast", submitted.Status)
	} else {
		r.Equal("mined", submitted.Status)
	}
	receipt := f.cliReceipt(result)
	f.requireRefundReceipt(receipt)
	after := f.state()
	r.Equal(tokenBalanceWithDelta(initialDepositorBalance, refundDepositAmount), after.DepositorBalance)
	r.Equal(tokenAmount(withdrawalDepositAmount), after.ApplicationBalance,
		"the finalized deposit remains in the application")
	r.Equal(before.PayerBalance, after.PayerBalance, "the gas payer does not receive the deposited tokens")
	r.Equal(tokenAmount(1), after.RefundCount)
	r.False(after.FirstRefunded)
	r.True(after.SecondRefunded)
	r.Equal(before.ExecutedOutputCount, after.ExecutedOutputCount)
	r.Equal(before.WithdrawalCount, after.WithdrawalCount)
	f.requireFinalized(f.first, true)
	f.requireFinalized(f.second, false)
	f.reject(f.app.appAddress.Hex(), f.second.Index.String(), secondFile, "RefundAlreadyIssued", after)
}

func (f *refundLifecycleFixture) deposit(amount uint64) *iinputbox.IInputBoxInputAdded {
	s, r := f.s, f.s.Require()
	start, err := s.client.BlockNumber(s.ctx)
	r.NoError(err)
	inputBoxAddr := inputBoxAddress(s.T())
	index := inputBoxInputCount(s.ctx, s.T(), s.client, inputBoxAddr, f.app.appAddress)
	s.depositERC20(s.appName, f.portal, f.token, withdrawalUserIndex, amount)
	end, err := s.client.BlockNumber(s.ctx)
	r.NoError(err)
	inputBox, err := iinputbox.NewIInputBox(inputBoxAddr, s.client)
	r.NoError(err)
	events, err := inputBox.FilterInputAdded(&bind.FilterOpts{Context: s.ctx, Start: start, End: &end},
		[]common.Address{f.app.appAddress}, []*big.Int{new(big.Int).SetUint64(index)})
	r.NoError(err)
	defer func() { r.NoError(events.Close()) }()
	r.True(events.Next(), "deposit must emit InputAdded")
	event := events.Event
	r.Equal(f.app.appAddress, event.AppContract)
	r.Equal(index, event.Index.Uint64())
	r.NotEmpty(event.Input)
	r.False(events.Next(), "one portal deposit must emit exactly one scoped input")
	r.NoError(events.Error())
	return event
}

func (f *refundLifecycleFixture) exportInput(event *iinputbox.IInputBoxInputAdded) string {
	s, r := f.s, f.s.Require()
	out, err := runCLI(s.ctx, "read", "inputs", s.appName, event.Index.String(), "--jsonrpc")
	r.NoError(err, "export the original input through JSON-RPC")
	var response api.SingleResponse[struct {
		RawData string `json:"raw_data"`
	}]
	r.NoError(json.Unmarshal([]byte(out), &response))
	r.Equal(hexutil.Encode(event.Input), response.Data.RawData, "data.raw_data must equal the full InputAdded.input")
	return f.inputFile("exported-input.hex", response.Data.RawData)
}

func (f *refundLifecycleFixture) inputFile(name, input string) string {
	file := filepath.Join(f.s.T().TempDir(), name)
	f.s.Require().NoError(os.WriteFile(file, []byte(input+"\n"), 0600))
	return file
}

func (f *refundLifecycleFixture) requireFinalized(event *iinputbox.IInputBoxInputAdded, want bool) {
	s, r := f.s, f.s.Require()
	head, err := s.client.BlockNumber(s.ctx)
	r.NoError(err)
	opts := &bind.CallOpts{Context: s.ctx, BlockNumber: new(big.Int).SetUint64(head)}
	validatorAddress, err := f.contract.GetOutputsMerkleRootValidator(opts)
	r.NoError(err)
	// wasInputFinalized has the same released interface for Authority,
	// Quorum, and Dave, though their finalization frontiers differ.
	validator, err := iconsensus.NewIConsensus(validatorAddress, s.client)
	r.NoError(err)
	finalized, err := validator.WasInputFinalized(opts, f.app.appAddress, event.Index,
		new(big.Int).SetUint64(event.Raw.BlockNumber))
	r.NoError(err)
	r.Equal(want, finalized, "input %s finalization at block %d", event.Index, head)
}

type refundChainState struct {
	DepositorBalance, ApplicationBalance, PayerBalance *big.Int
	RefundCount, ExecutedOutputCount, WithdrawalCount  *big.Int
	FirstRefunded, SecondRefunded                      bool
}

func (f *refundLifecycleFixture) state() refundChainState {
	s, r := f.s, f.s.Require()
	state := refundChainState{
		DepositorBalance:   s.tokenBalance(f.token, mnemonicAddress(s.T(), withdrawalUserIndex)),
		ApplicationBalance: s.tokenBalance(f.token, f.app.appAddress),
		PayerBalance:       s.tokenBalance(f.token, mnemonicAddress(s.T(), refundPayerIndex)),
	}
	opts := &bind.CallOpts{Context: s.ctx}
	var err error
	state.RefundCount, err = f.contract.GetNumberOfIssuedRefunds(opts)
	r.NoError(err)
	state.ExecutedOutputCount, err = f.contract.GetNumberOfExecutedOutputs(opts)
	r.NoError(err)
	state.WithdrawalCount, err = f.contract.GetNumberOfWithdrawals(opts)
	r.NoError(err)
	state.FirstRefunded, err = f.contract.WasRefundForInputIssued(opts, f.first.Index)
	r.NoError(err)
	state.SecondRefunded, err = f.contract.WasRefundForInputIssued(opts, f.second.Index)
	r.NoError(err)
	return state
}

func (f *refundLifecycleFixture) refund(application, index, inputFile string, flags ...string) (string, error) {
	args := []string{"refund", application, index, "--input-file", inputFile, "--yes", "--json"}
	args = append(args, flags...)
	return runCLIWithEnv(f.s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", refundPayerIndex)},
		args...)
}

func (f *refundLifecycleFixture) reject(application, index, inputFile, revertName string, want refundChainState) {
	_, err := f.refund(application, index, inputFile)
	f.s.Require().Error(err)
	f.s.Require().ErrorContains(err, "decoded revert: "+revertName)
	f.s.Require().Equal(want, f.state(), "rejected refund must preserve balances, counters, and replay flags")
}

func (f *refundLifecycleFixture) cliReceipt(out string) *types.Receipt {
	s, r := f.s, f.s.Require()
	var result struct {
		TransactionHash    common.Hash    `json:"transaction_hash"`
		ApplicationAddress common.Address `json:"application_address"`
		Status             string         `json:"status"`
		BlockNumber        string         `json:"block_number"`
	}
	r.NoError(json.Unmarshal([]byte(out), &result))
	r.Equal(f.app.appAddress, result.ApplicationAddress)
	r.NotEqual(common.Hash{}, result.TransactionHash)
	if result.Status == "mined" {
		// Waiting is now the CLI default: its successful result must already
		// have a successful receipt. Do not hide a missing wait with polling.
		receipt, err := s.client.TransactionReceipt(s.ctx, result.TransactionHash)
		r.NoError(err)
		r.NotNil(receipt)
		r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
		r.Equal(result.TransactionHash, receipt.TxHash)
		r.Equal(hexutil.EncodeBig(receipt.BlockNumber), result.BlockNumber)
		return receipt
	}
	r.Equal("broadcast", result.Status)
	r.Empty(result.BlockNumber, "broadcast-only output must not claim a mined block")
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	var receipt *types.Receipt
	err := pollUntil(ctx, time.Second, func() (bool, error) {
		var err error
		receipt, err = s.client.TransactionReceipt(ctx, result.TransactionHash)
		if errors.Is(err, ethereum.NotFound) {
			return false, nil
		}
		return receipt != nil, err
	})
	r.NoError(err)
	r.Equal(result.TransactionHash, receipt.TxHash)
	r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
	return receipt
}

func (f *refundLifecycleFixture) requireRefundReceipt(receipt *types.Receipt) {
	s, r := f.s, f.s.Require()
	tx, pending, err := s.client.TransactionByHash(s.ctx, receipt.TxHash)
	r.NoError(err)
	r.False(pending)
	sender, err := types.Sender(types.LatestSignerForChainID(s.chainID), tx)
	r.NoError(err)
	r.Equal(mnemonicAddress(s.T(), refundPayerIndex), sender, "a distinct gas payer signed the actual transaction")
	r.Equal(&f.app.appAddress, tx.To())
	appABI, err := iapplication.IApplicationMetaData.GetAbi()
	r.NoError(err)
	wantCall, err := appABI.Pack("issueRefund", f.second.Index, f.second.Input)
	r.NoError(err)
	r.Equal(wantCall, tx.Data())

	token, err := ierc20metadata.NewIERC20Metadata(f.token, s.client)
	r.NoError(err)
	tokenABI, err := ierc20metadata.IERC20MetadataMetaData.GetAbi()
	r.NoError(err)
	refunds, transfers := 0, 0
	for _, raw := range receipt.Logs {
		r.NotNil(raw)
		if raw.Address == f.app.appAddress && len(raw.Topics) > 0 && raw.Topics[0] == appABI.Events["RefundIssued"].ID {
			event, err := f.contract.ParseRefundIssued(*raw)
			r.NoError(err)
			r.Equal(f.second.Index, event.InputIndex)
			r.Equal(f.second.Input, event.Input)
			r.Equal(receipt.TxHash, event.Raw.TxHash)
			r.Equal(receipt.BlockNumber.Uint64(), event.Raw.BlockNumber)
			f.requireRefundOutput(event.Output, receipt.BlockNumber)
			refunds++
		}
		if raw.Address == f.token && len(raw.Topics) > 0 && raw.Topics[0] == tokenABI.Events["Transfer"].ID {
			event, err := token.ParseTransfer(*raw)
			r.NoError(err)
			r.Equal(f.app.appAddress, event.From)
			r.Equal(mnemonicAddress(s.T(), withdrawalUserIndex), event.To)
			r.Equal(tokenAmount(refundDepositAmount), event.Value)
			transfers++
		}
	}
	r.Equal(1, refunds, "exactly one RefundIssued must be emitted")
	r.Equal(1, transfers, "exactly one ERC-20 transfer must pay the original depositor")
}

func (f *refundLifecycleFixture) requireRefundOutput(raw []byte, block *big.Int) {
	s, r := f.s, f.s.Require()
	outputABI, err := outputs.OutputsMetaData.GetAbi()
	r.NoError(err)
	output, err := api.DecodeOutput(&model.Output{RawData: raw}, outputABI)
	r.NoError(err)
	r.Equal("DelegateCallVoucher", output.DecodedData.Type)
	destination := common.HexToAddress(output.DecodedData.Destination)
	code, err := s.client.CodeAt(s.ctx, destination, block)
	r.NoError(err)
	r.NotEmpty(code, "the canonical ERC-20 refund delegates to a deployed safe-transfer contract")
	// ISafeErc20Transfer.safeTransfer(token, to, value), as encoded by the
	// released LibErc20Deposit.buildRefund. Only the deployed destination is
	// taken from the event; token, recipient, amount, and full encoding are checked.
	payload := crypto.Keccak256([]byte("safeTransfer(address,address,uint256)"))[:refundSelectorBytes]
	payload = append(payload, common.LeftPadBytes(f.token.Bytes(), common.HashLength)...)
	payload = append(payload, common.LeftPadBytes(mnemonicAddress(s.T(), withdrawalUserIndex).Bytes(), common.HashLength)...)
	payload = append(payload, common.LeftPadBytes(tokenAmount(refundDepositAmount).Bytes(), common.HashLength)...)
	r.Equal(hexutil.Encode(payload), output.DecodedData.Payload)
	want, err := outputABI.Pack("DelegateCallVoucher", destination, payload)
	r.NoError(err)
	r.Equal(want, raw, "RefundIssued.output must contain exactly the canonical transfer output")
}

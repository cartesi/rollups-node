// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

// ForecloseSuite exercises the full foreclosure lifecycle:
//
//  1. Deploy an Authority app where the guardian wallet differs from the
//     node's default signer (FoundryMnemonic, account index 1) and the
//     withdrawal output builder is the devnet-deployed UsdWithdrawalOutputBuilder
//     (address surfaced via CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER).
//  2. Send one input through, claim accepted as usual.
//  3. The guardian (account index 1) calls IApplication.foreclose() via
//     `cartesi-rollups-cli foreclose`.
//  4. The evmreader observes the Foreclosure() event and records
//     (foreclose_block, foreclose_transaction) on the application row.
//
// Foreclosure records foreclose_block for a normal app while leaving its
// health status OK and enabled=true so evmreader continues observing
// post-foreclosure activity (drive-prove discovery, then Withdrawal
// indexing). This suite asserts the foreclose-observed signal and the
// operator-visible status split.
type ForecloseSuite struct {
	suite.Suite
	LogChecker
	ctx     context.Context
	cancel  context.CancelFunc
	appName string
}

func TestForeclose(t *testing.T) {
	suite.Run(t, new(ForecloseSuite))
}

func (s *ForecloseSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 10*time.Minute)
}

func (s *ForecloseSuite) TearDownSuite() {
	s.cancel()
}

func (s *ForecloseSuite) SetupTest() {
	s.StartLogCapture()
	s.appName = ""
}

func (s *ForecloseSuite) TearDownTest() {
	if s.appName != "" {
		_ = disableApplication(s.ctx, s.appName) //nolint:errcheck
	}
	s.CheckLogs(s.T())
}

// TestForecloseLifecycle deploys an authority app with the second derived
// mnemonic account as guardian, sends an input, confirms the claim is
// accepted, then forecloses on-chain via the CLI and waits for the node to
// record the foreclosure marker. The app stays enabled, keeps its health
// status OK, and has foreclose_block set; a terminal health status
// (DIVERGED/CORRUPTED) is reserved for genuine divergence or corruption.
func (s *ForecloseSuite) TestForecloseLifecycle() {
	require := s.Require()

	builderEnv := os.Getenv("CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS")
	require.NotEmpty(builderEnv,
		"CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS must be set (run `eval $(make env)`)")

	// Derive the guardian wallet from the same mnemonic the node uses, but
	// at index 1 so it's a distinct account from the node's default signer.
	const guardianIndex = 1
	guardianKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, guardianIndex)
	require.NoError(err, "derive guardian key")
	guardianAddr := crypto.PubkeyToAddress(guardianKey.PublicKey)
	s.T().Logf("Guardian address (mnemonic[%d]): %s", guardianIndex, guardianAddr.Hex())
	s.T().Logf("Withdrawal output builder:        %s", builderEnv)

	withdrawalConfigJSON := fmt.Sprintf(`{
		"guardian": "%s",
		"log2_leaves_per_account": 0,
		"log2_max_num_of_accounts": 20,
		"accounts_drive_start_index": 33554432,
		"withdrawal_output_builder": "%s"
	}`, guardianAddr.Hex(), builderEnv)
	withdrawalConfigJSON = strings.ReplaceAll(withdrawalConfigJSON, "\n", "")
	withdrawalConfigJSON = strings.ReplaceAll(withdrawalConfigJSON, "\t", "")

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("foreclose-test")

	// Phase 1 — normal lifecycle: deploy + send + claim accepted.
	runEchoLifecycleTest(s.ctx, s.T(), require, echoLifecycleConfig{
		AppName:  s.appName,
		DappPath: dappPath,
		Payload:  "hello cartesi (foreclose)",
		ExtraDeployArgs: []string{
			"--withdrawal-config", withdrawalConfigJSON,
		},
	})
	s.T().Log("=== Pre-foreclosure lifecycle complete ===")

	// Phase 2 — guardian forecloses via CLI. Use CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX
	// to switch the signer from the node's default (index 0) to the guardian
	// (index 1). The node will pick up the Foreclosure event on the next tick.
	s.T().Logf("Foreclosing %s with guardian wallet (mnemonic[%d])", s.appName, guardianIndex)
	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", guardianIndex)},
		"foreclose", s.appName, "--yes", "--json",
	)
	require.NoError(err, "guardian foreclose CLI call: %s", out)
	s.T().Logf("    foreclose tx: %s", strings.TrimSpace(out))

	// Phase 3 — wait for the node to record the foreclosure marker. evmreader
	// detects Foreclosure within a few ticks of its polling cadence and
	// writes (foreclose_block, foreclose_transaction) to the application
	// row. The `app status` CLI now emits a "Foreclose block:" line when
	// app.ForecloseBlock != 0, which is what waitForApplicationForeclosed
	// polls for.
	const observeTimeout = 30 * time.Second
	stateCtx, stateCancel := context.WithTimeout(s.ctx, observeTimeout)
	defer stateCancel()
	require.NoError(waitForApplicationForeclosed(stateCtx, s.T(), s.appName),
		"node did not record foreclose_block after guardian foreclose()")

	// Sanity: foreclosure stops normal work but does not disable L1
	// observation. The app keeps its health status OK, remains enabled, and
	// the node continues observing post-foreclosure events (drive-prove,
	// withdrawals).
	status, err := readApplicationStatus(s.ctx, s.appName)
	require.NoError(err, "read app status after foreclosure")
	require.Equal("OK", firstStatusLine(status),
		"ordinary foreclosure keeps health OK; foreclosure is recorded in foreclose_block")
	require.Contains(status, "Enabled: true",
		"ordinary foreclosure must keep the app enabled for L1 observation")
	require.NotContains(status, "DIVERGED",
		"foreclosure must not transition the app to a terminal health status (DIVERGED/CORRUPTED)")
	require.Contains(status, "Foreclose block:",
		"app status must surface the recorded foreclose_block")
	require.Contains(status, "Foreclose transaction:",
		"app status must surface the recorded foreclose_transaction")

	input, err := readInput(s.ctx, s.appName, 0)
	require.NoError(err, "read input 0 to find its epoch")
	epoch, err := readEpoch(s.ctx, s.appName, input.EpochIndex)
	require.NoError(err, "read epoch %d after foreclosure", input.EpochIndex)
	require.Equal(model.EpochStatus_ClaimAccepted, epoch.Status,
		"already accepted pre-foreclosure claims must remain CLAIM_ACCEPTED")

	s.T().Logf("Final app status:\n%s", status)
	s.T().Log("=== Foreclosure lifecycle complete ===")
}

func (s *ForecloseSuite) TestAuthorityForecloseWithoutInputs() {
	require := s.Require()

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("foreclose-no-input")

	_, err := deployApplication(s.ctx, s.appName, dappPath,
		"--salt", uniqueSalt(),
		"--withdrawal-config", withdrawalConfigJSON,
	)
	require.NoError(err, "deploy app")

	require.NoError(guardianForeclose(s.ctx, s.appName, guardianIndex),
		"guardian should be able to foreclose an app with no inputs")

	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	require.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName),
		"node did not record foreclosure")
	forecloseCancel()

	status, err := readApplicationStatus(s.ctx, s.appName)
	require.NoError(err, "read app status")
	require.Equal("OK", firstStatusLine(status),
		"ordinary no-input foreclosure keeps health OK; foreclosure is recorded in foreclose_block")
	require.Contains(status, "Enabled: true",
		"foreclosed app remains enabled for post-foreclosure L1 observation")

	_, err = readInput(s.ctx, s.appName, 0)
	require.Error(err, "no-input foreclosure should not create synthetic inputs")
	require.True(isCLIExitError(err), "missing input should be reported by the CLI")
}

func (s *ForecloseSuite) TestAuthorityForecloseBeforeEpochEndMarksEpochForeclosed() {
	require := s.Require()

	builderEnv := os.Getenv("CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS")
	require.NotEmpty(builderEnv,
		"CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS must be set (run `eval $(make env)`)")

	const guardianIndex = 1
	guardianKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, guardianIndex)
	require.NoError(err, "derive guardian key")
	guardianAddr := crypto.PubkeyToAddress(guardianKey.PublicKey)

	withdrawalConfigJSON := strings.NewReplacer("\n", "", "\t", "").Replace(fmt.Sprintf(`{
		"guardian": "%s",
		"log2_leaves_per_account": 0,
		"log2_max_num_of_accounts": 20,
		"accounts_drive_start_index": 33554432,
		"withdrawal_output_builder": "%s"
	}`, guardianAddr.Hex(), builderEnv))

	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.Dial(endpoint)
	require.NoError(err, "dial ethclient")
	defer client.Close()

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("foreclose-open-epoch")

	appAddr, err := deployApplication(s.ctx, s.appName, dappPath,
		"--salt", uniqueSalt(),
		"--withdrawal-config", withdrawalConfigJSON,
	)
	require.NoError(err, "deploy app")
	require.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei),
		"fund application contract")

	currentBlock, err := client.BlockNumber(s.ctx)
	require.NoError(err, "read current block")
	const epochLength = 10
	nextInputBlock := currentBlock + 1
	if mod := nextInputBlock % epochLength; mod > 6 { //nolint:mnd
		require.NoError(anvilMine(s.ctx, int(epochLength-mod)),
			"mine to place the next input near the start of an epoch")
	}

	inputIndex, inputBlock, err := sendInput(s.ctx, s.appName, "foreclose while epoch is open")
	require.NoError(err, "send input")
	require.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), s.appName, inputIndex)
	processCancel()
	require.NoError(err, "wait for input processing")
	require.Equal(model.InputCompletionStatus_Accepted, input.Status)

	epoch, err := readEpoch(s.ctx, s.appName, input.EpochIndex)
	require.NoError(err, "read input epoch")
	require.Less(inputBlock, epoch.LastBlock,
		"test setup must foreclose before the deterministic epoch end")

	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", guardianIndex)},
		"foreclose", s.appName, "--yes", "--json",
	)
	require.NoError(err, "guardian foreclose CLI call: %s", out)

	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	require.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName),
		"node did not record foreclosure")
	forecloseCancel()

	epochCtx, epochCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err = waitForEpochStatus(epochCtx, s.T(), s.appName, input.EpochIndex, model.EpochStatus_ClaimForeclosed)
	epochCancel()
	require.NoError(err, "open epoch should become CLAIM_FORECLOSED")
	require.NotEqual(model.EpochStatus_ClaimAccepted, epoch.Status,
		"epoch foreclosed before deterministic end must not be accepted after foreclosure")
}

func (s *ForecloseSuite) TestOutputExecutionAfterForeclosureIsRecorded() {
	require := s.Require()

	builderEnv := os.Getenv("CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS")
	require.NotEmpty(builderEnv,
		"CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS must be set (run `eval $(make env)`)")

	const guardianIndex = 1
	guardianKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, guardianIndex)
	require.NoError(err, "derive guardian key")
	guardianAddr := crypto.PubkeyToAddress(guardianKey.PublicKey)

	withdrawalConfigJSON := strings.NewReplacer("\n", "", "\t", "").Replace(fmt.Sprintf(`{
		"guardian": "%s",
		"log2_leaves_per_account": 0,
		"log2_max_num_of_accounts": 20,
		"accounts_drive_start_index": 33554432,
		"withdrawal_output_builder": "%s"
	}`, guardianAddr.Hex(), builderEnv))

	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.Dial(endpoint)
	require.NoError(err, "dial ethclient")
	defer client.Close()

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("foreclose-output-exec")

	appAddr, err := deployApplication(s.ctx, s.appName, dappPath,
		"--salt", uniqueSalt(),
		"--withdrawal-config", withdrawalConfigJSON,
	)
	require.NoError(err, "deploy app")
	require.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei),
		"fund application contract")

	inputIndex, _, err := sendInput(s.ctx, s.appName, "execute after foreclosure")
	require.NoError(err, "send input")
	require.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), s.appName, inputIndex)
	processCancel()
	require.NoError(err, "wait for input processing")
	require.Equal(model.InputCompletionStatus_Accepted, input.Status)

	outputsResp, err := readOutputs(s.ctx, s.appName)
	require.NoError(err, "read outputs")
	require.Len(outputsResp.Data, echoOutputsPerInput)

	var voucherIdx uint64
	voucherFound := false
	for _, out := range outputsResp.Data {
		if out.DecodedData != nil && out.DecodedData.Type == "Voucher" {
			voucherIdx = out.Index
			voucherFound = true
			break
		}
	}
	require.True(voucherFound, "voucher output not found")

	epoch, err := readEpoch(s.ctx, s.appName, outputsResp.Data[0].EpochIndex)
	require.NoError(err, "read epoch")
	currentBlock, err := client.BlockNumber(s.ctx)
	require.NoError(err, "read current block")
	if currentBlock <= epoch.LastBlock {
		require.NoError(anvilMine(s.ctx, int(epoch.LastBlock-currentBlock+1)), //nolint:gosec
			"mine past epoch last block")
	}

	claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), s.appName, epoch.Index, model.EpochStatus_ClaimAccepted)
	claimCancel()
	require.NoError(err, "wait for claim accepted")

	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", guardianIndex)},
		"foreclose", s.appName, "--yes", "--json",
	)
	require.NoError(err, "guardian foreclose CLI call: %s", out)

	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	require.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName),
		"node did not record foreclosure")
	forecloseCancel()

	txHash, err := executeOutput(s.ctx, s.appName, voucherIdx)
	require.NoError(err, "execute accepted voucher after foreclosure")
	require.NotEmpty(txHash)

	execCtx, execCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	err = waitForExecutionRecorded(execCtx, s.T(), s.appName, voucherIdx)
	execCancel()
	require.NoError(err, "wait for post-foreclosure execution tx hash in DB")
}

func (s *ForecloseSuite) TestSameBlockInputForecloseAndOutputOrdering() {
	require := s.Require()

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	client := newIntegrationEthClient(s.ctx, s.T())
	defer client.Close()

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("foreclose-same-block")

	appAddrString, err := deployApplication(s.ctx, s.appName, dappPath,
		"--salt", uniqueSalt(),
		"--withdrawal-config", withdrawalConfigJSON,
	)
	require.NoError(err, "deploy app")
	require.NoError(anvilSetBalance(s.ctx, appAddrString, oneEtherWei),
		"fund application contract")
	appAddr := common.HexToAddress(appAddrString)

	inputIndex, _, err := sendInput(s.ctx, s.appName, "same-block setup")
	require.NoError(err, "send setup input")
	require.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	setupInput, err := waitForInputProcessed(processCtx, s.T(), s.appName, inputIndex)
	processCancel()
	require.NoError(err, "wait for setup input")
	require.Equal(model.InputCompletionStatus_Accepted, setupInput.Status)

	outputsResp, err := readOutputs(s.ctx, s.appName)
	require.NoError(err, "read outputs")
	require.Len(outputsResp.Data, echoOutputsPerInput)

	var voucherIdx uint64
	voucherFound := false
	for _, output := range outputsResp.Data {
		if output.DecodedData != nil && output.DecodedData.Type == "Voucher" {
			voucherIdx = output.Index
			voucherFound = true
			break
		}
	}
	require.True(voucherFound, "voucher output not found")

	epoch, err := readEpoch(s.ctx, s.appName, setupInput.EpochIndex)
	require.NoError(err, "read setup epoch")
	currentBlock, err := client.BlockNumber(s.ctx)
	require.NoError(err, "read current block")
	if currentBlock <= epoch.LastBlock {
		require.NoError(anvilMine(s.ctx, int(epoch.LastBlock-currentBlock+1)), //nolint:gosec
			"mine past setup epoch")
	}

	claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), s.appName, epoch.Index, model.EpochStatus_ClaimAccepted)
	claimCancel()
	require.NoError(err, "wait for setup claim accepted")

	voucher, err := readOutput(s.ctx, s.appName, voucherIdx)
	require.NoError(err, "read voucher after claim proof generation")
	require.NotEmpty(voucher.OutputHashesSiblings, "voucher should have proof siblings before L1 execution")

	inputBoxAddr := inputBoxAddress(s.T())
	nextInputIndex := inputBoxInputCount(s.ctx, s.T(), client, inputBoxAddr, appAddr)
	require.Equal(uint64(1), nextInputIndex, "test setup should have exactly one pre-existing input")

	inputBox, err := iinputbox.NewIInputBox(inputBoxAddr, client)
	require.NoError(err, "bind input box")
	app, err := iapplication.NewIApplication(appAddr, client)
	require.NoError(err, "bind application")

	nextOpts := func(signerIndex uint32, gasPriceGwei int64) *bind.TransactOpts {
		opts := *transactorForMnemonicIndex(s.ctx, s.T(), client, signerIndex)
		opts.GasLimit = 2_000_000 //nolint:mnd
		opts.GasPrice = new(big.Int).Mul(
			big.NewInt(gasPriceGwei),
			big.NewInt(1_000_000_000), //nolint:mnd
		)
		return &opts
	}

	setAnvilAutomine(s.ctx, s.T(), false)
	defer setAnvilAutomine(s.ctx, s.T(), true)
	// The devnet runs anvil with --block-time 1, so interval mining keeps
	// producing blocks on a one-second timer even with automine off — which
	// would split the batch below across consecutive blocks. Turn interval
	// mining off for the batch and restore it after. Safe because the
	// integration suites run sequentially (no t.Parallel), so no other test
	// depends on block production during this window.
	setAnvilIntervalMining(s.ctx, s.T(), 0)
	defer setAnvilIntervalMining(s.ctx, s.T(), 1)

	// Use separate funded devnet accounts. With Anvil automine disabled, this
	// lets one manual mine include all pending txs in a single block; using
	// one account with sequential nonces can be split across blocks by the
	// devnet mempool.
	preInputTx, err := inputBox.AddInput(
		nextOpts(2, 10), appAddr, []byte("same-block before foreclose")) //nolint:mnd
	require.NoError(err, "send same-block pre-foreclosure input")
	forecloseTx, err := app.Foreclose(nextOpts(guardianIndex, 9)) //nolint:mnd
	require.NoError(err, "send same-block foreclosure")
	postInputTx, err := inputBox.AddInput(
		nextOpts(3, 8), appAddr, []byte("same-block after foreclose")) //nolint:mnd
	require.NoError(err, "send same-block post-foreclosure input")
	execTx, err := app.ExecuteOutput(
		nextOpts(4, 7), voucher.RawData, outputValidityProof(voucher)) //nolint:mnd
	require.NoError(err, "send same-block post-foreclosure output execution")

	require.NoError(anvilMine(s.ctx, 1), "mine same-block transaction batch")

	receiptCtx, receiptCancel := context.WithTimeout(s.ctx, 30*time.Second)
	preInputReceipt := waitReceipt(receiptCtx, s.T(), client, preInputTx)
	forecloseReceipt := waitReceipt(receiptCtx, s.T(), client, forecloseTx)
	postInputReceipt := waitReceipt(receiptCtx, s.T(), client, postInputTx)
	execReceipt := waitReceipt(receiptCtx, s.T(), client, execTx)
	receiptCancel()

	require.Equal(uint64(1), preInputReceipt.Status,
		"input before foreclose transaction in the same block should succeed")
	require.Equal(uint64(1), forecloseReceipt.Status, "foreclose transaction should succeed")
	require.Equal(uint64(0), postInputReceipt.Status,
		"input after foreclose transaction in the same block should revert")
	require.Equal(uint64(1), execReceipt.Status,
		"output execution after foreclose transaction in the same block should still succeed")
	require.Equal(preInputReceipt.BlockNumber.Uint64(), forecloseReceipt.BlockNumber.Uint64(),
		"pre-input and foreclose must be mined in the same block")
	require.Equal(forecloseReceipt.BlockNumber.Uint64(), postInputReceipt.BlockNumber.Uint64(),
		"post-input and foreclose must be mined in the same block")
	require.Less(preInputReceipt.TransactionIndex, forecloseReceipt.TransactionIndex,
		"test setup requires pre-input before foreclose in block order")
	require.Less(forecloseReceipt.TransactionIndex, postInputReceipt.TransactionIndex,
		"test setup requires post-input after foreclose in block order")
	require.Less(forecloseReceipt.TransactionIndex, execReceipt.TransactionIndex,
		"test setup requires output execution after foreclose in block order")

	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	require.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName),
		"node did not record foreclosure")
	forecloseCancel()

	inputCtx, inputCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	preInput, err := waitForInputProcessed(inputCtx, s.T(), s.appName, nextInputIndex)
	inputCancel()
	require.NoError(err, "pre-foreclosure same-block input should be indexed and processed")
	require.Equal(model.InputCompletionStatus_Accepted, preInput.Status)

	// The epoch holding the same-block pre-foreclosure input straddles the
	// foreclosure block (first_block <= foreclose_block <= last_block). Its
	// claim can never be accepted once the app is foreclosed, so the drain must
	// terminalize it to CLAIM_FORECLOSED rather than leave it dangling — the H1
	// boundary, end to end: a valid input at exactly the foreclosure block.
	epochCtx, epochCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(
		epochCtx, s.T(), s.appName, preInput.EpochIndex, model.EpochStatus_ClaimForeclosed)
	epochCancel()
	require.NoError(err,
		"straddling epoch with the same-block pre-foreclosure input must terminalize to CLAIM_FORECLOSED")

	_, err = readInput(s.ctx, s.appName, nextInputIndex+1)
	require.Error(err, "post-foreclosure same-block input should not be indexed")
	require.True(isCLIExitError(err), "missing post-foreclosure input should be reported by the CLI")

	execCtx, execCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	err = waitForExecutionRecorded(execCtx, s.T(), s.appName, voucherIdx)
	execCancel()
	require.NoError(err, "wait for same-block post-foreclosure output execution in DB")
}

// readApplicationStatus invokes `cartesi-rollups-cli app status <app>` and
// returns the raw output (status on first line; "Reason: ..." when the status
// has one).
func readApplicationStatus(ctx context.Context, appName string) (string, error) {
	return runCLI(ctx, "app", "status", appName)
}

func firstStatusLine(out string) string {
	return strings.TrimSpace(strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]) //nolint:mnd
}

// waitForApplicationStatus polls `app status` until the first line equals
// the wanted status or the context is cancelled.
func waitForApplicationStatus(
	ctx context.Context,
	t testing.TB,
	appName string,
	want string,
) error {
	var lastErr error
	err := pollUntil(ctx, 3*time.Second, func() (bool, error) {
		out, err := readApplicationStatus(ctx, appName)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				t.Logf("    waiting for state %s (poll error: %v)", want, err)
				return false, nil
			}
			return false, fmt.Errorf("poll app status: %w", err)
		}
		line := firstStatusLine(out)
		if line == want {
			return true, nil
		}
		t.Logf("    waiting for state %s (have %s)", want, line)
		return false, nil
	})
	if err != nil && lastErr != nil {
		return fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	return err
}

// waitForApplicationForeclosed polls `app status` until the output contains
// a "Foreclose block:" line (emitted by app/status/status.go when
// app.ForecloseBlock != 0). This is the gating signal for any test that drives
// a guardian foreclose() and waits for the node to observe it.
func waitForApplicationForeclosed(
	ctx context.Context,
	t testing.TB,
	appName string,
) error {
	var lastErr error
	err := pollUntil(ctx, 3*time.Second, func() (bool, error) {
		out, err := readApplicationStatus(ctx, appName)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				t.Logf("    waiting for foreclosure on %s (poll error: %v)", appName, err)
				return false, nil
			}
			return false, fmt.Errorf("poll app status: %w", err)
		}
		if strings.Contains(out, "Foreclose block:") {
			return true, nil
		}
		t.Logf("    waiting for foreclosure on %s (status: %q)",
			appName, strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]) //nolint:mnd
		return false, nil
	})
	if err != nil && lastErr != nil {
		return fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	return err
}

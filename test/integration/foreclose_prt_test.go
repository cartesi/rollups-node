// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ForeclosePrtSuite is the PRT-consensus counterpart to ForecloseSuite.
// Authority and PRT route foreclosed apps through structurally-different
// code paths (claimer's processForeclosedApps vs prt's handleForeclosedApp,
// each with its own drain gate), but the operator-visible outcome must be
// identical: the app keeps its health status OK, remains enabled for L1
// observation, records foreclose_block, and evmreader continues observing
// post-foreclosure activity. A regression in either service's per-consensus
// drain path would not be caught by the Authority foreclose test.
type ForeclosePrtSuite struct {
	suite.Suite
	LogChecker
	ctx       context.Context
	cancel    context.CancelFunc
	appName   string
	ethClient *ethclient.Client
}

func TestForeclosePrt(t *testing.T) {
	suite.Run(t, new(ForeclosePrtSuite))
}

func (s *ForeclosePrtSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 20*time.Minute)

	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.Dial(endpoint)
	s.Require().NoError(err, "dial ethclient")
	s.ethClient = client
}

func (s *ForeclosePrtSuite) TearDownSuite() {
	s.cancel()
	s.ethClient.Close()
}

func (s *ForeclosePrtSuite) SetupTest() {
	s.StartLogCapture()
	s.SetExpectedLogs(s.T(), ExpectedLog{
		Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
		Level:   LevelError,
		Reason:  "transient EVM reader race against Anvil during rapid PRT block mining",
	})
	s.appName = ""
}

func (s *ForeclosePrtSuite) TearDownTest() {
	if s.appName != "" {
		_ = disableApplication(s.ctx, s.appName) //nolint:errcheck
	}
	s.CheckLogs(s.T())
}

// TestForeclosePrtLifecycle deploys a PRT app with the second derived
// mnemonic account as guardian, runs the normal PRT lifecycle (input,
// tournament settlement, claim accepted), then forecloses on-chain via the
// CLI. The node must record foreclose_block while keeping health status OK and
// enabled=true for L1 observation — same operator-visible contract as
// the Authority path, but routed through prt.handleForeclosedApp rather than
// claimer.processForeclosedApps.
func (s *ForeclosePrtSuite) TestForeclosePrtLifecycle() {
	r := s.Require()

	builderEnv := os.Getenv("CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS")
	r.NotEmpty(builderEnv,
		"CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS must be set (run `eval $(make env)`)")

	const guardianIndex = 1
	guardianKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, guardianIndex)
	r.NoError(err, "derive guardian key")
	guardianAddr := crypto.PubkeyToAddress(guardianKey.PublicKey)
	s.T().Logf("Guardian address (mnemonic[%d]): %s", guardianIndex, guardianAddr.Hex())

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
	s.appName = uniqueAppName("foreclose-prt")

	// Phase 1 — full PRT lifecycle (input + tournament settlement + claim).
	// PreClaimHook settles epoch 0 (sealed-empty at deploy) and epoch 1
	// (carrying our input), matching the existing TestEchoPrtLifecycle.
	ethClient := s.ethClient
	runEchoLifecycleTest(s.ctx, s.T(), r, echoLifecycleConfig{
		AppName:  s.appName,
		DappPath: dappPath,
		Payload:  "hello cartesi (foreclose prt)",
		ExtraDeployArgs: []string{
			"--prt",
			"--withdrawal-config", withdrawalConfigJSON,
		},
		PreClaimHook: func(ctx context.Context, t testing.TB, r *require.Assertions, appName string) {
			settleTournament(ctx, t, r, ethClient, appName, 0)
			settleTournament(ctx, t, r, ethClient, appName, 1)
		},
	})
	s.T().Log("=== Pre-foreclosure PRT lifecycle complete ===")

	// Phase 2 — guardian forecloses via CLI (signer = mnemonic[1]).
	s.T().Logf("Foreclosing %s with guardian wallet (mnemonic[%d])", s.appName, guardianIndex)
	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", guardianIndex)},
		"foreclose", s.appName, "--yes", "--json",
	)
	r.NoError(err, "guardian foreclose CLI call: %s", out)
	s.T().Logf("    foreclose tx: %s", strings.TrimSpace(out))

	// Phase 3 — wait for the foreclose marker. evmreader is consensus-
	// agnostic; the marker lands the same way regardless of Authority vs
	// PRT, but the next services that read it differ.
	const observeTimeout = 30 * time.Second
	stateCtx, stateCancel := context.WithTimeout(s.ctx, observeTimeout)
	defer stateCancel()
	r.NoError(waitForApplicationForeclosed(stateCtx, s.T(), s.appName),
		"node did not record foreclose_block after guardian foreclose() on PRT app")

	// Sanity: the PRT service's handleForeclosedApp must NOT drive the app to
	// a terminal health status. The operator-visible contract is identical to
	// the Authority path: health status OK, enabled for L1 observation, and the
	// foreclose marker surfaced in `app status`.
	status, err := readApplicationStatus(s.ctx, s.appName)
	r.NoError(err, "read app status after foreclosure")
	r.Equal("OK", firstStatusLine(status),
		"ordinary foreclosure keeps a PRT app's health OK; foreclosure is recorded in foreclose_block")
	r.Contains(status, "Enabled: true",
		"PRT app should stay enabled for L1 observation after foreclosure")
	r.NotContains(status, "DIVERGED",
		"foreclosure must not transition a PRT app to a terminal health status (DIVERGED/CORRUPTED)")
	r.Contains(status, "Foreclose block:",
		"app status must surface the recorded foreclose_block")
	r.Contains(status, "Foreclose transaction:",
		"app status must surface the recorded foreclose_transaction")

	s.T().Logf("Final app status:\n%s", status)
	s.T().Log("=== PRT foreclosure lifecycle complete ===")
}

func (s *ForeclosePrtSuite) TestForeclosePrtWithoutInputs() {
	r := s.Require()

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("foreclose-prt-no-input")

	_, err := deployApplication(s.ctx, s.appName, dappPath,
		"--salt", uniqueSalt(),
		"--prt",
		"--withdrawal-config", withdrawalConfigJSON,
	)
	r.NoError(err, "deploy PRT app")

	r.NoError(guardianForeclose(s.ctx, s.appName, guardianIndex),
		"guardian should be able to foreclose a PRT app with no user inputs")

	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName),
		"node did not record PRT foreclosure")
	forecloseCancel()

	status, err := readApplicationStatus(s.ctx, s.appName)
	r.NoError(err, "read app status")
	r.Equal("OK", firstStatusLine(status),
		"ordinary no-input PRT foreclosure keeps health OK; foreclosure is recorded in foreclose_block")
	r.Contains(status, "Enabled: true",
		"foreclosed PRT app remains enabled for post-foreclosure L1 observation")
}

func (s *ForeclosePrtSuite) TestForeclosePrtBeforeTournamentSettlementStopsParticipation() {
	r := s.Require()

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("foreclose-prt-mid-tournament")

	appAddr, err := deployApplication(s.ctx, s.appName, dappPath,
		"--salt", uniqueSalt(),
		"--prt",
		"--withdrawal-config", withdrawalConfigJSON,
	)
	r.NoError(err, "deploy PRT app")
	r.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei),
		"fund application contract")

	inputIndex, _, err := sendInput(s.ctx, s.appName, "foreclose PRT before settlement")
	r.NoError(err, "send input")
	r.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), s.appName, inputIndex)
	processCancel()
	r.NoError(err, "wait for input processing")
	r.Equal(model.InputCompletionStatus_Accepted, input.Status)

	// PRT starts with an empty epoch 0. Settle it so the service can create
	// the root tournament for the input-carrying epoch, then foreclose before
	// that tournament reaches a winner.
	if input.EpochIndex > 0 {
		settleTournament(s.ctx, s.T(), r, s.ethClient, s.appName, 0)
	}
	tournament := waitForTournamentAndCommitment(s.ctx, s.T(), r, s.appName, input.EpochIndex)

	r.NoError(guardianForeclose(s.ctx, s.appName, guardianIndex),
		"guardian foreclose while PRT tournament is unresolved")

	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName),
		"node did not record PRT foreclosure")
	forecloseCancel()

	blocksMined, err := mineForTournamentTimeout(s.ctx, s.ethClient, tournament.Address)
	r.NoError(err, "mine past unresolved tournament timeout")
	s.T().Logf("    mined %d blocks after foreclosure; PRT service must not settle", blocksMined)

	// The mid-tournament claim was never accepted on chain and can never be now
	// that the app is foreclosed, so the drain terminalizes it to CLAIM_FORECLOSED
	// — not CLAIM_ACCEPTED, and not left stuck at CLAIM_COMPUTED.
	claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), s.appName, input.EpochIndex, model.EpochStatus_ClaimForeclosed)
	claimCancel()
	r.NoError(err, "foreclosed PRT epoch must be terminalized to CLAIM_FORECLOSED")

	status, err := readApplicationStatus(s.ctx, s.appName)
	r.NoError(err, "read app status")
	r.Equal("OK", firstStatusLine(status),
		"ordinary mid-tournament PRT foreclosure keeps health OK; foreclosure is recorded in foreclose_block")
}

func (s *ForeclosePrtSuite) TestForeclosePrtOutputExecutionAfterForeclosureIsRecorded() {
	r := s.Require()

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("foreclose-prt-output")

	appAddr, err := deployApplication(s.ctx, s.appName, dappPath,
		"--salt", uniqueSalt(),
		"--prt",
		"--withdrawal-config", withdrawalConfigJSON,
	)
	r.NoError(err, "deploy PRT app")
	r.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei),
		"fund application contract")

	inputIndex, _, err := sendInput(s.ctx, s.appName, "foreclose PRT output execution")
	r.NoError(err, "send input")
	r.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), s.appName, inputIndex)
	processCancel()
	r.NoError(err, "wait for PRT input")
	r.Equal(model.InputCompletionStatus_Accepted, input.Status)

	outputsResp, err := readOutputs(s.ctx, s.appName)
	r.NoError(err, "read outputs")
	r.Len(outputsResp.Data, echoOutputsPerInput)
	voucherIdx := firstVoucherOutputIndex(s.T(), outputsResp.Data)

	for epochIndex := uint64(0); epochIndex <= input.EpochIndex; epochIndex++ {
		settleTournament(s.ctx, s.T(), r, s.ethClient, s.appName, epochIndex)
	}

	claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), s.appName, input.EpochIndex, model.EpochStatus_ClaimAccepted)
	claimCancel()
	r.NoError(err, "PRT input epoch should reach CLAIM_ACCEPTED")

	r.NoError(guardianForeclose(s.ctx, s.appName, guardianIndex), "guardian foreclose")
	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName),
		"node did not record PRT foreclosure")
	forecloseCancel()

	txHash, err := executeOutput(s.ctx, s.appName, voucherIdx)
	r.NoError(err, "execute accepted PRT voucher after foreclosure")
	r.NotEmpty(txHash)

	execCtx, execCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	err = waitForExecutionRecorded(execCtx, s.T(), s.appName, voucherIdx)
	execCancel()
	r.NoError(err, "wait for post-foreclosure PRT output execution in DB")
}

func (s *ForeclosePrtSuite) TestForeclosePrtReregisterReplay() {
	r := s.Require()

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	appAName := uniqueAppName("foreclose-prt-replay-a")
	s.appName = appAName

	appAddr, err := deployApplication(s.ctx, appAName, dappPath,
		"--salt", uniqueSalt(),
		"--prt",
		"--withdrawal-config", withdrawalConfigJSON,
	)
	r.NoError(err, "deploy PRT app A")
	r.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei),
		"fund application contract")

	inputIndex, _, err := sendInput(s.ctx, appAName, "foreclose PRT replay")
	r.NoError(err, "send input")
	r.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	inputA, err := waitForInputProcessed(processCtx, s.T(), appAName, inputIndex)
	processCancel()
	r.NoError(err, "wait for A input")
	r.Equal(model.InputCompletionStatus_Accepted, inputA.Status)

	for epochIndex := uint64(0); epochIndex <= inputA.EpochIndex; epochIndex++ {
		settleTournament(s.ctx, s.T(), r, s.ethClient, appAName, epochIndex)
	}

	claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), appAName, inputA.EpochIndex, model.EpochStatus_ClaimAccepted)
	claimCancel()
	r.NoError(err, "A input epoch should reach CLAIM_ACCEPTED")

	r.NoError(guardianForeclose(s.ctx, appAName, guardianIndex), "guardian foreclose A")
	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), appAName),
		"A did not record PRT foreclosure")
	forecloseCancel()

	r.NoError(disableApplication(s.ctx, appAName), "disable A before remove")
	r.NoError(removeApplication(s.ctx, appAName), "remove A")
	s.appName = ""

	s.appName = uniqueAppName("foreclose-prt-replay-b")
	r.NoError(registerPrtApplication(s.ctx, s.appName, appAddr, dappPath),
		"register PRT app B at %s", appAddr)

	processCtx, processCancel = context.WithTimeout(s.ctx, inputProcessingTimeout)
	inputB, err := waitForInputProcessed(processCtx, s.T(), s.appName, inputIndex)
	processCancel()
	r.NoError(err, "B should replay input")
	r.Equal(model.InputCompletionStatus_Accepted, inputB.Status)

	forecloseCtx, forecloseCancel = context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), s.appName),
		"B did not record already-existing PRT foreclosure")
	forecloseCancel()

	// A replayed, already-foreclosed app rebuilds its local epoch and commitment
	// state, then reconciles read-only against the chain: because A settled this
	// epoch on-chain before foreclosure, B observes the seal and reaches the same
	// CLAIM_ACCEPTED state a real-time node did — without sending any Settle
	// transaction or otherwise participating in DaveConsensus.
	claimCtx, claimCancel = context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), s.appName, inputA.EpochIndex, model.EpochStatus_ClaimAccepted)
	claimCancel()
	r.NoError(err, "replayed foreclosed app B should reconcile the pre-foreclosure epoch to CLAIM_ACCEPTED")

	status, err := readApplicationStatus(s.ctx, s.appName)
	r.NoError(err, "read B status")
	r.Equal("OK", firstStatusLine(status))
	r.Contains(status, "Enabled: true")
}

func registerPrtApplication(ctx context.Context, appName, appAddress, templatePath string) error {
	_, err := runCLI(ctx, "app", "register",
		"-n", appName,
		"-a", appAddress,
		"-t", templatePath,
		"--prt",
	)
	return err
}

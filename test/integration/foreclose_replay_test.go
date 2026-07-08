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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ForecloseReplaySuite verifies the "new node bootstraps against an already-
// foreclosed application" scenario: a fresh node entry for a foreclosed
// contract must
//
//  1. ingest pre-foreclosure inputs from chain,
//  2. process them through the advancer + validator to produce the same
//     local epoch/input state the original node had,
//  3. reconcile the pre-foreclosure on-chain-accepted claims to
//     CLAIM_ACCEPTED locally via the claimer's read-only getClaim path,
//  4. record the on-chain Foreclosure event as a foreclose marker on the
//     application row.
//
// The claimer must keep reconciling foreclosed apps via its read-only
// getClaim path; filtering them out of the claimer SELECTs entirely would
// leave the new node's local DB stuck at CLAIM_COMPUTED — diverging from
// chain reality and breaking downstream tooling that depends on the final
// accepted state.
type ForecloseReplaySuite struct {
	suite.Suite
	LogChecker
	ctx    context.Context
	cancel context.CancelFunc
}

func TestForecloseReplay(t *testing.T) {
	suite.Run(t, new(ForecloseReplaySuite))
}

func (s *ForecloseReplaySuite) SetupSuite() {
	// Two-app lifecycle (deploy + send + accept x 3 epochs + foreclose +
	// remove + register + replay + drain) needs more headroom than the
	// single-app foreclose test.
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 20*time.Minute)
}

func (s *ForecloseReplaySuite) TearDownSuite() {
	s.cancel()
}

func (s *ForecloseReplaySuite) SetupTest() {
	s.StartLogCapture()
}

func (s *ForecloseReplaySuite) TearDownTest() {
	// Unique-suffix names avoid collision across runs, so explicit teardown
	// is not required; leaving the apps registered also lets a debugger
	// inspect their final state.
	s.CheckLogs(s.T())
}

// TestForecloseReregisterReplay is the full lifecycle described on the
// suite type.
func (s *ForecloseReplaySuite) TestForecloseReregisterReplay() {
	r := s.Require()

	// The test mines large block batches (15 per input x 3 inputs, plus
	// the wait for each claim acceptance, plus the B-side replay) which
	// can race Anvil's historical state reads while node scanners catch up.
	s.SetExpectedLogs(s.T(),
		ExpectedLog{
			Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
			Level:   LevelError,
			Reason: "transient Anvil historical-state error during rapid mining; " +
				"the scanner retries on its next tick",
		},
	)

	builderEnv := os.Getenv("CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS")
	r.NotEmpty(builderEnv,
		"CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS must be set (run `eval $(make env)`)")

	const guardianIndex = 1
	guardianKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, guardianIndex)
	r.NoError(err, "derive guardian key")
	guardianAddr := crypto.PubkeyToAddress(guardianKey.PublicKey)

	withdrawalConfigJSON := strings.NewReplacer("\n", "", "\t", "").Replace(fmt.Sprintf(`{
		"guardian": "%s",
		"log2_leaves_per_account": 0,
		"log2_max_num_of_accounts": 20,
		"accounts_drive_start_index": 33554432,
		"withdrawal_output_builder": "%s"
	}`, guardianAddr.Hex(), builderEnv))

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")

	// ─── Phase 1 — deploy A, send 3 inputs across 3 epochs, wait for all
	//                claims accepted on chain, then foreclose ─────────────
	appAName := uniqueAppName("foreclose-replay-a")
	s.T().Logf("--- Phase 1: deploy %s with guardian=%s ---", appAName, guardianAddr.Hex())

	appAddr, err := deployApplication(s.ctx, appAName, dappPath,
		"--salt", uniqueSalt(),
		"--withdrawal-config", withdrawalConfigJSON,
	)
	r.NoError(err, "deploy A")
	s.T().Logf("    application deployed at %s", appAddr)

	r.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei),
		"fund application contract")

	// Send 3 inputs and bump anvil between each so they land in 3 distinct
	// epochs. Default epoch_length = 10 blocks, so mining 15 blocks
	// guarantees the next input falls into a fresh epoch.
	const numInputs = 3
	inputEpochs := make([]uint64, numInputs)
	for i := range numInputs {
		payload := fmt.Sprintf("foreclose-replay-input-%d", i)
		idx, _, err := sendInput(s.ctx, appAName, payload)
		r.NoError(err, "send input %d", i)
		r.Equal(uint64(i), idx, "input index must be %d", i)

		processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
		input, err := waitForInputProcessed(processCtx, s.T(), appAName, idx)
		processCancel()
		r.NoError(err, "wait for input %d", i)
		r.Equal(model.InputCompletionStatus_Accepted, input.Status)
		inputEpochs[i] = input.EpochIndex
		s.T().Logf("    input %d processed in epoch %d", i, input.EpochIndex)

		if i < numInputs-1 {
			r.NoError(anvilMine(s.ctx, 15), "mine to next epoch")
		}
	}

	distinctEpochs := dedupAscending(inputEpochs)
	r.Len(distinctEpochs, numInputs,
		"inputs must land in 3 distinct epochs; got %v", inputEpochs)

	// Wait for every epoch to reach CLAIM_ACCEPTED on chain.
	for _, ep := range distinctEpochs {
		claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
		epoch, err := waitForEpochStatus(claimCtx, s.T(), appAName, ep, model.EpochStatus_ClaimAccepted)
		claimCancel()
		r.NoError(err, "epoch %d should reach CLAIM_ACCEPTED", ep)
		r.NotNil(epoch.OutputsMerkleRoot, "epoch %d outputs merkle root", ep)
		s.T().Logf("    epoch %d accepted", ep)
	}

	snapA := captureAppSnapshot(s.ctx, s.T(), r, appAName, numInputs, distinctEpochs)
	s.T().Logf("    snapshot A: %d inputs, %d epochs", len(snapA.Inputs), len(snapA.Epochs))

	// Foreclose with the guardian wallet (mnemonic[1]).
	s.T().Logf("    foreclosing %s with guardian (mnemonic[%d])", appAName, guardianIndex)
	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", guardianIndex)},
		"foreclose", appAName, "--yes", "--json",
	)
	r.NoError(err, "guardian foreclose: %s", out)

	aCtx, aCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(aCtx, s.T(), appAName),
		"A did not record foreclose_block after guardian foreclose()")
	aCancel()
	s.T().Logf("=== Phase 1 complete: %s foreclose marker recorded ===", appAName)

	// ─── Phase 2 — remove A and re-register the same on-chain address
	//                under a new name B ─────────────────────────────────
	// `app remove` rejects apps with enabled=true, so disable A first. A
	// normal foreclosure records foreclose_block while the health status stays
	// OK and leaves enabled=true for L1 observation.
	s.T().Logf("--- Phase 2: remove %s and register the same address as B ---", appAName)
	r.NoError(disableApplication(s.ctx, appAName), "disable %s before remove", appAName)
	r.NoError(removeApplication(s.ctx, appAName), "remove %s", appAName)

	appBName := uniqueAppName("foreclose-replay-b")
	r.NoError(registerApplication(s.ctx, appBName, appAddr, dappPath),
		"register %s pointing at %s", appBName, appAddr)
	s.T().Logf("    %s registered at %s", appBName, appAddr)

	// ─── Phase 3 — wait for B to replay all inputs and to observe the
	//                foreclosure marker (the same on-chain Foreclosure
	//                event A saw, since both apps point at the same
	//                IApplication address). B stays enabled for L1 observation,
	//                keeps its health status OK, and records foreclose_block.
	s.T().Log("--- Phase 3: wait for B to replay + observe foreclosure marker ---")
	for i := range uint64(numInputs) {
		processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
		input, err := waitForInputProcessed(processCtx, s.T(), appBName, i)
		processCancel()
		r.NoError(err, "B: wait for input %d", i)
		r.Equal(snapA.Inputs[i].Status, input.Status,
			"input %d status must match A", i)
	}

	bCtx, bCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(bCtx, s.T(), appBName),
		"B did not record foreclose_block after replay")
	bCancel()

	// The foreclose marker is recorded on the first evmreader tick that sees
	// the Foreclosure event, which fires earlier than the claimer's per-epoch
	// reconciliation. Wait explicitly for the claimer's read-only getClaim
	// path to flip every replayed epoch CLAIM_COMPUTED → CLAIM_ACCEPTED
	// before snapshotting.
	for _, ep := range distinctEpochs {
		claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
		_, err := waitForEpochStatus(claimCtx, s.T(), appBName, ep, model.EpochStatus_ClaimAccepted)
		claimCancel()
		r.NoError(err, "B: epoch %d should reconcile to CLAIM_ACCEPTED", ep)
	}
	s.T().Logf("=== Phase 3 complete: %s foreclose marker recorded ===", appBName)

	// ─── Phase 4 — compare B's persisted state to A's snapshot ────────
	s.T().Log("--- Phase 4: compare B's persisted state to A's snapshot ---")
	snapB := captureAppSnapshot(s.ctx, s.T(), r, appBName, numInputs, distinctEpochs)

	r.Len(snapB.Inputs, len(snapA.Inputs),
		"B should have the same number of inputs as A")
	r.Len(snapB.Epochs, len(snapA.Epochs),
		"B should have the same number of accepted epochs as A")

	for i := range snapA.Inputs {
		compareReplayedInput(s.T(), r, &snapA.Inputs[i], &snapB.Inputs[i], i)
	}
	for ep, epochA := range snapA.Epochs {
		epochB, ok := snapB.Epochs[ep]
		r.True(ok, "B is missing epoch %d", ep)
		compareReplayedEpoch(s.T(), r, epochA, epochB, ep)
	}

	s.T().Log("=== Foreclosure-replay test complete: B's state matches A's snapshot ===")
}

func (s *ForecloseReplaySuite) TestForecloseReregisterReplayReaderMode() {
	if !isNodeSelfManaged() {
		s.T().Skip("skipping: reader-mode replay test requires test-managed node")
	}

	r := s.Require()
	s.SetExpectedLogs(s.T(),
		ExpectedLog{
			Pattern: regexp.MustCompile(`service=evm-reader.*context canceled`),
			Level:   LevelError,
			Reason:  "benign shutdown noise from switching the shared node into reader mode",
		},
		ExpectedLog{
			Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
			Level:   LevelError,
			Reason:  "transient EVM reader race against Anvil during restart catch-up",
		},
	)

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")

	appAName := uniqueAppName("foreclose-reader-a")
	appAddr, err := deployApplication(s.ctx, appAName, dappPath,
		"--salt", uniqueSalt(),
		"--withdrawal-config", withdrawalConfigJSON,
	)
	r.NoError(err, "deploy A")
	r.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei),
		"fund application contract")

	inputIndex, _, err := sendInput(s.ctx, appAName, "foreclose reader replay")
	r.NoError(err, "send input")
	r.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), appAName, inputIndex)
	processCancel()
	r.NoError(err, "wait for input")
	r.Equal(model.InputCompletionStatus_Accepted, input.Status)

	claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), appAName, input.EpochIndex, model.EpochStatus_ClaimAccepted)
	claimCancel()
	r.NoError(err, "wait for A accepted claim")

	r.NoError(guardianForeclose(s.ctx, appAName, guardianIndex), "guardian foreclose A")
	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), appAName),
		"A did not record foreclosure")
	forecloseCancel()

	r.NoError(disableApplication(s.ctx, appAName), "disable A before remove")
	r.NoError(removeApplication(s.ctx, appAName), "remove A")

	readerMode := false
	defer func() {
		if readerMode {
			stopSharedNode(s.T())
			startSharedNode(s.T())
		}
	}()

	stopSharedNode(s.T())
	startSharedNodeWithEnv(s.T(), "CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false")
	readerMode = true

	appBName := uniqueAppName("foreclose-reader-b")
	r.NoError(registerApplication(s.ctx, appBName, appAddr, dappPath),
		"register B at %s", appAddr)

	processCtx, processCancel = context.WithTimeout(s.ctx, inputProcessingTimeout)
	inputB, err := waitForInputProcessed(processCtx, s.T(), appBName, inputIndex)
	processCancel()
	r.NoError(err, "B should replay input in reader mode")
	r.Equal(model.InputCompletionStatus_Accepted, inputB.Status)

	forecloseCtx, forecloseCancel = context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), appBName),
		"B did not record already-existing foreclosure in reader mode")
	forecloseCancel()

	claimCtx, claimCancel = context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), appBName, input.EpochIndex, model.EpochStatus_ClaimAccepted)
	claimCancel()
	r.NoError(err, "B should reconcile accepted claim in reader mode")

	status, err := readApplicationStatus(s.ctx, appBName)
	r.NoError(err, "read B status")
	r.Equal("OK", firstStatusLine(status))
	r.Contains(status, "Enabled: true")

	stopSharedNode(s.T())
	startSharedNode(s.T())
	readerMode = false
}

func (s *ForecloseReplaySuite) TestOutputExecutionAfterForeclosureReplaysOnReregisteredApp() {
	r := s.Require()
	s.SetExpectedLogs(s.T(), ExpectedLog{
		Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
		Level:   LevelError,
		Reason: "transient Anvil historical-state error while claimer scans accepted claims; " +
			"the scanner retries on its next tick",
	})

	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")

	appAName := uniqueAppName("foreclose-output-replay-a")
	appAddr, err := deployApplication(s.ctx, appAName, dappPath,
		"--salt", uniqueSalt(),
		"--withdrawal-config", withdrawalConfigJSON,
	)
	r.NoError(err, "deploy A")
	r.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei),
		"fund application contract")

	inputIndex, _, err := sendInput(s.ctx, appAName, "foreclose output replay")
	r.NoError(err, "send input")
	r.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), appAName, inputIndex)
	processCancel()
	r.NoError(err, "wait for input")
	r.Equal(model.InputCompletionStatus_Accepted, input.Status)

	outputsResp, err := readOutputs(s.ctx, appAName)
	r.NoError(err, "read outputs")
	r.Len(outputsResp.Data, echoOutputsPerInput)

	var voucherIdx uint64
	voucherFound := false
	for _, output := range outputsResp.Data {
		if output.DecodedData != nil && output.DecodedData.Type == "Voucher" {
			voucherIdx = output.Index
			voucherFound = true
			break
		}
	}
	r.True(voucherFound, "voucher output not found")

	claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), appAName, input.EpochIndex, model.EpochStatus_ClaimAccepted)
	claimCancel()
	r.NoError(err, "wait for A accepted claim")

	r.NoError(guardianForeclose(s.ctx, appAName, guardianIndex), "guardian foreclose A")
	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), appAName),
		"A did not record foreclosure")
	forecloseCancel()

	txHash, err := executeOutput(s.ctx, appAName, voucherIdx)
	r.NoError(err, "execute accepted voucher after A foreclosure")
	r.NotEmpty(txHash)

	execCtx, execCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	err = waitForExecutionRecorded(execCtx, s.T(), appAName, voucherIdx)
	execCancel()
	r.NoError(err, "A should record post-foreclosure output execution")

	r.NoError(disableApplication(s.ctx, appAName), "disable A before remove")
	r.NoError(removeApplication(s.ctx, appAName), "remove A")

	appBName := uniqueAppName("foreclose-output-replay-b")
	r.NoError(registerApplication(s.ctx, appBName, appAddr, dappPath),
		"register B at %s", appAddr)

	processCtx, processCancel = context.WithTimeout(s.ctx, inputProcessingTimeout)
	inputB, err := waitForInputProcessed(processCtx, s.T(), appBName, inputIndex)
	processCancel()
	r.NoError(err, "B should replay input")
	r.Equal(model.InputCompletionStatus_Accepted, inputB.Status)

	forecloseCtx, forecloseCancel = context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), appBName),
		"B did not record already-existing foreclosure")
	forecloseCancel()

	claimCtx, claimCancel = context.WithTimeout(s.ctx, claimAcceptedTimeout)
	_, err = waitForEpochStatus(claimCtx, s.T(), appBName, input.EpochIndex, model.EpochStatus_ClaimAccepted)
	claimCancel()
	r.NoError(err, "B should reconcile accepted claim")

	execCtx, execCancel = context.WithTimeout(s.ctx, inputProcessingTimeout)
	err = waitForExecutionRecorded(execCtx, s.T(), appBName, voucherIdx)
	execCancel()
	r.NoError(err, "B should replay the post-foreclosure OutputExecuted event")
}

// appSnapshot is the subset of an application's persisted state we compare
// between the original node (A) and the re-registered node (B). Fields whose
// value comes from the broadcast tx (claim_transaction_hash, staged_at_block,
// timestamps) are deliberately excluded — A produces them through Stage-1/2
// broadcasts while B reaches CLAIM_ACCEPTED purely through the read-only
// reconciliation path, so equality there is not expected.
type appSnapshot struct {
	Inputs []model.Input
	Epochs map[uint64]*model.Epoch
}

func captureAppSnapshot(
	ctx context.Context,
	t testing.TB,
	r *require.Assertions,
	appName string,
	numInputs int,
	epochIndices []uint64,
) appSnapshot {
	t.Helper()
	snap := appSnapshot{
		Inputs: make([]model.Input, numInputs),
		Epochs: make(map[uint64]*model.Epoch, len(epochIndices)),
	}
	for i := range uint64(numInputs) {
		input, err := readInput(ctx, appName, i)
		r.NoError(err, "read %s input %d", appName, i)
		snap.Inputs[i] = *input
	}
	for _, ep := range epochIndices {
		epoch, err := readEpoch(ctx, appName, ep)
		r.NoError(err, "read %s epoch %d", appName, ep)
		snap.Epochs[ep] = epoch
	}
	return snap
}

func compareReplayedInput(t testing.TB, r *require.Assertions, a, b *model.Input, i int) {
	t.Helper()
	r.Equal(a.Index, b.Index, "input %d: index", i)
	r.Equal(a.EpochIndex, b.EpochIndex, "input %d: epoch index", i)
	r.Equal(a.Status, b.Status, "input %d: status", i)
	r.Equal(a.BlockNumber, b.BlockNumber, "input %d: block number", i)
	r.Equal(a.RawData, b.RawData, "input %d: raw data", i)
	r.Equal(a.TransactionHash, b.TransactionHash, "input %d: transaction hash", i)
	r.Equal(a.LogIndex, b.LogIndex, "input %d: log index", i)
}

func compareReplayedEpoch(t testing.TB, r *require.Assertions, a, b *model.Epoch, ep uint64) {
	t.Helper()
	r.Equal(a.Status, b.Status, "epoch %d: status", ep)
	r.Equal(a.Index, b.Index, "epoch %d: index", ep)
	r.Equal(a.FirstBlock, b.FirstBlock, "epoch %d: first block", ep)
	r.Equal(a.LastBlock, b.LastBlock, "epoch %d: last block", ep)
	r.Equal(a.InputIndexLowerBound, b.InputIndexLowerBound, "epoch %d: input lower bound", ep)
	r.Equal(a.InputIndexUpperBound, b.InputIndexUpperBound, "epoch %d: input upper bound", ep)
	r.Equal(a.OutputsMerkleRoot, b.OutputsMerkleRoot, "epoch %d: outputs merkle root", ep)
	r.Equal(a.MachineHash, b.MachineHash, "epoch %d: machine hash", ep)
}

func dedupAscending(in []uint64) []uint64 {
	seen := map[uint64]bool{}
	out := make([]uint64, 0, len(in))
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// removeApplication removes a registered application from the local DB via
// `cartesi-rollups-cli app remove`. The CLI rejects apps with enabled=true, so
// callers must set enabled=false first.
func removeApplication(ctx context.Context, appName string) error {
	_, err := runCLI(ctx, "app", "remove", appName, "--yes")
	return err
}

// registerApplication registers an existing on-chain Application contract in
// the local DB under a new name, without redeploying. Reads consensus address,
// epoch length, withdrawal config, etc. from the on-chain contract — only
// the name, address, and template path are required.
func registerApplication(ctx context.Context, appName, appAddress, templatePath string) error {
	_, err := runCLI(ctx, "app", "register",
		"-n", appName,
		"-a", appAddress,
		"-t", templatePath,
	)
	return err
}

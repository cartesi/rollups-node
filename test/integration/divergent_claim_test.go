// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthority"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

// DivergentClaimSuite models the compromised-owner-key attack on an Authority
// application: the operator's private key has been leaked, and the attacker
// uses it to push a crafted divergent claim to chain before the operator's
// node can submit the legitimate one. The node's claimer must observe the
// divergence and drive the application to DIVERGED; the same outcome must
// hold when a fresh node bootstraps against the already-divergent chain.
//
//	Phase 1 — attack:
//	  1. Deploy Authority (node = owner).
//	  2. Send inputs 0 and 1 in distinct epochs; wait for legitimate ACCEPT.
//	  3. Stop the node so the attacker can race the pipeline deterministically.
//	  4. Send input 2 and mine past the 3rd epoch's last block.
//	  5. Attacker submits a divergent claim for epoch 2 (random outputsMerkleRoot,
//	     reusing epoch 1's proof for valid-length argument bytes). The chain
//	     emits ClaimSubmitted + ClaimStaged with the divergent machine root.
//	     acceptClaim is intentionally NOT called — this models the realistic
//	     attacker who pushes a single divergent claim and disappears.
//	  6. Restart the node. It detects input 2, computes the legitimate claim
//	     locally, scans the chain via findClaimSubmittedEventAndSucc (the
//	     accepted-scan returns nil because no ClaimAccepted exists), and
//	     marks the application DIVERGED with reason
//	     `authority_divergence_at_submission`.
//
//	Phase 2 — replay against a now-divergent chain, in reader mode:
//	  7. Remove app A.
//	  8. Restart the node with CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false
//	     so the claimer cannot submit anything; only the read-only scan
//	     pipeline runs.
//	  9. Re-register the same on-chain address as app B.
//	  10. The reader-mode node replays inputs 0-2, finds epochs 0/1
//	      legitimately accepted (reconciles), reaches CLAIM_COMPUTED for
//	      epoch 2, scans the chain, finds the divergent claim, and marks B
//	      DIVERGED. The point of this phase is to confirm that the
//	      divergence-detection path is independent of the submission path —
//	      a node with no key (or a paranoid operator who has disabled
//	      submission) still drives the right terminal state.
type DivergentClaimSuite struct {
	suite.Suite
	LogChecker
	ctx    context.Context
	cancel context.CancelFunc
}

func TestDivergentClaim(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("skipping: divergent-claim test requires test-managed node " +
			"(it stops/starts the shared node mid-test)")
	}
	suite.Run(t, new(DivergentClaimSuite))
}

func (s *DivergentClaimSuite) SetupSuite() {
	// Two-app lifecycle (deploy + 3 epochs + attack + replay) is comparable
	// in length to the foreclose-replay suite.
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 20*time.Minute)
}

func (s *DivergentClaimSuite) TearDownSuite() {
	// Phase 2 brings the node up in reader mode. Subsequent suites expect
	// the default (claim-submission-enabled) configuration, so always
	// recycle the node here regardless of state.
	if sharedNode != nil {
		s.T().Log("Stopping reader-mode node before restoring default for subsequent suites...")
		stopSharedNode(s.T())
	}
	s.T().Log("Restarting shared node in default mode for subsequent suites...")
	startSharedNode(s.T())
	s.cancel()
}

func (s *DivergentClaimSuite) SetupTest() {
	s.StartLogCapture()
}

func (s *DivergentClaimSuite) TearDownTest() {
	// Both apps end the test in DIVERGED (terminal); the disable helper
	// rejects that state. Leave them; unique names mean no collision next run.
	s.CheckLogs(s.T())
}

// TestDivergentClaimReplay is the full lifecycle described on the suite type.
func (s *DivergentClaimSuite) TestDivergentClaimReplay() {
	r := s.Require()

	// Both apps end the test in DIVERGED with one of the two Authority
	// divergence reasons — Authority's submit-stage-accept lifecycle means
	// whichever scan (ClaimSubmitted or ClaimAccepted) lands first wins,
	// and both are terminal. The claimer's tick wraps the transition error
	// and re-logs it, so we allow-list that too. Stopping the node mid-
	// tick (Phase 1.5 and Phase 2 transitions) cancels in-flight RPC
	// queries, producing a handful of evmreader ERR lines that are benign
	// shutdown noise. The rapid mining can race the EVM reader's block
	// fetcher; tolerate transient BlockOutOfRangeError.
	s.SetExpectedLogs(s.T(),
		ExpectedLog{
			Pattern: regexp.MustCompile(
				`marking application as diverged.*authority_divergence_at_(submission|acceptance)`),
			Level: LevelError,
			Reason: "expected DIVERGED transition for both the attacked original app and " +
				"the re-registered replay app (compromised-owner-key attack scenario)",
		},
		ExpectedLog{
			Pattern: regexp.MustCompile(
				`Tick service=claimer.*authority_divergence_at_(submission|acceptance)`),
			Level:  LevelError,
			Reason: "claimer Tick wraps and re-logs the divergence-induced DIVERGED error",
		},
		ExpectedLog{
			Pattern: regexp.MustCompile(`service=evm-reader.*context canceled`),
			Level:   LevelError,
			Reason: "benign shutdown noise from stopping the node mid-tick; " +
				"retryablehttp wraps the cancellation as `Post \"<url>\": context canceled`",
		},
		ExpectedLog{
			Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
			Level:   LevelError,
			Reason:  "transient EVM reader race against Anvil during rapid block mining",
		},
	)

	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.Dial(endpoint)
	r.NoError(err, "dial ethclient")
	defer client.Close()

	chainID, err := client.ChainID(s.ctx)
	r.NoError(err, "fetch chain id")

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	appAName := uniqueAppName("divergent-a")
	const guardianIndex = 1
	withdrawalConfigJSON, _ := withdrawalConfigForGuardian(s.T(), guardianIndex)

	// ─── Phase 1: deploy and run epochs 0–1 to legitimate ACCEPT ────────
	s.T().Logf("--- Phase 1: deploy %s and accept two legitimate claims ---", appAName)

	appAddrStr, consensusAddrStr, err := deployApplicationWithConsensus(s.ctx,
		appAName, dappPath, "--salt", uniqueSalt(), "--withdrawal-config", withdrawalConfigJSON)
	r.NoError(err, "deploy A")
	appAddr := common.HexToAddress(appAddrStr)
	consensusAddr := common.HexToAddress(consensusAddrStr)
	s.T().Logf("    app=%s consensus=%s", appAddr.Hex(), consensusAddr.Hex())

	r.NoError(anvilSetBalance(s.ctx, appAddrStr, oneEtherWei),
		"fund application contract")

	// Inputs 0 and 1 go through the normal flow so we can observe both the
	// legitimate ClaimAccepted on chain AND grab a valid-length
	// outputsMerkleProof from epoch 1 to reuse for the attack.
	inputEpochs := make([]uint64, 0, 3) //nolint:mnd
	for i := 0; i < 2; i++ {            //nolint:mnd
		payload := fmt.Sprintf("divergent-input-%d", i)
		idx, _, err := sendInput(s.ctx, appAName, payload)
		r.NoError(err, "send input %d", i)
		r.Equal(uint64(i), idx) //nolint:gosec

		procCtx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
		input, err := waitForInputProcessed(procCtx, s.T(), appAName, idx)
		cancel()
		r.NoError(err, "wait for input %d", i)
		r.Equal(model.InputCompletionStatus_Accepted, input.Status)
		inputEpochs = append(inputEpochs, input.EpochIndex)

		claimCtx, claimCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
		_, err = waitForEpochStatus(claimCtx, s.T(), appAName, input.EpochIndex,
			model.EpochStatus_ClaimAccepted)
		claimCancel()
		r.NoError(err, "epoch %d → CLAIM_ACCEPTED", input.EpochIndex)
		s.T().Logf("    input %d processed; epoch %d ACCEPTED", i, input.EpochIndex)

		// Mine to the next epoch boundary so input i+1 lands in a distinct epoch.
		r.NoError(anvilMine(s.ctx, 15), "mine to next epoch") //nolint:mnd
	}

	// Read epoch 1 to harvest a valid-length outputsMerkleProof — the
	// IAuthority contract validates only the proof's length, not its
	// semantic correctness, so we can splice it into the divergent payload.
	epoch1, err := readEpoch(s.ctx, appAName, inputEpochs[1])
	r.NoError(err, "read epoch 1")
	r.NotEmpty(epoch1.TxBufferProof,
		"epoch 1 must have an outputs merkle proof to reuse for the attack")
	epochLen := epoch1.LastBlock - epoch1.FirstBlock + 1
	s.T().Logf("    epoch length = %d blocks; epoch 1 proof = %d siblings",
		epochLen, len(epoch1.TxBufferProof))

	// ─── Phase 1.5: stop the node so the attacker cannot lose the race ──
	s.T().Log("--- Phase 1.5: stop node, then send input 2 and submit divergent claim ---")
	stopSharedNode(s.T())

	// Send input 2 — it lands at whatever block anvil mines for the tx.
	idx2, block2, err := sendInput(s.ctx, appAName, "divergent-input-2")
	r.NoError(err, "send input 2")
	r.Equal(uint64(2), idx2) //nolint:mnd,gosec
	s.T().Logf("    input 2 sent at block %d", block2)

	// Compute the epoch input 2 landed in from its block number relative
	// to epoch 1. Guard against the (unexpected) case where mining timing
	// drifts and input 2 falls inside epoch 1 — that would underflow the
	// uint64 subtraction and produce a nonsense target epoch.
	r.Greater(block2, epoch1.LastBlock,
		"input 2 must land past epoch %d's last block (%d); got block %d",
		inputEpochs[1], epoch1.LastBlock, block2)
	targetEpochIndex := inputEpochs[1] + ((block2 - epoch1.LastBlock - 1) / epochLen) + 1
	targetEpochFirstBlock := epoch1.FirstBlock + (targetEpochIndex-inputEpochs[1])*epochLen
	targetEpochLastBlock := targetEpochFirstBlock + epochLen - 1
	r.GreaterOrEqual(block2, targetEpochFirstBlock,
		"input 2 block %d must be inside epoch %d's window [%d, %d]",
		block2, targetEpochIndex, targetEpochFirstBlock, targetEpochLastBlock)
	r.LessOrEqual(block2, targetEpochLastBlock,
		"input 2 block %d must be inside epoch %d's window [%d, %d]",
		block2, targetEpochIndex, targetEpochFirstBlock, targetEpochLastBlock)
	s.T().Logf("    input 2 lands in epoch %d [blocks %d-%d]",
		targetEpochIndex, targetEpochFirstBlock, targetEpochLastBlock)

	currentBlock, err := client.BlockNumber(s.ctx)
	r.NoError(err, "read current block")
	if currentBlock <= targetEpochLastBlock {
		blocksToClose := int(targetEpochLastBlock - currentBlock + 1) //nolint:gosec
		r.NoError(anvilMine(s.ctx, blocksToClose), "mine to close target epoch")
		s.T().Logf("    mined %d blocks to close epoch %d at block %d",
			blocksToClose, targetEpochIndex, targetEpochLastBlock)
	}

	// ── Attacker submits the divergent claim ─────────────────────────────
	// Using mnemonic[0] — the same key the operator/node uses. This models
	// the compromised-key threat: the attacker holds the same private key,
	// so the chain accepts the call as the legitimate Authority owner.
	attackerKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, 0)
	r.NoError(err, "derive attacker key (same as node owner)")
	attackerOpts, err := bind.NewKeyedTransactorWithChainID(attackerKey, chainID)
	r.NoError(err, "new keyed transactor")
	attackerOpts.Context = s.ctx

	authorityBinding, err := iauthority.NewIAuthority(consensusAddr, client)
	r.NoError(err, "bind iauthority")

	divergentOutputs := randomBytes32(s.T())
	proof := merkleProofToBytes32(epoch1.TxBufferProof)
	s.T().Logf("    attacker submitting divergent claim: lpbn=%d outputs=0x%x proof_siblings=%d",
		targetEpochLastBlock, divergentOutputs, len(proof))
	submitTx, err := authorityBinding.SubmitClaim(attackerOpts, appAddr,
		new(big.Int).SetUint64(targetEpochLastBlock), divergentOutputs, proof)
	r.NoError(err, "attacker SubmitClaim")
	submitReceipt, err := bind.WaitMined(s.ctx, client, submitTx)
	r.NoError(err, "wait for divergent submitClaim tx to mine")
	r.Equal(uint64(1), submitReceipt.Status, "divergent submitClaim tx must succeed on chain")
	s.T().Logf("    divergent submitClaim mined in block %d tx=%s",
		submitReceipt.BlockNumber.Uint64(), submitTx.Hash().Hex())

	// Deliberately do NOT call acceptClaim. Modeling a realistic attacker
	// pushing a single divergent claim to chain — and exercising the node's
	// ClaimSubmitted-scan divergence path, which lives behind the service-
	// level findClaimSubmittedEventAndSucc wrapper that asserts
	// checkEpochSequenceConstraint on the previous epoch. Phase 2's
	// reader-mode replay used to trip that invariant because the catch-up
	// reconciliation of the prior legitimate epochs left
	// claim_transaction_hash NULL; the production fix to
	// UpdateEpochWithAcceptedClaim (optional txHash arg) and the relaxed
	// checkEpochConstraint now let the divergence detection proceed.

	// ─── Phase 1 conclusion: restart node, expect DIVERGED ────────────
	s.T().Log("--- Phase 1: restart node and wait for divergence-driven DIVERGED ---")
	startSharedNode(s.T())

	stateCtx, stateCancel := context.WithTimeout(s.ctx, 5*time.Minute) //nolint:mnd
	r.NoError(waitForApplicationStatus(stateCtx, s.T(), appAName, "DIVERGED"),
		"A should reach DIVERGED after observing the divergent on-chain claim")
	stateCancel()
	statusA, err := readApplicationStatus(s.ctx, appAName)
	r.NoError(err)
	r.Regexp(`authority_divergence_at_(submission|acceptance)`, statusA,
		"A's DIVERGED reason must reference one of the two Authority divergence buckets")
	s.T().Logf("=== Phase 1 complete: %s is DIVERGED ===\n%s", appAName, statusA)

	s.T().Log("--- Phase 1.6: guardian forecloses the already-DIVERGED app ---")
	r.NoError(guardianForeclose(s.ctx, appAName, guardianIndex),
		"guardian foreclosure should still be indexed after divergence made the app DIVERGED")
	forecloseCtx, forecloseCancel := context.WithTimeout(s.ctx, 30*time.Second)
	r.NoError(waitForApplicationForeclosed(forecloseCtx, s.T(), appAName),
		"A should record foreclosure even though status is DIVERGED")
	forecloseCancel()
	statusA, err = readApplicationStatus(s.ctx, appAName)
	r.NoError(err, "read A status after foreclosure")
	r.Equal("DIVERGED", firstStatusLine(statusA),
		"foreclosure after divergence should not erase the DIVERGED reason")
	r.Contains(statusA, "Foreclose block:",
		"DIVERGED app should still surface the recorded foreclose block")

	// ─── Phase 2: reader mode replay ─────────────────────────────────────
	s.T().Log("--- Phase 2: remove A, restart node in reader mode, re-register as B ---")
	r.NoError(disableApplication(s.ctx, appAName), "disable %s before remove", appAName)
	r.NoError(removeApplication(s.ctx, appAName), "remove %s", appAName)

	stopSharedNode(s.T())
	// CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false brings the claimer up
	// in read-only mode: it computes claims locally and runs the scan path
	// but never broadcasts a submitClaim tx. The divergence-detection path
	// must still fire — that is the assertion of this phase.
	startSharedNodeWithEnv(s.T(), "CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false")

	appBName := uniqueAppName("divergent-b")
	r.NoError(registerApplication(s.ctx, appBName, appAddrStr, dappPath),
		"register %s at %s", appBName, appAddrStr)
	s.T().Logf("    %s registered at %s", appBName, appAddrStr)

	// B has to replay all 3 inputs locally before it reaches the epoch
	// where the divergent claim sits. Wait for the same DIVERGED outcome.
	for i := uint64(0); i < 3; i++ { //nolint:mnd
		procCtx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
		input, err := waitForInputProcessed(procCtx, s.T(), appBName, i)
		cancel()
		r.NoError(err, "B: wait for input %d", i)
		r.Equal(model.InputCompletionStatus_Accepted, input.Status)
	}
	stateCtx, stateCancel = context.WithTimeout(s.ctx, 5*time.Minute) //nolint:mnd
	r.NoError(waitForApplicationStatus(stateCtx, s.T(), appBName, "DIVERGED"),
		"B should reach DIVERGED via the read-only scan path")
	stateCancel()

	statusB, err := readApplicationStatus(s.ctx, appBName)
	r.NoError(err)
	r.Regexp(`authority_divergence_at_(submission|acceptance)`, statusB,
		"B's DIVERGED reason must reference one of the Authority divergence buckets "+
			"(the read-only scan path proves the divergence even with submission disabled)")
	s.T().Logf("=== Phase 2 complete: %s is DIVERGED in reader mode ===\n%s", appBName, statusB)
}

// deployApplicationWithConsensus wraps deployApplication so the test also
// gets the on-chain Authority/IConsensus address — needed to bind the
// IAuthority contract for the attacker's direct submitClaim call.
func deployApplicationWithConsensus(
	ctx context.Context,
	appName, dappPath string,
	extraArgs ...string,
) (appAddr string, consensusAddr string, err error) {
	args := []string{"deploy", "application", appName, dappPath, "--json"}
	args = append(args, extraArgs...)
	out, err := runCLI(ctx, args...)
	if err != nil {
		return "", "", fmt.Errorf("deploy: %w", err)
	}
	var parsed struct {
		IApplicationAddress string `json:"iapplication_address"`
		IConsensusAddress   string `json:"iconsensus_address"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return "", "", fmt.Errorf("parse deploy output: %w", err)
	}
	if parsed.IApplicationAddress == "" || parsed.IConsensusAddress == "" {
		return "", "", fmt.Errorf("deploy output missing addresses: %s", out)
	}
	return parsed.IApplicationAddress, parsed.IConsensusAddress, nil
}

// randomBytes32 returns 32 random bytes for use as a fake outputsMerkleRoot.
// The hash is deliberately arbitrary — the IAuthority contract performs no
// semantic check on it, so any 32-byte value is accepted, and the resulting
// machineMerkleRoot derived from it will not match the node's legitimate
// computation.
func randomBytes32(t testing.TB) [32]byte {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return b
}

// merkleProofToBytes32 reshapes []common.Hash from the JSON-RPC API into the
// [][32]byte the abigen IAuthority.SubmitClaim binding expects.
func merkleProofToBytes32(in []common.Hash) [][32]byte {
	out := make([][32]byte, len(in))
	for i, h := range in {
		out[i] = h
	}
	return out
}

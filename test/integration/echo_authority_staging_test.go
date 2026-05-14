// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
)

// EchoAuthorityStagingSuite exercises the non-fast-path claim flow by
// deploying an Authority application with claimStagingPeriod >= 2. With a
// non-zero staging period the chain forces COMPUTED → SUBMITTED → STAGED →
// ACCEPTED (with a wait for the period to elapse). The default tests use
// claimStagingPeriod = 0, where submit and stage happen atomically and the
// staged-then-accepted gap is not observable.
type EchoAuthorityStagingSuite struct {
	suite.Suite
	LogChecker
	ctx     context.Context
	cancel  context.CancelFunc
	appName string
}

func TestEchoAuthorityStaging(t *testing.T) {
	suite.Run(t, new(EchoAuthorityStagingSuite))
}

func (s *EchoAuthorityStagingSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 10*time.Minute)
}

func (s *EchoAuthorityStagingSuite) TearDownSuite() {
	s.cancel()
}

func (s *EchoAuthorityStagingSuite) SetupTest() {
	s.StartLogCapture()
	s.appName = ""
}

func (s *EchoAuthorityStagingSuite) TearDownTest() {
	if s.appName != "" {
		s.T().Logf("Disabling application %s", s.appName)
		if err := disableApplication(s.ctx, s.appName); err != nil {
			s.T().Errorf("failed to disable application %s: %v", s.appName, err)
		}
	}
	s.CheckLogs(s.T())
}

// TestEchoAuthorityStagingPath deploys with --claim-staging-period 5 and
// runs the full lifecycle. The 5-block period is large enough to make the
// STAGED state visible in node logs (the claim sits in STAGED until anvil
// advances 5 blocks past the staging tx) but small enough to keep the test
// short. The chain-side acceptClaim() will revert with
// ClaimStagingPeriodNotOverYet until the period elapses, which the claimer
// treats as transient until the next tick — the existing retry loop drives
// the transition once the period clears.
func (s *EchoAuthorityStagingSuite) TestEchoAuthorityStagingPath() {
	r := s.Require()
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("echo-authority-staging")

	runEchoLifecycleTest(s.ctx, s.T(), r, echoLifecycleConfig{
		AppName:         s.appName,
		DappPath:        dappPath,
		Payload:         "hello cartesi (staging)",
		ExtraDeployArgs: []string{"--claim-staging-period", "5"},
	})

	// Pin the staging-path invariant: the epoch must have gone through
	// CLAIM_STAGED with a recorded staged_at_block. A regression that
	// skipped CLAIM_STAGED entirely (e.g., a re-introduced fast-path that
	// ignored claim_staging_period > 0) would leave staged_at_block NULL
	// even after the epoch reached CLAIM_ACCEPTED — the schema preserves
	// the column through the accept transition (`epoch_staged_requires_block`
	// CHECK fires only on CLAIM_STAGED rows).
	input, err := readInput(s.ctx, s.appName, 0)
	r.NoError(err, "read input 0 to find its epoch")
	epoch, err := readEpoch(s.ctx, s.appName, input.EpochIndex)
	r.NoError(err, "read epoch %d", input.EpochIndex)
	r.Equal(model.EpochStatus_ClaimAccepted, epoch.Status,
		"epoch must reach CLAIM_ACCEPTED before this assertion is meaningful")
	r.NotNil(epoch.StagedAtBlock,
		"epoch must have gone through CLAIM_STAGED — staged_at_block is preserved through ACCEPTED")

	s.T().Log("=== Authority staging-path lifecycle complete (STAGED observation pinned) ===")
}

func (s *EchoAuthorityStagingSuite) TestEchoAuthorityForecloseStagedClaim() {
	r := s.Require()

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
	s.appName = uniqueAppName("echo-authority-staged-foreclose")

	const claimStagingPeriod = "100"
	appAddr, err := deployApplication(
		s.ctx,
		s.appName,
		dappPath,
		"--salt", uniqueSalt(),
		"--claim-staging-period", claimStagingPeriod,
		"--withdrawal-config", withdrawalConfigJSON,
	)
	r.NoError(err, "deploy foreclosable authority app")
	r.NoError(anvilSetBalance(s.ctx, appAddr, oneEtherWei), "fund application contract")

	inputIndex, _, err := sendInput(s.ctx, s.appName, "hello cartesi (foreclosed staged claim)")
	r.NoError(err, "send input")
	r.Equal(uint64(0), inputIndex)

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	input, err := waitForInputProcessed(processCtx, s.T(), s.appName, inputIndex)
	processCancel()
	r.NoError(err, "wait for input processing")
	r.Equal(model.InputCompletionStatus_Accepted, input.Status)

	r.NoError(anvilMine(s.ctx, 15), "mine past the default authority epoch boundary")

	stagedCtx, stagedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err := waitForEpochStatus(stagedCtx, s.T(), s.appName, input.EpochIndex, model.EpochStatus_ClaimStaged)
	stagedCancel()
	r.NoError(err, "wait for authority claim to become CLAIM_STAGED")
	r.NotNil(epoch.StagedAtBlock, "staged claim must record staged_at_block before foreclosure")

	out, err := runCLIWithEnv(s.ctx,
		[]string{fmt.Sprintf("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=%d", guardianIndex)},
		"foreclose", s.appName, "--yes", "--json",
	)
	r.NoError(err, "guardian foreclose CLI call: %s", out)
	s.T().Logf("    foreclose tx: %s", strings.TrimSpace(out))

	stateCtx, stateCancel := context.WithTimeout(s.ctx, 30*time.Second)
	err = waitForApplicationForeclosed(stateCtx, s.T(), s.appName)
	stateCancel()
	r.NoError(err, "app did not record foreclose_block after guardian foreclose()")

	foreclosedCtx, foreclosedCancel := context.WithTimeout(s.ctx, claimAcceptedTimeout)
	epoch, err = waitForEpochStatus(foreclosedCtx, s.T(), s.appName, input.EpochIndex, model.EpochStatus_ClaimForeclosed)
	foreclosedCancel()
	r.NoError(err, "foreclosed staged claim should become CLAIM_FORECLOSED without waiting for staging-period expiry")
	r.NotNil(epoch.StagedAtBlock, "staged_at_block should be preserved after CLAIM_FORECLOSED")
	r.NotNil(epoch.OutputsMerkleRoot, "local claim data should be preserved when terminalizing as CLAIM_FORECLOSED")
}

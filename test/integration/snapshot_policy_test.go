// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

// IMPORTANT: These integration tests share a single Anvil blockchain instance.
// PRT tests call anvilMine() which globally advances the block number, affecting
// Authority epoch boundaries. Tests MUST NOT run in parallel (no t.Parallel()).
// The go test runner executes tests within a package sequentially by default,
// which is required for correctness here.

package integration

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SnapshotPolicySuite struct {
	suite.Suite
	LogChecker
	ctx     context.Context
	cancel  context.CancelFunc
	appName string
}

func TestSnapshotPolicy(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("skipping: node is externally managed (compose); " +
			"snapshot policy tests require test-managed node for restart")
	}
	suite.Run(t, new(SnapshotPolicySuite))
}

func (s *SnapshotPolicySuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(
		context.Background(), 30*time.Minute)
}

func (s *SnapshotPolicySuite) TearDownSuite() {
	// If a test failed mid-restart, the node may be stopped. Restart it
	// so subsequent test suites have a running node.
	if sharedNode == nil {
		s.T().Log("Restarting shared node for subsequent tests...")
		startSharedNode(s.T())
	}
	s.cancel()
}

func (s *SnapshotPolicySuite) SetupTest() {
	s.StartLogCapture()
	s.appName = ""
}

func (s *SnapshotPolicySuite) TearDownTest() {
	if s.appName != "" {
		s.T().Logf("Disabling application %s", s.appName)
		if err := disableApplication(s.ctx, s.appName); err != nil {
			s.T().Logf(
				"warning: failed to disable %s: %v", s.appName, err)
		}
	}
	s.CheckLogs(s.T())
}

// deployWithSnapshotPolicy deploys an echo-dapp in disabled state, sets the
// snapshot policy via CLI, then enables the application. This approach avoids
// creating a JSON execution-parameters file with all the default values.
func (s *SnapshotPolicySuite) deployWithSnapshotPolicy(
	appName, dappPath string,
	policy model.SnapshotPolicy,
	extraDeployArgs ...string,
) string {
	require := s.Require()
	t := s.T()

	deployArgs := append(
		[]string{"--salt", uniqueSalt(), "--enable=false"},
		extraDeployArgs...)

	t.Logf("    deploying %s in disabled state...", appName)
	addr, err := deployApplication(
		s.ctx, appName, dappPath, deployArgs...)
	require.NoError(err, "deploy echo-dapp (disabled)")
	t.Logf("    deployed at %s (disabled)", addr)

	t.Logf("    setting snapshot_policy=%s via CLI...", policy)
	_, err = runCLI(s.ctx,
		"app", "execution-parameters", "set",
		appName, "snapshot_policy", string(policy))
	require.NoError(err, "set snapshot policy")

	t.Logf("    enabling application %s...", appName)
	_, err = runCLI(s.ctx,
		"app", "status", appName, "enabled", "--yes")
	require.NoError(err, "enable application")

	return addr
}

// snapshotPolicyConfig configures a snapshot policy test.
type snapshotPolicyConfig struct {
	// Policy is the snapshot policy to test.
	Policy model.SnapshotPolicy
	// ExtraDeployArgs are additional CLI flags for deploy (e.g., "--prt").
	ExtraDeployArgs []string
	// PreClaimHook, if non-nil, is called before claim/execution verification.
	PreClaimHook func(
		ctx context.Context, t testing.TB,
		require *require.Assertions, appName string,
	)
}

// runSnapshotPolicyTest runs the shared test flow for a given snapshot policy:
// deploy with policy, send input, restart node, send another, verify pipeline.
func (s *SnapshotPolicySuite) runSnapshotPolicyTest(cfg snapshotPolicyConfig) {
	require := s.Require()
	dappPath := envOrDefault(
		"CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	policyStr := strings.ToLower(
		strings.ReplaceAll(string(cfg.Policy), "_", "-"))
	s.appName = uniqueAppName("snap-" + policyStr)
	defer timed(s.T(), "snapshot "+policyStr+" test")()

	// === Deploy with snapshot policy ===

	s.T().Logf(
		"--- Setup: deploying echo-dapp with %s snapshot policy ---",
		policyStr)
	addr := s.deployWithSnapshotPolicy(
		s.appName, dappPath, cfg.Policy, cfg.ExtraDeployArgs...)

	err := anvilSetBalance(s.ctx, addr, oneEtherWei)
	require.NoError(err, "fund application contract")
	s.T().Log("    funded application contract with 1 ETH")

	// === Send first input ===

	s.T().Logf("Sending first input (snap-%s-1)...", policyStr)
	idx1, _, err := sendInput(
		s.ctx, s.appName, "snap-"+policyStr+"-1")
	require.NoError(err, "send first input")
	s.T().Logf("    input sent (index=%d)", idx1)

	func() {
		defer timed(s.T(), "wait for first input")()
		processCtx, processCancel := context.WithTimeout(
			s.ctx, inputProcessingTimeout)
		defer processCancel()

		input, err := waitForInputProcessed(
			processCtx, s.T(), s.appName, idx1)
		require.NoError(err, "wait for first input processing")
		require.Equal(
			model.InputCompletionStatus_Accepted, input.Status)
		s.T().Log("    first input ACCEPTED")
	}()

	outputs, err := readOutputs(s.ctx, s.appName)
	require.NoError(err, "read outputs after first input")
	require.Equal(
		uint64(echoOutputsPerInput), outputs.Pagination.TotalCount,
		"should have %d outputs after 1 input", echoOutputsPerInput)
	s.T().Logf("    verified %d outputs after first input",
		echoOutputsPerInput)

	// === Restart node to test snapshot loading ===

	s.T().Logf(
		"Restarting node to test snapshot loading with %s policy...",
		policyStr)
	func() {
		defer timed(s.T(), "node restart cycle ("+policyStr+")")()
		stopSharedNode(s.T())
		startSharedNode(s.T())
	}()

	// === Send second input after restart ===

	s.T().Logf(
		"Sending second input after restart (snap-%s-2)...", policyStr)
	idx2, _, err := sendInput(
		s.ctx, s.appName, "snap-"+policyStr+"-2")
	require.NoError(err, "send second input after restart")
	s.T().Logf("    input sent (index=%d)", idx2)

	func() {
		defer timed(s.T(), "wait for second input after restart")()
		processCtx, processCancel := context.WithTimeout(
			s.ctx, inputProcessingTimeout)
		defer processCancel()

		input, err := waitForInputProcessed(
			processCtx, s.T(), s.appName, idx2)
		require.NoError(err, "wait for second input after restart")
		require.Equal(
			model.InputCompletionStatus_Accepted, input.Status)
		s.T().Log("    second input ACCEPTED after restart")
	}()

	outputsAfter, err := readOutputs(s.ctx, s.appName)
	require.NoError(err, "read outputs after restart")
	require.Equal(
		uint64(2*echoOutputsPerInput),
		outputsAfter.Pagination.TotalCount,
		"should have %d outputs after 2 inputs",
		2*echoOutputsPerInput)
	s.T().Logf("    verified %d outputs after restart",
		2*echoOutputsPerInput)

	// === Optional pre-claim hook (e.g. PRT tournament settlement) ===

	if cfg.PreClaimHook != nil {
		cfg.PreClaimHook(s.ctx, s.T(), require, s.appName)
	}

	// === Verify full L1 pipeline ===

	s.T().Log("Verifying claim and execution...")
	epochIndex := outputs.Data[0].EpochIndex

	var voucherIdx, noticeIdx uint64
	voucherFound, noticeFound := false, false
	for _, out := range outputs.Data {
		if out.DecodedData == nil {
			continue
		}
		switch out.DecodedData.Type {
		case "Voucher":
			voucherIdx = out.Index
			voucherFound = true
		case "Notice":
			noticeIdx = out.Index
			noticeFound = true
		}
	}
	require.True(voucherFound, "voucher output not found")
	require.True(noticeFound, "notice output not found")

	verifyClaimAndExecute(s.ctx, s.T(), require, verifyAndExecuteConfig{
		AppName:      s.appName,
		EpochIndex:   epochIndex,
		EpochOutputs: outputs.Data,
		VoucherIdx:   voucherIdx,
		NoticeIdx:    noticeIdx,
	})

	s.T().Logf("=== Snapshot %s test complete ===", policyStr)
}

// TestSnapshotPolicyEveryInput tests the EVERY_INPUT snapshot policy
// with Authority consensus.
func (s *SnapshotPolicySuite) TestSnapshotPolicyEveryInput() {
	s.runSnapshotPolicyTest(snapshotPolicyConfig{
		Policy: model.SnapshotPolicy_EveryInput,
	})
}

// TestSnapshotPolicyEveryEpoch tests the EVERY_EPOCH snapshot policy
// with Authority consensus.
func (s *SnapshotPolicySuite) TestSnapshotPolicyEveryEpoch() {
	s.runSnapshotPolicyTest(snapshotPolicyConfig{
		Policy: model.SnapshotPolicy_EveryEpoch,
	})
}

// TestSnapshotPolicyEveryInputPrt tests the EVERY_INPUT snapshot policy
// with PRT (Dave) consensus.
func (s *SnapshotPolicySuite) TestSnapshotPolicyEveryInputPrt() {
	// PRT settlement mines hundreds of blocks rapidly, which can cause
	// transient BlockOutOfRangeError in the EVM reader.
	s.SetExpectedLogs(s.T(), ExpectedLog{
		Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
		Level:   LevelError,
		Reason:  "transient Anvil error during rapid block mining in PRT settlement",
	})

	endpoint := envOrDefault(
		"CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	ethClient, err := ethclient.Dial(endpoint)
	s.Require().NoError(err, "dial ethclient")
	defer ethClient.Close()

	s.runSnapshotPolicyTest(snapshotPolicyConfig{
		Policy:          model.SnapshotPolicy_EveryInput,
		ExtraDeployArgs: []string{"--prt"},
		PreClaimHook: func(
			ctx context.Context, t testing.TB,
			require *require.Assertions, appName string,
		) {
			settleTournament(ctx, t, require, ethClient, appName, 0)
			settleTournament(ctx, t, require, ethClient, appName, 1)
		},
	})
}

// TestSnapshotPolicyEveryEpochPrt tests the EVERY_EPOCH snapshot policy
// with PRT (Dave) consensus.
func (s *SnapshotPolicySuite) TestSnapshotPolicyEveryEpochPrt() {
	// PRT settlement mines hundreds of blocks rapidly, which can cause
	// transient BlockOutOfRangeError in the EVM reader.
	s.SetExpectedLogs(s.T(), ExpectedLog{
		Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
		Level:   LevelError,
		Reason:  "transient Anvil error during rapid block mining in PRT settlement",
	})

	endpoint := envOrDefault(
		"CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	ethClient, err := ethclient.Dial(endpoint)
	s.Require().NoError(err, "dial ethclient")
	defer ethClient.Close()

	s.runSnapshotPolicyTest(snapshotPolicyConfig{
		Policy:          model.SnapshotPolicy_EveryEpoch,
		ExtraDeployArgs: []string{"--prt"},
		PreClaimHook: func(
			ctx context.Context, t testing.TB,
			require *require.Assertions, appName string,
		) {
			settleTournament(ctx, t, require, ethClient, appName, 0)
			settleTournament(ctx, t, require, ethClient, appName, 1)
		},
	})
}

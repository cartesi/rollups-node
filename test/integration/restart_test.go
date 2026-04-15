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
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RestartSuite struct {
	suite.Suite
	LogChecker
	ctx      context.Context
	cancel   context.CancelFunc
	app1Name string
	app2Name string
}

func TestRestart(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("skipping: node is externally managed (compose); " +
			"restart tests require test-managed node")
	}
	suite.Run(t, new(RestartSuite))
}

func (s *RestartSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(
		context.Background(), 25*time.Minute)
}

func (s *RestartSuite) TearDownSuite() {
	// If a test failed mid-restart, the node may be stopped. Restart it
	// so subsequent test suites have a running node.
	if sharedNode == nil {
		s.T().Log("Restarting shared node for subsequent tests...")
		startSharedNode(s.T())
	}
	s.cancel()
}

func (s *RestartSuite) SetupTest() {
	s.StartLogCapture()
	s.app1Name = ""
	s.app2Name = ""
}

func (s *RestartSuite) TearDownTest() {
	for _, name := range []string{s.app1Name, s.app2Name} {
		if name != "" {
			s.T().Logf("Disabling application %s", name)
			if err := disableApplication(s.ctx, name); err != nil {
				s.T().Logf(
					"warning: failed to disable %s: %v", name, err)
			}
		}
	}
	s.CheckLogs(s.T())
}

// restartConfig configures the shared restart test flow.
type restartConfig struct {
	// ExtraDeployArgs are additional CLI flags for deploy (e.g., "--prt").
	ExtraDeployArgs []string
	// PreClaimHook, if non-nil, is called after outputs are verified
	// but before the claim/execution phase.
	PreClaimHook func(
		ctx context.Context, t testing.TB,
		require *require.Assertions, appName string,
	)
}

// runRestartTest is the shared flow for both Authority and PRT restart tests.
// It deploys two apps, processes inputs, restarts the node, sends more inputs,
// and verifies the full L1 pipeline.
func (s *RestartSuite) runRestartTest(cfg restartConfig) {
	require := s.Require()
	dappPath := envOrDefault(
		"CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	defer timed(s.T(), "full restart multi-app lifecycle test")()

	// === Phase 1: Deploy two apps and process inputs ===

	s.T().Log("--- Phase 1: Deploy two apps and process inputs before restart ---")

	func() {
		defer timed(s.T(), "deploy two echo-dapps")()

		deployArgs1 := append(
			[]string{"--salt", uniqueSalt()}, cfg.ExtraDeployArgs...)
		deployArgs2 := append(
			[]string{"--salt", uniqueSalt()}, cfg.ExtraDeployArgs...)

		s.T().Logf("    deploying app-1: name=%s", s.app1Name)
		addr1, err := deployApplication(
			s.ctx, s.app1Name, dappPath, deployArgs1...)
		require.NoError(err, "deploy app-1")
		s.T().Logf("    app-1 deployed at %s", addr1)

		s.T().Logf("    deploying app-2: name=%s", s.app2Name)
		addr2, err := deployApplication(
			s.ctx, s.app2Name, dappPath, deployArgs2...)
		require.NoError(err, "deploy app-2")
		s.T().Logf("    app-2 deployed at %s", addr2)

		err = anvilSetBalance(s.ctx, addr1, oneEtherWei)
		require.NoError(err, "fund app-1 contract")
		err = anvilSetBalance(s.ctx, addr2, oneEtherWei)
		require.NoError(err, "fund app-2 contract")
		s.T().Log("    both apps funded with 1 ETH")
	}()

	s.T().Log("Sending one input to each app before restart...")
	idx1, _, err := sendInput(s.ctx, s.app1Name, "pre-restart-1")
	require.NoError(err, "send input to app-1")
	s.T().Logf("    app-1: input sent (index=%d)", idx1)

	idx2, _, err := sendInput(s.ctx, s.app2Name, "pre-restart-2")
	require.NoError(err, "send input to app-2")
	s.T().Logf("    app-2: input sent (index=%d)", idx2)

	func() {
		defer timed(s.T(), "wait for pre-restart inputs")()
		processCtx, processCancel := context.WithTimeout(
			s.ctx, inputProcessingTimeout)
		defer processCancel()

		input1, err := waitForInputProcessed(
			processCtx, s.T(), s.app1Name, idx1)
		require.NoError(err, "wait for app-1 input processing")
		require.Equal(
			model.InputCompletionStatus_Accepted, input1.Status)
		s.T().Log("    app-1: input ACCEPTED")

		input2, err := waitForInputProcessed(
			processCtx, s.T(), s.app2Name, idx2)
		require.NoError(err, "wait for app-2 input processing")
		require.Equal(
			model.InputCompletionStatus_Accepted, input2.Status)
		s.T().Log("    app-2: input ACCEPTED")
	}()

	s.T().Log("Verifying pre-restart outputs...")
	outputs1, err := readOutputs(s.ctx, s.app1Name)
	require.NoError(err, "read app-1 outputs")
	require.Equal(
		uint64(echoOutputsPerInput), outputs1.Pagination.TotalCount,
		"app-1 should have %d outputs", echoOutputsPerInput)

	outputs2, err := readOutputs(s.ctx, s.app2Name)
	require.NoError(err, "read app-2 outputs")
	require.Equal(
		uint64(echoOutputsPerInput), outputs2.Pagination.TotalCount,
		"app-2 should have %d outputs", echoOutputsPerInput)
	s.T().Logf("    both apps have %d outputs each — correct",
		echoOutputsPerInput)

	// === Phase 2: Stop and restart the node ===

	s.T().Log("--- Phase 2: Restarting node to test machine synchronization ---")
	func() {
		defer timed(s.T(), "node restart cycle")()
		stopSharedNode(s.T())
		startSharedNode(s.T())
	}()

	// === Phase 3: Send more inputs and verify ===

	s.T().Log("--- Phase 3: Send inputs after restart and verify ---")

	s.T().Log("Sending one input to each app after restart...")
	idx1b, _, err := sendInput(s.ctx, s.app1Name, "post-restart-1")
	require.NoError(err, "send post-restart input to app-1")
	s.T().Logf("    app-1: input sent (index=%d)", idx1b)

	idx2b, _, err := sendInput(s.ctx, s.app2Name, "post-restart-2")
	require.NoError(err, "send post-restart input to app-2")
	s.T().Logf("    app-2: input sent (index=%d)", idx2b)

	func() {
		defer timed(s.T(), "wait for post-restart inputs")()
		processCtx, processCancel := context.WithTimeout(
			s.ctx, inputProcessingTimeout)
		defer processCancel()

		input1b, err := waitForInputProcessed(
			processCtx, s.T(), s.app1Name, idx1b)
		require.NoError(err, "wait for app-1 post-restart input")
		require.Equal(
			model.InputCompletionStatus_Accepted, input1b.Status)
		s.T().Log("    app-1: post-restart input ACCEPTED")

		input2b, err := waitForInputProcessed(
			processCtx, s.T(), s.app2Name, idx2b)
		require.NoError(err, "wait for app-2 post-restart input")
		require.Equal(
			model.InputCompletionStatus_Accepted, input2b.Status)
		s.T().Log("    app-2: post-restart input ACCEPTED")
	}()

	s.T().Log("Verifying post-restart outputs...")
	outputs1after, err := readOutputs(s.ctx, s.app1Name)
	require.NoError(err, "read app-1 outputs after restart")
	require.Equal(
		uint64(2*echoOutputsPerInput),
		outputs1after.Pagination.TotalCount,
		"app-1 should have %d outputs after 2 inputs",
		2*echoOutputsPerInput)

	outputs2after, err := readOutputs(s.ctx, s.app2Name)
	require.NoError(err, "read app-2 outputs after restart")
	require.Equal(
		uint64(2*echoOutputsPerInput),
		outputs2after.Pagination.TotalCount,
		"app-2 should have %d outputs after 2 inputs",
		2*echoOutputsPerInput)
	s.T().Logf(
		"    both apps have %d outputs each after restart — correct",
		2*echoOutputsPerInput)

	s.T().Log("Node survived restart and both apps continued processing")

	// === Optional pre-claim hook (e.g. PRT tournament settlement) ===

	if cfg.PreClaimHook != nil {
		cfg.PreClaimHook(s.ctx, s.T(), require, s.app1Name)
	}

	// === Verify full L1 pipeline for app-1 ===

	s.T().Log("Verifying claim and execution for app-1...")
	epochIndex := outputs1.Data[0].EpochIndex

	var voucherIdx, noticeIdx uint64
	voucherFound, noticeFound := false, false
	for _, out := range outputs1.Data {
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
	require.True(voucherFound, "app-1: voucher not found")
	require.True(noticeFound, "app-1: notice not found")

	verifyClaimAndExecute(s.ctx, s.T(), require, verifyAndExecuteConfig{
		AppName:      s.app1Name,
		EpochIndex:   epochIndex,
		EpochOutputs: outputs1.Data,
		VoucherIdx:   voucherIdx,
		NoticeIdx:    noticeIdx,
	})

	s.T().Log("=== Restart lifecycle complete: " +
		"both apps survived restart, full L1 pipeline verified ===")
}

// TestRestartMultiAppAuthority tests restart with Authority consensus.
func (s *RestartSuite) TestRestartMultiAppAuthority() {
	s.app1Name = uniqueAppName("restart-auth-1")
	s.app2Name = uniqueAppName("restart-auth-2")
	s.runRestartTest(restartConfig{})
}

// TestRestartMultiAppPrt tests restart with PRT (Dave) consensus.
func (s *RestartSuite) TestRestartMultiAppPrt() {
	// PRT settlement mines hundreds of blocks rapidly, which can cause
	// transient BlockOutOfRangeError in the EVM reader.
	s.SetExpectedLogs(s.T(), ExpectedLog{
		Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
		Level:   LevelError,
		Reason:  "transient Anvil error during rapid block mining in PRT settlement",
	})

	s.app1Name = uniqueAppName("restart-prt-1")
	s.app2Name = uniqueAppName("restart-prt-2")

	endpoint := envOrDefault(
		"CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	ethClient, err := ethclient.Dial(endpoint)
	s.Require().NoError(err, "dial ethclient")
	defer ethClient.Close()

	s.runRestartTest(restartConfig{
		ExtraDeployArgs: []string{"--prt"},
		PreClaimHook: func(
			ctx context.Context, t testing.TB,
			require *require.Assertions, _ string,
		) {
			// Settle tournaments for BOTH apps together. Mining blocks
			// for one app's tournament timeout also advances the shared
			// chain, so we must ensure all apps' commitments are joined
			// before mining — otherwise the other app's tournament can
			// time out without a commitment ("finished without winners").
			apps := []string{s.app1Name, s.app2Name}
			for _, epochIdx := range []uint64{0, 1} {
				var tournaments []*model.Tournament
				for _, name := range apps {
					t.Logf("Waiting for %s epoch %d "+
						"tournament and commitment...",
						name, epochIdx)
					tour := waitForTournamentAndCommitment(
						ctx, t, require, name, epochIdx)
					tournaments = append(tournaments, tour)
				}
				for i, tour := range tournaments {
					blocksMined, err := mineForTournamentTimeout(
						ctx, ethClient, tour.Address)
					require.NoError(err,
						"mine for %s epoch %d timeout",
						apps[i], epochIdx)
					if blocksMined > 0 {
						t.Logf("    mined %d blocks for %s epoch %d",
							blocksMined, apps[i], epochIdx)
					}
				}
				for _, name := range apps {
					waitForTournamentWinner(
						ctx, t, require, name, epochIdx)
				}
				t.Logf("    epoch %d settled for both apps", epochIdx)
			}
		},
	})
}

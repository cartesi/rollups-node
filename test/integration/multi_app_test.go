// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/suite"
)

type MultiAppSuite struct {
	suite.Suite
	LogChecker
	ctx    context.Context
	cancel context.CancelFunc

	app1Name string
	app2Name string
	app1Addr string
	app2Addr string
}

func TestMultiApp(t *testing.T) {
	suite.Run(t, new(MultiAppSuite))
}

func (s *MultiAppSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 10*time.Minute)
	s.app1Name = uniqueAppName("multi-app-1")
	s.app2Name = uniqueAppName("multi-app-2")

	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")

	// Deploy two independent echo-dapp instances with unique random salts
	// to avoid CREATE2 address collisions (same factory + same template hash)
	// and to ensure tests are idempotent across re-runs.
	s.T().Log("--- Setup: deploying two independent echo-dapps to test isolation ---")
	s.T().Logf("    deploying app-1: name=%s", s.app1Name)
	addr1, err := deployApplication(s.ctx, s.app1Name, dappPath, "--salt", uniqueSalt())
	s.Require().NoError(err, "deploy app-1")
	s.app1Addr = addr1
	s.T().Logf("    app-1 deployed at %s", addr1)

	s.T().Logf("    deploying app-2: name=%s", s.app2Name)
	addr2, err := deployApplication(s.ctx, s.app2Name, dappPath, "--salt", uniqueSalt())
	s.Require().NoError(err, "deploy app-2")
	s.app2Addr = addr2
	s.T().Logf("    app-2 deployed at %s", addr2)

	// Fund both application contracts so they can execute vouchers.
	err = anvilSetBalance(s.ctx, addr1, oneEtherWei)
	s.Require().NoError(err, "fund app-1 contract")
	err = anvilSetBalance(s.ctx, addr2, oneEtherWei)
	s.Require().NoError(err, "fund app-2 contract")
	s.T().Log("    both apps funded with 1 ETH for voucher execution")
}

func (s *MultiAppSuite) TearDownSuite() {
	s.cancel()
}

func (s *MultiAppSuite) SetupTest() {
	s.StartLogCapture()
}

func (s *MultiAppSuite) TearDownTest() {
	for _, name := range []string{s.app1Name, s.app2Name} {
		if name != "" {
			s.T().Logf("Disabling application %s", name)
			if err := disableApplication(s.ctx, name); err != nil {
				s.T().Errorf("failed to disable application %s: %v", name, err)
			}
		}
	}
	s.CheckLogs(s.T())
}

// TestMultiAppIsolation verifies that two applications running on the same node
// process inputs independently and produce isolated outputs, reports, and claims.
func (s *MultiAppSuite) TestMultiAppIsolation() {
	require := s.Require()
	payload1 := "input-for-app-1"
	payload2 := "input-for-app-2"
	defer timed(s.T(), "full multi-app isolation test")()

	// --- L1 -> Machine: send one input to each app and verify independent processing ---

	s.T().Log("Sending one input to each app — they should process independently with separate input indices...")
	idx1, _, err := sendInput(s.ctx, s.app1Name, payload1)
	require.NoError(err, "send input to app-1")
	require.Equal(uint64(0), idx1)
	s.T().Logf("    app-1: input sent (index=%d)", idx1)

	idx2, _, err := sendInput(s.ctx, s.app2Name, payload2)
	require.NoError(err, "send input to app-2")
	require.Equal(uint64(0), idx2, "app-2 should start at input index 0 independently")
	s.T().Logf("    app-2: input sent (index=%d) — independent counter, also starts at 0", idx2)

	func() {
		defer timed(s.T(), "wait for both apps to process inputs")()
		s.T().Log("Waiting for both apps to process their inputs through separate Cartesi Machine instances...")
		processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
		defer processCancel()

		input1, err := waitForInputProcessed(processCtx, s.T(), s.app1Name, idx1)
		require.NoError(err, "wait for app-1 input processing")
		require.Equal(model.InputCompletionStatus_Accepted, input1.Status)
		s.T().Log("    app-1: input ACCEPTED")

		input2, err := waitForInputProcessed(processCtx, s.T(), s.app2Name, idx2)
		require.NoError(err, "wait for app-2 input processing")
		require.Equal(model.InputCompletionStatus_Accepted, input2.Status)
		s.T().Log("    app-2: input ACCEPTED")
	}()

	// --- Verify output and report isolation between apps ---

	s.T().Log("Checking output isolation — each app should have exactly 3 outputs from its own input...")
	outputs1, err := readOutputs(s.ctx, s.app1Name)
	require.NoError(err, "read app-1 outputs")
	require.Equal(uint64(echoOutputsPerInput), outputs1.Pagination.TotalCount,
		"app-1 should have %d outputs", echoOutputsPerInput)

	outputs2, err := readOutputs(s.ctx, s.app2Name)
	require.NoError(err, "read app-2 outputs")
	require.Equal(uint64(echoOutputsPerInput), outputs2.Pagination.TotalCount,
		"app-2 should have %d outputs", echoOutputsPerInput)
	s.T().Logf("    both apps have %d outputs each — isolated", echoOutputsPerInput)

	s.T().Log("Checking report isolation — each app should have exactly 1 report...")
	reports1, err := readReports(s.ctx, s.app1Name)
	require.NoError(err, "read app-1 reports")
	require.Equal(uint64(echoReportsPerInput), reports1.Pagination.TotalCount,
		"app-1 should have %d report(s)", echoReportsPerInput)

	reports2, err := readReports(s.ctx, s.app2Name)
	require.NoError(err, "read app-2 reports")
	require.Equal(uint64(echoReportsPerInput), reports2.Pagination.TotalCount,
		"app-2 should have %d report(s)", echoReportsPerInput)
	s.T().Logf("    both apps have %d report(s) each — isolated", echoReportsPerInput)

	// --- Cross-app isolation: sending to app-1 must not affect app-2 ---

	s.T().Log("Sending a second input to app-1 only — app-2 output count must remain unchanged...")
	idx1b, _, err := sendInput(s.ctx, s.app1Name, "second-input")
	require.NoError(err, "send second input to app-1")
	require.Equal(uint64(1), idx1b, "app-1 second input should be index 1")

	func() {
		defer timed(s.T(), "wait for app-1 second input")()
		processCtx2, processCancel2 := context.WithTimeout(s.ctx, inputProcessingTimeout)
		defer processCancel2()

		input1b, err := waitForInputProcessed(processCtx2, s.T(), s.app1Name, idx1b)
		require.NoError(err, "wait for app-1 second input processing")
		require.Equal(model.InputCompletionStatus_Accepted, input1b.Status)
	}()

	outputs1after, err := readOutputs(s.ctx, s.app1Name)
	require.NoError(err, "read app-1 outputs after second input")
	require.Equal(uint64(2*echoOutputsPerInput), outputs1after.Pagination.TotalCount,
		"app-1 should have %d outputs after 2 inputs", 2*echoOutputsPerInput)

	outputs2after, err := readOutputs(s.ctx, s.app2Name)
	require.NoError(err, "read app-2 outputs after app-1 second input")
	require.Equal(uint64(echoOutputsPerInput), outputs2after.Pagination.TotalCount,
		"app-2 should still have %d outputs", echoOutputsPerInput)
	s.T().Logf("    app-1 grew to %d outputs, app-2 still has %d — no cross-contamination",
		2*echoOutputsPerInput, echoOutputsPerInput)

	// --- Consensus + L1 execution for both apps independently ---

	client := newIntegrationEthClient(s.ctx, s.T())
	defer client.Close()
	var maxLastBlock uint64
	for _, app := range []struct {
		name    string
		outputs *api.ListResponse[api.DecodedOutput]
	}{
		{s.app1Name, outputs1},
		{s.app2Name, outputs2},
	} {
		epoch, err := readEpoch(s.ctx, app.name, app.outputs.Data[0].EpochIndex)
		require.NoError(err, "read %s epoch %d before claim verification", app.name, app.outputs.Data[0].EpochIndex)
		maxLastBlock = max(maxLastBlock, epoch.LastBlock)
	}
	currentBlock, err := client.BlockNumber(s.ctx)
	require.NoError(err, "read current block")
	if currentBlock <= maxLastBlock {
		require.NoError(anvilMine(s.ctx, int(maxLastBlock-currentBlock+1)), //nolint:gosec
			"mine past latest multi-app epoch")
	}

	s.T().Log("Verifying claims and executing outputs on both apps independently...")
	for _, app := range []struct {
		name    string
		outputs *api.ListResponse[api.DecodedOutput]
	}{
		{s.app1Name, outputs1},
		{s.app2Name, outputs2},
	} {
		epochIndex := app.outputs.Data[0].EpochIndex

		var voucherIdx, noticeIdx uint64
		voucherFound, noticeFound := false, false
		for _, out := range app.outputs.Data {
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
		require.True(voucherFound, "%s: voucher output not found", app.name)
		require.True(noticeFound, "%s: notice output not found", app.name)

		verifyClaimAndExecute(s.ctx, s.T(), require, verifyAndExecuteConfig{
			AppName:      app.name,
			EpochIndex:   epochIndex,
			EpochOutputs: app.outputs.Data,
			VoucherIdx:   voucherIdx,
			NoticeIdx:    noticeIdx,
		})
	}

	s.T().Log("=== Multi-app isolation complete: independent processing, claims, and L1 execution verified ===")
}

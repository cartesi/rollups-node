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
	"fmt"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
)

// Timeouts for polling operations.
const (
	inputProcessingTimeout = 3 * time.Minute
	claimAcceptedTimeout   = 5 * time.Minute
)

// oneEtherWei is 1 ETH in wei (10^18), hex-encoded for Anvil RPC calls.
// Used to fund application contracts so vouchers can transfer ETH during tests.
const oneEtherWei = "0xde0b6b3a7640000"

// Expected output/report counts per echo-dapp input.
// The ioctl-echo-loop dapp produces 3 outputs (voucher + delegatecall voucher + notice)
// and 1 report per accepted input. These constants keep assertions in sync with dapp behavior.
const (
	echoOutputsPerInput           = 3 // Voucher + DelegateCallVoucher + Notice
	echoReportsPerInput           = 1
	rejectOutputsPerAcceptedInput = 2 // Voucher + Notice (no DelegateCallVoucher)
	rejectReportsPerAcceptedInput = 1
)

// echoLifecycleConfig configures a shared echo lifecycle test that covers:
// deploy, send input, verify outputs/reports, wait for claim, verify proofs,
// execute voucher, and validate notice.
type echoLifecycleConfig struct {
	// AppName is the unique application name for deployment.
	AppName string
	// DappPath is the filesystem path to the dapp snapshot.
	DappPath string
	// Payload is the input payload to send.
	Payload string
	// ExtraDeployArgs are additional CLI flags for deploy (e.g., "--prt").
	ExtraDeployArgs []string
	// PreClaimHook, if non-nil, is called after outputs are verified
	// but before the claim/execution phase. For PRT consensus, this settles
	// tournaments; for other consensus types, it can perform any required
	// pre-claim actions.
	// It receives the application name.
	PreClaimHook func(ctx context.Context, t testing.TB, require *require.Assertions, appName string)
}

// runEchoLifecycleTest runs the full L1->Machine->L1 pipeline for an echo-dapp.
// It works for both Authority and PRT consensus, controlled by the config.
func runEchoLifecycleTest(ctx context.Context, t testing.TB, require *require.Assertions, cfg echoLifecycleConfig) {
	// --- Setup: deploy and fund ---

	t.Logf("--- Setup: deploying echo-dapp %s ---", cfg.AppName)
	t.Logf("    dapp=%s path=%s", cfg.AppName, cfg.DappPath)
	defer timed(t, "full echo lifecycle")()

	deployArgs := append([]string{"--salt", uniqueSalt()}, cfg.ExtraDeployArgs...)

	func() {
		defer timed(t, "deploy echo-dapp")()
		appAddr, err := deployApplication(ctx, cfg.AppName, cfg.DappPath, deployArgs...)
		require.NoError(err, "deploy echo-dapp")
		t.Logf("    application deployed at %s", appAddr)

		err = anvilSetBalance(ctx, appAddr, oneEtherWei)
		require.NoError(err, "fund application contract")
		t.Log("    funded application contract with 1 ETH for voucher execution")
	}()

	// --- L1 -> Machine: send input and wait for processing ---

	t.Logf("Sending input payload=%q to the echo-dapp on L1", cfg.Payload)
	inputIndex, blockNum, err := sendInput(ctx, cfg.AppName, cfg.Payload)
	require.NoError(err, "send input")
	require.Equal(uint64(0), inputIndex)
	t.Logf("    input accepted on-chain: index=%d block=%d", inputIndex, blockNum)

	func() {
		defer timed(t, "wait for input processing")()
		t.Log("Waiting for the advancer to pick up the input and run it through the Cartesi Machine...")
		processCtx, processCancel := context.WithTimeout(ctx, inputProcessingTimeout)
		defer processCancel()

		input, err := waitForInputProcessed(processCtx, t, cfg.AppName, inputIndex)
		require.NoError(err, "wait for input processing")
		require.Equal(model.InputCompletionStatus_Accepted, input.Status)
		t.Log("    machine processed the input and returned ACCEPTED")
	}()

	// --- Verify off-chain results: outputs and reports ---

	t.Log("Checking that the echo-dapp produced the expected outputs (voucher, delegate-call voucher, notice)...")
	outputsResp, err := readOutputs(ctx, cfg.AppName)
	require.NoError(err, "read outputs")
	require.Equal(uint64(echoOutputsPerInput), outputsResp.Pagination.TotalCount,
		"expected %d outputs (voucher + delegatecall voucher + notice)", echoOutputsPerInput)
	require.Len(outputsResp.Data, echoOutputsPerInput)

	var voucherIdx, noticeIdx uint64
	voucherFound, delegateVoucherFound, noticeFound := false, false, false
	for _, out := range outputsResp.Data {
		if out.DecodedData == nil {
			continue
		}
		switch out.DecodedData.Type {
		case "Voucher":
			voucherIdx = out.Index
			voucherFound = true
		case "DelegateCallVoucher":
			delegateVoucherFound = true
		case "Notice":
			noticeIdx = out.Index
			noticeFound = true
		}
	}
	require.True(voucherFound, "voucher output not found")
	require.True(delegateVoucherFound, "delegate call voucher output not found")
	require.True(noticeFound, "notice output not found")

	reportsResp, err := readReports(ctx, cfg.AppName)
	require.NoError(err, "read reports")
	require.Equal(uint64(echoReportsPerInput), reportsResp.Pagination.TotalCount,
		"expected %d report(s)", echoReportsPerInput)
	t.Log("    all outputs and reports verified")

	// --- Optional pre-claim hook (e.g. PRT tournament settlement) ---

	if cfg.PreClaimHook != nil {
		cfg.PreClaimHook(ctx, t, require, cfg.AppName)
	}

	// --- Consensus + L1 execution (shared phase) ---

	epochIndex := outputsResp.Data[0].EpochIndex

	verifyClaimAndExecute(ctx, t, require, verifyAndExecuteConfig{
		AppName:          cfg.AppName,
		EpochIndex:       epochIndex,
		EpochOutputs:     outputsResp.Data,
		VoucherIdx:       voucherIdx,
		NoticeIdx:        noticeIdx,
		CheckReExecution: true,
	})
}

// rejectExceptionLifecycleConfig configures a shared reject/exception lifecycle
// test that covers: deploy, send 3 inputs (where input #1 is rejected/exception),
// verify outputs/reports, wait for claim, verify proofs, execute voucher, and
// validate notice.
type rejectExceptionLifecycleConfig struct {
	// AppName is the unique application name for deployment.
	AppName string
	// DappPath is the filesystem path to the dapp snapshot.
	DappPath string
	// TestName is used for log messages (e.g., "reject", "exception").
	TestName string
	// FailStatus is the expected status for the failed input (e.g., REJECTED, EXCEPTION).
	FailStatus model.InputCompletionStatus
	// ExtraDeployArgs are additional CLI flags for deploy (e.g., "--prt").
	ExtraDeployArgs []string
	// PreClaimHook, if non-nil, is called after outputs are verified
	// but before the claim/execution phase. For PRT consensus, this settles
	// tournaments; for other consensus types, it can perform any required
	// pre-claim actions.
	// It receives the application name.
	PreClaimHook func(ctx context.Context, t testing.TB, require *require.Assertions, appName string)
	// EpochIndex, if non-nil, overrides the epoch used for claim/execution.
	// If nil, the first output's epoch index is used.
	EpochIndex *uint64
}

// runRejectExceptionLifecycleTest runs the reject/exception pipeline for a dapp
// that rejects or throws on input index 1. Works for both Authority and PRT
// consensus, controlled by the config.
func runRejectExceptionLifecycleTest(
	ctx context.Context,
	t testing.TB,
	require *require.Assertions,
	cfg rejectExceptionLifecycleConfig,
) {
	// --- Setup: deploy and fund the dapp ---

	t.Logf("--- Setup: deploying %s-loop-dapp (will %s input #1) ---", cfg.TestName, cfg.FailStatus)
	t.Logf("    dapp=%s path=%s", cfg.AppName, cfg.DappPath)
	defer timed(t, fmt.Sprintf("full %s lifecycle", cfg.TestName))()

	deployArgs := append([]string{"--salt", uniqueSalt()}, cfg.ExtraDeployArgs...)

	func() {
		defer timed(t, fmt.Sprintf("deploy %s-loop-dapp", cfg.TestName))()
		appAddr, err := deployApplication(ctx, cfg.AppName, cfg.DappPath, deployArgs...)
		require.NoError(err, "deploy %s-loop-dapp", cfg.TestName)
		t.Logf("    application deployed at %s", appAddr)

		err = anvilSetBalance(ctx, appAddr, oneEtherWei)
		require.NoError(err, "fund application contract")
		t.Log("    funded application contract with 1 ETH for voucher execution")
	}()

	// --- L1 -> Machine: send 3 inputs where input #1 will be rejected/exception ---

	t.Logf("Sending 3 inputs — the dapp will %s input #1 while accepting #0 and #2...", cfg.FailStatus)
	const numInputs = 3
	for i := range numInputs {
		payload := fmt.Sprintf("%s-payload-%d", cfg.TestName, i)
		idx, _, err := sendInput(ctx, cfg.AppName, payload)
		require.NoError(err, "send input %d", i)
		require.Equal(uint64(i), idx, "input index mismatch")
		t.Logf("    input %d sent (payload=%q)", i, payload)
	}

	func() {
		defer timed(t, "wait for input processing (3 inputs)")()
		t.Log("Waiting for the advancer to process all 3 inputs through the Cartesi Machine...")
		processCtx, processCancel := context.WithTimeout(ctx, inputProcessingTimeout)
		defer processCancel()

		expectedStatuses := map[uint64]model.InputCompletionStatus{
			0: model.InputCompletionStatus_Accepted,
			1: cfg.FailStatus,
			2: model.InputCompletionStatus_Accepted,
		}

		for i := range uint64(numInputs) {
			input, err := waitForInputProcessed(processCtx, t, cfg.AppName, i)
			require.NoError(err, "wait for input %d processing", i)
			require.Equal(expectedStatuses[i], input.Status,
				"input %d: expected status %s, got %s", i, expectedStatuses[i], input.Status)
			t.Logf("    input %d: %s", i, input.Status)
		}
	}()

	// --- Verify off-chain results: only accepted inputs produce outputs ---

	t.Logf("Checking outputs — only accepted inputs should produce them (no outputs from %s input #1)...",
		cfg.FailStatus)
	outputsResp, err := readOutputs(ctx, cfg.AppName)
	require.NoError(err, "read outputs")
	numAccepted := uint64(2)
	require.Equal(numAccepted*rejectOutputsPerAcceptedInput, outputsResp.Pagination.TotalCount,
		"expected %d outputs (%d per accepted input x %d accepted inputs)",
		numAccepted*rejectOutputsPerAcceptedInput, rejectOutputsPerAcceptedInput, numAccepted)

	for _, out := range outputsResp.Data {
		require.NotEqual(uint64(1), out.InputIndex,
			"output %d should not belong to %s input (index 1)", out.Index, cfg.FailStatus)
	}
	t.Logf("    %d outputs found, none from the failed input — correct",
		numAccepted*rejectOutputsPerAcceptedInput)

	t.Log("Checking reports — same rule: only accepted inputs produce reports...")
	reportsResp, err := readReports(ctx, cfg.AppName)
	require.NoError(err, "read reports")
	require.Equal(numAccepted*rejectReportsPerAcceptedInput, reportsResp.Pagination.TotalCount,
		"expected %d reports (%d per accepted input x %d accepted inputs)",
		numAccepted*rejectReportsPerAcceptedInput, rejectReportsPerAcceptedInput, numAccepted)
	t.Logf("    %d reports found — correct", numAccepted*rejectReportsPerAcceptedInput)

	// --- Optional pre-claim hook (e.g. PRT tournament settlement) ---

	if cfg.PreClaimHook != nil {
		cfg.PreClaimHook(ctx, t, require, cfg.AppName)
	}

	// --- Consensus + L1 execution (shared phase) ---

	var epochIndex uint64
	if cfg.EpochIndex != nil {
		epochIndex = *cfg.EpochIndex
	} else {
		epochIndex = outputsResp.Data[0].EpochIndex
	}

	// Collect outputs belonging to the claimed epoch and find a voucher + notice among them.
	var epochOutputs []api.DecodedOutput
	var voucherIdx, noticeIdx uint64
	voucherFound, noticeFound := false, false
	for _, out := range outputsResp.Data {
		if out.EpochIndex != epochIndex {
			continue
		}
		epochOutputs = append(epochOutputs, out)
		if out.DecodedData == nil {
			continue
		}
		switch out.DecodedData.Type {
		case "Voucher":
			if !voucherFound {
				voucherIdx = out.Index
				voucherFound = true
			}
		case "Notice":
			if !noticeFound {
				noticeIdx = out.Index
				noticeFound = true
			}
		}
	}
	require.True(voucherFound, "voucher output not found in epoch %d", epochIndex)
	require.True(noticeFound, "notice output not found in epoch %d", epochIndex)

	verifyClaimAndExecute(ctx, t, require, verifyAndExecuteConfig{
		AppName:      cfg.AppName,
		EpochIndex:   epochIndex,
		EpochOutputs: epochOutputs,
		VoucherIdx:   voucherIdx,
		NoticeIdx:    noticeIdx,
	})

	t.Logf("=== %s test complete: %s handling + L1 execution verified ===", cfg.TestName, cfg.FailStatus)
}

// verifyAndExecuteConfig describes the post-settlement verification phase:
// wait for claim, check proofs, execute voucher, validate notice.
type verifyAndExecuteConfig struct {
	AppName      string
	EpochIndex   uint64
	EpochOutputs []api.DecodedOutput
	VoucherIdx   uint64
	NoticeIdx    uint64
	// CheckReExecution, if true, verifies that re-executing the voucher
	// reverts (vouchers are single-use).
	CheckReExecution bool
}

// verifyClaimAndExecute waits for the epoch claim to be accepted, verifies
// Merkle proofs, executes a voucher, and validates a notice on L1. This is the
// shared tail of both echo and reject/exception lifecycle tests.
func verifyClaimAndExecute(
	ctx context.Context,
	t testing.TB,
	require *require.Assertions,
	cfg verifyAndExecuteConfig,
) {
	// --- Consensus: wait for claim acceptance ---

	func() {
		defer timed(t, "wait for claim acceptance")()
		t.Logf("Waiting for the claim to be accepted for epoch %d...", cfg.EpochIndex)
		claimCtx, claimCancel := context.WithTimeout(ctx, claimAcceptedTimeout)
		defer claimCancel()

		epoch, err := waitForEpochStatus(
			claimCtx, t, cfg.AppName, cfg.EpochIndex, model.EpochStatus_ClaimAccepted)
		require.NoError(err, "wait for claim accepted")
		require.NotNil(epoch.OutputsMerkleRoot, "epoch claim should be set")
		t.Logf("    epoch %d claim accepted (hash=%s)", cfg.EpochIndex, *epoch.OutputsMerkleRoot)
	}()

	// --- Verify Merkle proofs ---

	t.Logf("Verifying that Merkle proofs were generated for epoch %d outputs...", cfg.EpochIndex)
	for _, out := range cfg.EpochOutputs {
		refreshed, err := readOutput(ctx, cfg.AppName, out.Index)
		require.NoError(err, "read output %d", out.Index)
		require.NotEmpty(refreshed.OutputHashesSiblings,
			"output %d should have proof siblings", out.Index)
	}
	t.Log("    all outputs have valid Merkle proof siblings")

	// --- L1 execution: execute the voucher and validate the notice on-chain ---

	func() {
		defer timed(t, "L1 voucher execution + notice validation")()

		t.Logf("Executing voucher (output %d) on L1 — this calls the destination contract...", cfg.VoucherIdx)
		txHash, err := executeOutput(ctx, cfg.AppName, cfg.VoucherIdx)
		require.NoError(err, "execute voucher")
		require.NotEmpty(txHash)
		t.Logf("    voucher executed successfully (tx=%s)", txHash)

		t.Log("Waiting for the EVM reader to detect the execution event and record it in the DB...")
		execCtx, execCancel := context.WithTimeout(ctx, inputProcessingTimeout)
		defer execCancel()

		err = waitForExecutionRecorded(execCtx, t, cfg.AppName, cfg.VoucherIdx)
		require.NoError(err, "wait for execution tx hash in DB")
		t.Log("    execution event recorded in DB")

		t.Logf("Validating notice (output %d) on L1 — proving it was emitted by the machine...", cfg.NoticeIdx)
		err = validateOutput(ctx, cfg.AppName, cfg.NoticeIdx)
		require.NoError(err, "validate notice should succeed (no revert)")
		t.Log("    notice validated on-chain (proof accepted by the contract)")
	}()

	// --- Optional safety check: vouchers are single-use ---

	if cfg.CheckReExecution {
		t.Log("Attempting to execute the same voucher again (should revert — vouchers are single-use)...")
		_, err := executeOutput(ctx, cfg.AppName, cfg.VoucherIdx)
		require.Error(err, "second voucher execution should revert")
		t.Log("    correctly reverted — voucher cannot be replayed")
	}
}

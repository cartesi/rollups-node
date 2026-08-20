// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	jsonrpcclient "github.com/cartesi/rollups-node/pkg/jsonrpc/client"
	"github.com/stretchr/testify/suite"
)

type TerminalMachineStatesSuite struct {
	suite.Suite
	LogChecker
	ctx     context.Context
	cancel  context.CancelFunc
	appName string
}

type terminalMachineStateCase struct {
	namePrefix        string
	dappPathEnv       string
	defaultDappPath   string
	payloadPrefix     string
	description       string
	terminalInput     uint64
	inputStatus       model.InputCompletionStatus
	applicationStatus model.ApplicationStatus
}

const (
	terminalObservationWindow     = 6 * time.Second
	terminalObservationInterval   = 500 * time.Millisecond
	terminalObservationRPCTimeout = 2 * time.Second
)

var terminalMachineRestartExpectedLog = ExpectedLog{
	Pattern: regexp.MustCompile(`service=(?:claimer|evm-reader).*context canceled`),
	Level:   LevelError,
	Reason:  "benign service cancellation while deliberately restarting the node",
}

func TestTerminalMachineStates(t *testing.T) {
	if !isNodeSelfManaged() {
		t.Skip("skipping: durable terminal-state test requires a test-managed node restart")
	}
	suite.Run(t, new(TerminalMachineStatesSuite))
}

func (s *TerminalMachineStatesSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 12*time.Minute)
}

func (s *TerminalMachineStatesSuite) TearDownSuite() {
	// Restore the shared node if the test failed between stop and restart.
	if sharedNode == nil {
		s.T().Log("Restarting shared node for subsequent tests...")
		startSharedNode(s.T())
	}
	s.cancel()
}

func (s *TerminalMachineStatesSuite) SetupTest() {
	s.StartLogCapture()
	s.appName = ""
}

func (s *TerminalMachineStatesSuite) TearDownTest() {
	if s.appName != "" {
		s.T().Logf("Disabling application %s", s.appName)
		if err := disableApplication(s.ctx, s.appName); err != nil {
			s.T().Errorf("failed to disable application %s: %v", s.appName, err)
		}
	}
	s.CheckLogs(s.T())
}

func (s *TerminalMachineStatesSuite) TestMachineHaltSurvivesRestart() {
	s.runTerminalMachineState(terminalMachineStateCase{
		namePrefix:        "halt-loop",
		dappPathEnv:       "CARTESI_TEST_HALT_DAPP_PATH",
		defaultDappPath:   "applications/halt-loop-dapp",
		payloadPrefix:     "halt",
		description:       "a guest that accepts input 0 and exits while handling input 1",
		terminalInput:     1,
		inputStatus:       model.InputCompletionStatus_MachineHalted,
		applicationStatus: model.ApplicationStatus_MachineHalted,
	})
}

func (s *TerminalMachineStatesSuite) TestMcycleOverflowSurvivesRestart() {
	s.runTerminalMachineState(terminalMachineStateCase{
		namePrefix:        "mcycle-overflow",
		dappPathEnv:       "CARTESI_TEST_MCYCLE_OVERFLOW_DAPP_PATH",
		defaultDappPath:   "applications/mcycle-overflow-dapp",
		payloadPrefix:     "overflow",
		description:       "an accepted-yield machine with mcycle near UINT64_MAX",
		terminalInput:     0,
		inputStatus:       model.InputCompletionStatus_Overflow,
		applicationStatus: model.ApplicationStatus_McycleOverflow,
	})
}

func (s *TerminalMachineStatesSuite) TestUnexpectedYieldSurvivesRestart() {
	s.runTerminalMachineState(terminalMachineStateCase{
		namePrefix:        "unexpected-yield",
		dappPathEnv:       "CARTESI_TEST_UNEXPECTED_YIELD_DAPP_PATH",
		defaultDappPath:   "applications/unexpected-yield-dapp",
		payloadPrefix:     "unexpected-yield",
		description:       "a guest that issues unsupported manual yield reason 9",
		terminalInput:     0,
		inputStatus:       model.InputCompletionStatus_UnexpectedYield,
		applicationStatus: model.ApplicationStatus_UnexpectedYield,
	})
}

func (s *TerminalMachineStatesSuite) runTerminalMachineState(tc terminalMachineStateCase) {
	s.T().Helper()
	s.SetExpectedLogs(s.T(), terminalExecutionExpectedLog, terminalMachineRestartExpectedLog)
	require := s.Require()
	s.appName = uniqueAppName(tc.namePrefix)
	dappPath := envOrDefault(tc.dappPathEnv, tc.defaultDappPath)

	s.T().Logf("Deploying %s...", tc.description)
	_, err := deployApplication(s.ctx, s.appName, dappPath, "--salt", uniqueSalt())
	require.NoError(err, "deploy %s", tc.defaultDappPath)

	for index := range tc.terminalInput + 1 {
		got, _, sendErr := sendInput(s.ctx, s.appName,
			fmt.Sprintf("%s-payload-%d", tc.payloadPrefix, index))
		require.NoError(sendErr, "send input %d", index)
		require.Equal(index, got, "input index mismatch")
	}

	processCtx, processCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer processCancel()
	for index := range tc.terminalInput {
		accepted, waitErr := waitForInputProcessed(processCtx, s.T(), s.appName, index)
		require.NoError(waitErr, "wait for accepted input %d", index)
		require.Equal(model.InputCompletionStatus_Accepted, accepted.Status)
	}

	terminal, err := waitForInputProcessed(processCtx, s.T(), s.appName, tc.terminalInput)
	require.NoError(err, "wait for terminal input")
	require.Equal(tc.inputStatus, terminal.Status)

	rpc := jsonrpcclient.NewClient(
		envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc"),
	)
	app := s.getApplication(rpc)
	require.Equal(tc.applicationStatus, app.Status)
	require.NotNil(app.Reason)
	require.Equal(fmt.Sprintf("input %d completed with %s", tc.terminalInput, tc.inputStatus), *app.Reason)

	var epochResponse api.SingleResponse[*model.Epoch]
	err = rpc.Call(s.ctx, "cartesi_getEpoch", api.GetEpochParams{
		Application: s.appName,
		EpochIndex:  fmt.Sprintf("0x%x", terminal.EpochIndex),
	}, &epochResponse)
	require.NoError(err, "read terminal epoch through JSON-RPC")
	require.NotNil(epochResponse.Data)
	require.True(epochResponse.Data.HasCompleteStateProof(),
		"terminal epoch must expose all three machine-state proof leaves")

	outputs, err := readOutputs(s.ctx, s.appName)
	require.NoError(err, "read outputs")
	require.Zero(outputs.Pagination.TotalCount, "terminal fixture must not emit outputs")
	reports, err := readReports(s.ctx, s.appName)
	require.NoError(err, "read reports")
	require.Zero(reports.Pagination.TotalCount, "terminal fixture must not emit reports")

	s.T().Logf("Restarting the node after durable %s...", tc.inputStatus)
	stopSharedNode(s.T())
	startSharedNode(s.T())

	app = s.getApplication(rpc)
	require.Equal(tc.applicationStatus, app.Status)

	inputIndex, _, err := sendInput(s.ctx, s.appName, "post-terminal-payload")
	require.NoError(err, "send input after restart")
	require.Equal(tc.terminalInput+1, inputIndex)

	indexCtx, indexCancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	pending, err := waitForInputIndexed(indexCtx, s.T(), s.appName, inputIndex)
	indexCancel()
	require.NoError(err, "wait for post-halt input indexing")
	require.Equal(model.InputCompletionStatus_None, pending.Status)

	s.requireInputRemainsPending(rpc, inputIndex)

	app = s.getApplication(rpc)
	require.Equal(tc.applicationStatus, app.Status)
	s.T().Logf("The %s app remained observable, but execution did not restart", tc.inputStatus)
}

func (s *TerminalMachineStatesSuite) requireInputRemainsPending(
	rpc *jsonrpcclient.Client,
	inputIndex uint64,
) {
	s.T().Helper()
	require := s.Require()
	timer := time.NewTimer(terminalObservationWindow)
	defer timer.Stop()
	ticker := time.NewTicker(terminalObservationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return
		case <-ticker.C:
			callCtx, cancel := context.WithTimeout(s.ctx, terminalObservationRPCTimeout)
			var response api.SingleResponse[*model.Input]
			err := rpc.Call(callCtx, "cartesi_getInput", api.GetInputParams{
				Application: s.appName,
				InputIndex:  fmt.Sprintf("0x%x", inputIndex),
			}, &response)
			cancel()
			require.NoError(err, "observe post-terminal input")
			require.NotNil(response.Data, "post-terminal input disappeared")
			require.Equal(model.InputCompletionStatus_None, response.Data.Status,
				"a restarted node must not execute inputs after a durable terminal outcome")
		}
	}
}

func (s *TerminalMachineStatesSuite) getApplication(
	rpc *jsonrpcclient.Client,
) *model.Application {
	s.T().Helper()
	var response api.SingleResponse[*model.Application]
	err := rpc.Call(s.ctx, "cartesi_getApplication", api.GetApplicationParams{
		Application: s.appName,
	}, &response)
	s.Require().NoError(err, "read application through JSON-RPC")
	s.Require().NotNil(response.Data)
	return response.Data
}

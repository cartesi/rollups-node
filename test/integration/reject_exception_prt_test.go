// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

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

// prtBlockOutOfRangeAllowlist tolerates the transient Anvil
// BlockOutOfRangeError that surfaces when PRT settlement mines hundreds of
// blocks rapidly past the EVM reader's last polled head.
var prtBlockOutOfRangeAllowlist = ExpectedLog{
	Pattern: regexp.MustCompile(`BlockOutOfRangeError`),
	Level:   LevelError,
	Reason:  "transient Anvil error during rapid block mining in PRT settlement",
}

type RejectExceptionPrtSuite struct {
	suite.Suite
	LogChecker
	ctx      context.Context
	cancel   context.CancelFunc
	appNames []string

	ethClient *ethclient.Client
}

func TestRejectExceptionPrt(t *testing.T) {
	suite.Run(t, new(RejectExceptionPrtSuite))
}

func (s *RejectExceptionPrtSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 10*time.Minute)

	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.Dial(endpoint)
	s.Require().NoError(err, "dial ethclient")
	s.ethClient = client
}

func (s *RejectExceptionPrtSuite) TearDownSuite() {
	s.cancel()
	s.ethClient.Close()
}

func (s *RejectExceptionPrtSuite) SetupTest() {
	s.StartLogCapture()
	s.appNames = nil
}

func (s *RejectExceptionPrtSuite) TearDownTest() {
	for _, name := range s.appNames {
		s.T().Logf("Disabling application %s", name)
		if err := disableApplication(s.ctx, name); err != nil {
			s.T().Errorf("failed to disable application %s: %v", name, err)
		}
	}
	s.CheckLogs(s.T())
}

// TestRejectInputPrt deploys a reject-loop-dapp with PRT consensus,
// sends 3 inputs, and verifies that input 1 is REJECTED while inputs 0 and 2
// are ACCEPTED. Then settles tournaments and executes outputs on L1.
func (s *RejectExceptionPrtSuite) TestRejectInputPrt() {
	s.SetExpectedLogs(s.T(), prtBlockOutOfRangeAllowlist)

	ethClient := s.ethClient
	prtEpoch := uint64(1)
	appName := uniqueAppName("reject-prt-loop")
	s.appNames = append(s.appNames, appName)
	runRejectExceptionLifecycleTest(s.ctx, s.T(), s.Require(), rejectExceptionLifecycleConfig{
		AppName:         appName,
		DappPath:        envOrDefault("CARTESI_TEST_REJECT_DAPP_PATH", "applications/reject-loop-dapp"),
		TestName:        "reject",
		FailStatus:      model.InputCompletionStatus_Rejected,
		ExtraDeployArgs: []string{"--prt"},
		EpochIndex:      &prtEpoch,
		PreClaimHook: func(ctx context.Context, t testing.TB, require *require.Assertions, appName string) {
			settleTournament(ctx, t, require, ethClient, appName, 0)
			settleTournament(ctx, t, require, ethClient, appName, 1)
		},
	})
}

// TestExceptionInputPrt deploys an exception-loop-dapp with PRT consensus,
// sends 3 inputs, and verifies that input 1 is EXCEPTION while inputs 0 and 2
// are ACCEPTED. Then settles tournaments and executes outputs on L1.
func (s *RejectExceptionPrtSuite) TestExceptionInputPrt() {
	s.SetExpectedLogs(s.T(), prtBlockOutOfRangeAllowlist)

	ethClient := s.ethClient
	prtEpoch := uint64(1)
	appName := uniqueAppName("exception-prt-loop")
	s.appNames = append(s.appNames, appName)
	runRejectExceptionLifecycleTest(s.ctx, s.T(), s.Require(), rejectExceptionLifecycleConfig{
		AppName:         appName,
		DappPath:        envOrDefault("CARTESI_TEST_EXCEPTION_DAPP_PATH", "applications/exception-loop-dapp"),
		TestName:        "exception",
		FailStatus:      model.InputCompletionStatus_Exception,
		ExtraDeployArgs: []string{"--prt"},
		EpochIndex:      &prtEpoch,
		PreClaimHook: func(ctx context.Context, t testing.TB, require *require.Assertions, appName string) {
			settleTournament(ctx, t, require, ethClient, appName, 0)
			settleTournament(ctx, t, require, ethClient, appName, 1)
		},
	})
}

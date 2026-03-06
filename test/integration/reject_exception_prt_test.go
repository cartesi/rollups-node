// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RejectExceptionPrtSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc

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

// TestRejectInputPrt deploys a reject-loop-dapp with PRT consensus,
// sends 3 inputs, and verifies that input 1 is REJECTED while inputs 0 and 2
// are ACCEPTED. Then settles tournaments and executes outputs on L1.
func (s *RejectExceptionPrtSuite) TestRejectInputPrt() {
	ethClient := s.ethClient
	prtEpoch := uint64(1)
	runRejectExceptionLifecycleTest(s.ctx, s.T(), s.Require(), rejectExceptionLifecycleConfig{
		AppName:         uniqueAppName("reject-prt-loop"),
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
	ethClient := s.ethClient
	prtEpoch := uint64(1)
	runRejectExceptionLifecycleTest(s.ctx, s.T(), s.Require(), rejectExceptionLifecycleConfig{
		AppName:         uniqueAppName("exception-prt-loop"),
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

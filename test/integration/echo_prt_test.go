// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type EchoPrtSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc

	ethClient *ethclient.Client
}

func TestEchoPrt(t *testing.T) {
	suite.Run(t, new(EchoPrtSuite))
}

func (s *EchoPrtSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 10*time.Minute)

	endpoint := envOrDefault("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	client, err := ethclient.Dial(endpoint)
	s.Require().NoError(err, "dial ethclient")
	s.ethClient = client
}

func (s *EchoPrtSuite) TearDownSuite() {
	s.cancel()
	s.ethClient.Close()
}

// TestEchoPrtLifecycle tests the PRT (Dave consensus) path:
// deploy with --prt, send input, verify outputs/reports, then complete
// both epoch 0 (empty, sealed at deploy) and epoch 1 (with input) tournaments.
func (s *EchoPrtSuite) TestEchoPrtLifecycle() {
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	ethClient := s.ethClient

	runEchoLifecycleTest(s.ctx, s.T(), s.Require(), echoLifecycleConfig{
		AppName:         uniqueAppName("echo-prt"),
		DappPath:        dappPath,
		Payload:         "prt-hello",
		ExtraDeployArgs: []string{"--prt"},
		PreClaimHook: func(ctx context.Context, t testing.TB, require *require.Assertions, appName string) {
			settleTournament(ctx, t, require, ethClient, appName, 0)
			settleTournament(ctx, t, require, ethClient, appName, 1)
		},
	})

	s.T().Log("=== PRT lifecycle complete: L1 -> Machine -> Tournament -> L1 execution verified ===")
}

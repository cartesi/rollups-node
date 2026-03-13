// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type EchoAuthoritySuite struct {
	suite.Suite
	LogChecker
	ctx     context.Context
	cancel  context.CancelFunc
	appName string
}

func TestEchoAuthority(t *testing.T) {
	suite.Run(t, new(EchoAuthoritySuite))
}

func (s *EchoAuthoritySuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 10*time.Minute)
}

func (s *EchoAuthoritySuite) TearDownSuite() {
	s.cancel()
}

func (s *EchoAuthoritySuite) SetupTest() {
	s.StartLogCapture()
	s.appName = ""
}

func (s *EchoAuthoritySuite) TearDownTest() {
	if s.appName != "" {
		s.T().Logf("Disabling application %s", s.appName)
		if err := disableApplication(s.ctx, s.appName); err != nil {
			s.T().Errorf("failed to disable application %s: %v", s.appName, err)
		}
	}
	s.CheckLogs(s.T())
}

// TestEchoAuthorityLifecycle tests the full L1->Machine->L1 pipeline:
// deploy, send input, verify outputs, wait for claim, execute voucher, validate notice.
func (s *EchoAuthoritySuite) TestEchoAuthorityLifecycle() {
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("echo-authority")

	runEchoLifecycleTest(s.ctx, s.T(), s.Require(), echoLifecycleConfig{
		AppName:  s.appName,
		DappPath: dappPath,
		Payload:  "hello cartesi",
	})

	s.T().Log("=== Authority lifecycle complete: L1 → Machine → Proofs → L1 execution verified ===")
}

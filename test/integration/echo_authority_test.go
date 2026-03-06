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
	ctx    context.Context
	cancel context.CancelFunc
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

// TestEchoAuthorityLifecycle tests the full L1->Machine->L1 pipeline:
// deploy, send input, verify outputs, wait for claim, execute voucher, validate notice.
func (s *EchoAuthoritySuite) TestEchoAuthorityLifecycle() {
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")

	runEchoLifecycleTest(s.ctx, s.T(), s.Require(), echoLifecycleConfig{
		AppName:  uniqueAppName("echo-authority"),
		DappPath: dappPath,
		Payload:  "hello cartesi",
	})

	s.T().Log("=== Authority lifecycle complete: L1 → Machine → Proofs → L1 execution verified ===")
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"regexp"
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

// TestInspect verifies the inspect API happy path: a deployed echo-dapp
// returns the payload as a report with status Accepted. No errors are
// expected in the node logs — the polling loop gets 503 (machine not
// ready) until the advancer creates the machine, which is logged at
// WARN level, not ERR.
func (s *EchoAuthoritySuite) TestInspect() {
	dappPath := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	s.appName = uniqueAppName("echo-inspect")

	s.T().Logf("Deploying echo-dapp %s for inspect test", s.appName)
	_, err := deployApplication(s.ctx, s.appName, dappPath, "--salt", uniqueSalt())
	s.Require().NoError(err, "deploy echo-dapp")

	// Wait for the machine to be ready by polling inspect until it succeeds.
	s.T().Log("Waiting for inspect to become available...")
	var result *inspectResult
	deadline := time.After(2 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		result, err = inspectApplication(s.ctx, s.appName, "hello")
		if err == nil {
			break
		}
		select {
		case <-deadline:
			s.Require().NoError(err, "inspect did not become available within timeout")
		case <-ticker.C:
		}
	}

	s.Require().Equal("Accepted", result.Status)
	s.Require().Len(result.Reports, 1, "echo-dapp should return 1 report")
	s.T().Logf("Inspect returned status=%s reports=%d", result.Status, len(result.Reports))
	s.T().Log("=== Inspect happy path complete ===")
}

// TestInspectNotFound verifies that inspecting a non-existent application
// returns an error from the CLI and produces an ERR log in the node.
// The Required allowed error ensures the test fails if the node does NOT
// log the expected error.
func (s *EchoAuthoritySuite) TestInspectNotFound() {
	s.SetAllowedErrors(AllowedError{
		Pattern:  regexp.MustCompile(`Application not found`),
		Reason:   "intentional inspect of non-existent application",
		Required: true,
	})

	s.T().Log("Inspecting non-existent application (should fail with 404)...")
	_, err := inspectApplication(s.ctx, "nonexistent-app-12345", "hello")
	s.Require().Error(err, "inspect of non-existent app should fail")
	s.T().Logf("Got expected error: %v", err)
	s.T().Log("=== Inspect not-found test complete ===")
}

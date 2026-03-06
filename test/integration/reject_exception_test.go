// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/suite"
)

type RejectExceptionSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
}

func TestRejectException(t *testing.T) {
	suite.Run(t, new(RejectExceptionSuite))
}

func (s *RejectExceptionSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 10*time.Minute)
}

func (s *RejectExceptionSuite) TearDownSuite() {
	s.cancel()
}

// TestRejectInput deploys a reject-loop-dapp (ioctl-echo-loop --reject=1),
// sends 3 inputs, and verifies that input 1 is REJECTED while inputs 0 and 2
// are ACCEPTED with correct outputs and reports.
func (s *RejectExceptionSuite) TestRejectInput() {
	runRejectExceptionLifecycleTest(s.ctx, s.T(), s.Require(), rejectExceptionLifecycleConfig{
		AppName:    uniqueAppName("reject-loop"),
		DappPath:   envOrDefault("CARTESI_TEST_REJECT_DAPP_PATH", "applications/reject-loop-dapp"),
		TestName:   "reject",
		FailStatus: model.InputCompletionStatus_Rejected,
	})
}

// TestExceptionInput deploys an exception-loop-dapp (ioctl-echo-loop --exception=1),
// sends 3 inputs, and verifies that input 1 is EXCEPTION while inputs 0 and 2
// are ACCEPTED with correct outputs and reports.
func (s *RejectExceptionSuite) TestExceptionInput() {
	runRejectExceptionLifecycleTest(s.ctx, s.T(), s.Require(), rejectExceptionLifecycleConfig{
		AppName:    uniqueAppName("exception-loop"),
		DappPath:   envOrDefault("CARTESI_TEST_EXCEPTION_DAPP_PATH", "applications/exception-loop-dapp"),
		TestName:   "exception",
		FailStatus: model.InputCompletionStatus_Exception,
	})
}

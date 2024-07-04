// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package util

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	suiteTimeout = 120 * time.Second
)

type RetrySuite struct {
	suite.Suite
	simpleMock *SimpleMock
}

func TestRetrySuite(t *testing.T) {
	suite.Run(t, new(RetrySuite))
}

func (s *RetrySuite) SetupSuite() {
	s.simpleMock = &SimpleMock{}
}
func (s *RetrySuite) TearDownSuite() {}

func (s *RetrySuite) TestRetry() {

	s.simpleMock.On(
		"execute",
		mock.Anything).
		Once().
		Return(0, fmt.Errorf("An error"))

	s.simpleMock.On(
		"execute",
		mock.Anything).
		Once().
		Return(0, nil)

	CallFunctionWithRetryPolicy(s.simpleMock.execute, 0, 3, 1*time.Millisecond)

	s.simpleMock.AssertNumberOfCalls(s.T(), "execute", 2)
}

func (s *RetrySuite) TestRetryMaxRetries() {

	s.simpleMock.On(
		"execute",
		mock.Anything).
		Return(0, fmt.Errorf("An error"))

	CallFunctionWithRetryPolicy(s.simpleMock.execute, 0, 3, 1*time.Millisecond)

	s.simpleMock.AssertNumberOfCalls(s.T(), "execute", 4)
}

type SimpleMock struct {
	mock.Mock
}

func (m *SimpleMock) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
}

func (m *SimpleMock) execute(
	arg int,
) (int, error) {
	args := m.Called(arg)
	return args.Get(0).(int), args.Error(1)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retry

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type RetrySuite struct {
	suite.Suite
}

func TestRetrySuite(t *testing.T) {
	suite.Run(t, new(RetrySuite))
}

func (s *RetrySuite) SetupSuite()    {}
func (s *RetrySuite) TearDownSuite() {}

func (s *RetrySuite) TestRetry() {
	simpleMock := &SimpleMock{}

	simpleMock.On(
		"execute",
		mock.Anything).
		Once().
		Return(0, fmt.Errorf("An error"))

	simpleMock.On(
		"execute",
		mock.Anything).
		Return(0, nil)

	_, err := CallFunctionWithRetryPolicy(simpleMock.execute, 0, 3, 1*time.Millisecond, "TEST")
	s.Require().Nil(err)

	simpleMock.AssertNumberOfCalls(s.T(), "execute", 2)

}

func (s *RetrySuite) TestRetryMaxRetries() {

	simpleMock := &SimpleMock{}
	simpleMock.On(
		"execute",
		mock.Anything).
		Return(0, fmt.Errorf("An error"))

	_, err := CallFunctionWithRetryPolicy(simpleMock.execute, 0, 3, 1*time.Millisecond, "TEST")
	s.Require().NotNil(err)

	simpleMock.AssertNumberOfCalls(s.T(), "execute", 4)

}

type SimpleMock struct {
	mock.Mock
}

func (m *SimpleMock) execute(
	arg int,
) (int, error) {
	args := m.Called(arg)
	return args.Get(0).(int), args.Error(1)
}

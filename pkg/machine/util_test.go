// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const FAST_DEADLINE = 2 * time.Second //nolint:revive // Keep the established test helper name.

func TestUtil(t *testing.T) {
	suite.Run(t, new(UtilSuite))
}

type UtilSuite struct {
	suite.Suite
	logger  *slog.Logger
	tempDir string
}

func (s *UtilSuite) SetupSuite() {
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "machine_test_*")
	s.Require().NoError(err)
	s.tempDir = tempDir
}

func (s *UtilSuite) TearDownSuite() {
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
	}
}

// Test StartServer function
func (s *UtilSuite) TestStartServer() {
	require := s.Require()

	// Test with nil logger
	_, _, _, err := StartServer(nil, FAST_DEADLINE)
	require.Error(err)
	require.Contains(err.Error(), "logger must not be nil")

	// Test with valid logger
	remote, _, _, err := StartServer(s.logger, FAST_DEADLINE)
	require.Nil(err)
	err = remote.ShutdownServer(FAST_DEADLINE)
	require.Nil(err)
}

// Test StopServer function
func (s *UtilSuite) TestStopServer() {
	require := s.Require()

	// Test with nil logger
	err := StopServer(testMachineAddress, nil, FAST_DEADLINE)
	require.Error(err)
	require.Contains(err.Error(), "logger must not be nil")

	// Test with invalid address
	err = StopServer("invalid:address", s.logger, FAST_DEADLINE)
	require.Error(err)

	// Test with non-existent server
	err = StopServer("127.0.0.1:54321", s.logger, FAST_DEADLINE)
	require.Error(err)
}

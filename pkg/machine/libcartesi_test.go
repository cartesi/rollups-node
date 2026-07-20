// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"errors"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func TestLibCartesi(t *testing.T) {
	suite.Run(t, new(LibCartesiSuite))
}

type LibCartesiSuite struct {
	suite.Suite
	mockRemoteMachine *MockRemoteMachine
	backend           *LibCartesiBackend
}

func (s *LibCartesiSuite) SetupTest() {
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
}

func (s *LibCartesiSuite) TestLoad() {
	require := s.Require()

	// Test successful load
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("Load", "/test/dir", "config").Return(nil)

	err := s.backend.Load("/test/dir", "config", 5*time.Second)
	require.NoError(err)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test load with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	err = s.backend.Load("/test/dir", "config", 5*time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test load with load error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("Load", "/test/dir", "config").Return(errors.New("load error"))

	err = s.backend.Load("/test/dir", "config", 5*time.Second)
	require.Error(err)
	require.Contains(err.Error(), "load error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestRun() {
	require := s.Require()

	// Test successful run
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("Run", uint64(1000)).Return(emulator.BreakReason(YieldedManually), nil)

	breakReason, err := s.backend.Run(1000, 5*time.Second)
	require.NoError(err)
	require.Equal(YieldedManually, breakReason)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test run with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	_, err = s.backend.Run(1000, 5*time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test run with run error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("Run", uint64(1000)).Return(emulator.BreakReason(Failed), errors.New("run error"))

	_, err = s.backend.Run(1000, 5*time.Second)
	require.Error(err)
	require.Contains(err.Error(), "run error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestGetRootHash() {
	require := s.Require()

	expectedHash := randomFakeHash()

	// Test successful get root hash
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("GetRootHash").Return(expectedHash, nil)

	hash, err := s.backend.GetRootHash(5 * time.Second)
	require.NoError(err)
	require.Equal(expectedHash, hash)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	_, err = s.backend.GetRootHash(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with get hash error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("GetRootHash").Return(Hash{}, errors.New("hash error"))

	_, err = s.backend.GetRootHash(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "hash error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestIsAtManualYield() {
	require := s.Require()

	// Test manual yield true
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ReadReg", emulator.REG_IFLAGS_Y).Return(uint64(emulator.ManualYieldReasonAccepted), nil)

	isAtYield, err := s.backend.IsAtManualYield(5 * time.Second)
	require.NoError(err)
	require.True(isAtYield)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test manual yield false
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ReadReg", emulator.REG_IFLAGS_Y).Return(uint64(0), nil)

	isAtYield, err = s.backend.IsAtManualYield(5 * time.Second)
	require.NoError(err)
	require.False(isAtYield)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	_, err = s.backend.IsAtManualYield(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with read register error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ReadReg", emulator.REG_IFLAGS_Y).Return(uint64(0), errors.New("read error"))

	_, err = s.backend.IsAtManualYield(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "read error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestReadMCycle() {
	require := s.Require()

	// Test successful read
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ReadReg", emulator.REG_MCYCLE).Return(uint64(12345), nil)

	cycle, err := s.backend.ReadMCycle(5 * time.Second)
	require.NoError(err)
	require.Equal(uint64(12345), cycle)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	_, err = s.backend.ReadMCycle(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with read error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ReadReg", emulator.REG_MCYCLE).Return(uint64(0), errors.New("read error"))

	_, err = s.backend.ReadMCycle(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "read error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestSendCmioResponse() {
	require := s.Require()

	data := []byte("test data")
	expectedHash := randomFakeHash()

	// Test successful send
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("SendCmioResponse", uint16(1), data, &expectedHash).Return(nil)

	err := s.backend.SendCmioResponse(1, data, &expectedHash, 5*time.Second)
	require.NoError(err)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	err = s.backend.SendCmioResponse(1, data, &expectedHash, 5*time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with send error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("SendCmioResponse", uint16(1), data, &expectedHash).Return(errors.New("send error"))

	err = s.backend.SendCmioResponse(1, data, &expectedHash, 5*time.Second)
	require.Error(err)
	require.Contains(err.Error(), "send error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestReceiveCmioRequest() {
	require := s.Require()

	expectedData := []byte("response data")

	// Test successful receive
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ReceiveCmioRequest").Return(uint8(1), uint16(2), expectedData, nil)

	cmd, reason, data, err := s.backend.ReceiveCmioRequest(5 * time.Second)
	require.NoError(err)
	require.Equal(uint8(1), cmd)
	require.Equal(uint16(2), reason)
	require.Equal(expectedData, data)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	_, _, _, err = s.backend.ReceiveCmioRequest(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with receive error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ReceiveCmioRequest").Return(uint8(0), uint16(0), []byte(nil), errors.New("receive error"))

	_, _, _, err = s.backend.ReceiveCmioRequest(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "receive error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestStore() {
	require := s.Require()

	// Test successful store
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("Store", "/test/dir").Return(nil)

	err := s.backend.Store("/test/dir", 5*time.Second)
	require.NoError(err)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	err = s.backend.Store("/test/dir", 5*time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with store error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("Store", "/test/dir").Return(errors.New("store error"))

	err = s.backend.Store("/test/dir", 5*time.Second)
	require.Error(err)
	require.Contains(err.Error(), "store error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestDelete() {
	s.mockRemoteMachine.On("Delete").Return()

	s.backend.Delete()
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestForkServer() {
	require := s.Require()

	forkedMachine := new(emulator.RemoteMachine)

	// Test successful fork
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ForkServer").Return(forkedMachine, "127.0.0.1:54321", uint32(54321), nil)

	backend, address, pid, err := s.backend.ForkServer(5 * time.Second)
	require.NoError(err)
	require.NotNil(backend)
	require.Equal("127.0.0.1:54321", address)
	require.Equal(uint32(54321), pid)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	_, _, _, err = s.backend.ForkServer(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with fork error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ForkServer").Return((*emulator.RemoteMachine)(nil), "", uint32(0), errors.New("fork error"))

	_, _, _, err = s.backend.ForkServer(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "fork error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestShutdownServer() {
	require := s.Require()

	// Test successful shutdown
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ShutdownServer").Return(nil)

	err := s.backend.ShutdownServer(5 * time.Second)
	require.NoError(err)
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with timeout error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(errors.New("timeout error"))

	err = s.backend.ShutdownServer(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "failed to set operation timeout")
	s.mockRemoteMachine.AssertExpectations(s.T())

	// Test with shutdown error
	s.mockRemoteMachine = new(MockRemoteMachine)
	s.backend = &LibCartesiBackend{inner: s.mockRemoteMachine}
	s.mockRemoteMachine.On("SetTimeout", int64(5000)).Return(nil)
	s.mockRemoteMachine.On("ShutdownServer").Return(errors.New("shutdown error"))

	err = s.backend.ShutdownServer(5 * time.Second)
	require.Error(err)
	require.Contains(err.Error(), "shutdown error")
	s.mockRemoteMachine.AssertExpectations(s.T())
}

func (s *LibCartesiSuite) TestNewMachineRuntimeConfig() {
	require := s.Require()

	config, err := s.backend.NewMachineRuntimeConfig()
	require.NoError(err)
	require.Contains(config, "concurrency")
	require.Contains(config, "update_merkle_tree")

	// Verify it's valid JSON
	require.True(config[0] == '{' && config[len(config)-1] == '}')
}

func (s *LibCartesiSuite) TestCmioRxBufferSize() {
	require := s.Require()

	size := s.backend.CmioRxBufferSize()
	require.Equal(uint64(1<<emulator.CmioRxBufferLog2Size), size)
	require.Greater(size, uint64(0))
}

// MockRemoteMachine mocks the emulator.RemoteMachine
type MockRemoteMachine struct {
	mock.Mock
}

func (m *MockRemoteMachine) SetTimeout(timeoutMs int64) error {
	args := m.Called(timeoutMs)
	return args.Error(0)
}

func (m *MockRemoteMachine) Load(dir string, runtimeConfig string) error {
	args := m.Called(dir, runtimeConfig)
	return args.Error(0)
}

func (m *MockRemoteMachine) Run(mcycleEnd uint64) (emulator.BreakReason, error) {
	args := m.Called(mcycleEnd)
	return args.Get(0).(emulator.BreakReason), args.Error(1)
}

func (m *MockRemoteMachine) GetRootHash() (emulator.Hash, error) {
	args := m.Called()
	return args.Get(0).(Hash), args.Error(1)
}

func (m *MockRemoteMachine) GetProof(address uint64, log2size int32) (string, error) {
	args := m.Called(address, log2size)
	return args.Get(0).(string), args.Error(1)
}

func (m *MockRemoteMachine) ReadReg(reg emulator.RegID) (uint64, error) {
	args := m.Called(reg)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRemoteMachine) SendCmioResponse(reason uint16, data []byte, revertRootHash *Hash) error {
	args := m.Called(reason, data, revertRootHash)
	return args.Error(0)
}

func (m *MockRemoteMachine) ReceiveCmioRequest() (uint8, uint16, []byte, error) {
	args := m.Called()
	return args.Get(0).(uint8), args.Get(1).(uint16), args.Get(2).([]byte), args.Error(3)
}

func (m *MockRemoteMachine) Store(directory string) error {
	args := m.Called(directory)
	return args.Error(0)
}

func (m *MockRemoteMachine) WriteMemory(address uint64, data []byte) error {
	args := m.Called(address, data)
	return args.Error(0)
}

func (m *MockRemoteMachine) Delete() {
	m.Called()
}

func (m *MockRemoteMachine) ForkServer() (*emulator.RemoteMachine, string, uint32, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.String(1), args.Get(2).(uint32), args.Error(3)
	}
	return args.Get(0).(*emulator.RemoteMachine), args.String(1), args.Get(2).(uint32), args.Error(3)
}

func (m *MockRemoteMachine) ShutdownServer() error {
	args := m.Called()
	return args.Error(0)
}

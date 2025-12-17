// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"crypto/rand"
	"time"

	"github.com/stretchr/testify/mock"
)

// MockBackend is a testify mock implementation of the Backend interface
type MockBackend struct {
	mock.Mock
}

func (m *MockBackend) Load(dir string, runtimeConfig string, timeout time.Duration) error {
	args := m.Called(dir, runtimeConfig, timeout)
	return args.Error(0)
}

func (m *MockBackend) Store(directory string, timeout time.Duration) error {
	args := m.Called(directory, timeout)
	return args.Error(0)
}

func (m *MockBackend) Run(mcycleEnd uint64, timeout time.Duration) (BreakReason, error) {
	args := m.Called(mcycleEnd, timeout)
	return args.Get(0).(BreakReason), args.Error(1)
}

func (m *MockBackend) IsAtManualYield(timeout time.Duration) (bool, error) {
	args := m.Called(timeout)
	return args.Bool(0), args.Error(1)
}

func (m *MockBackend) ReadMCycle(timeout time.Duration) (uint64, error) {
	args := m.Called(timeout)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockBackend) SendCmioResponse(reason uint16, data []byte, timeout time.Duration) error {
	args := m.Called(reason, data, timeout)
	return args.Error(0)
}

func (m *MockBackend) ReceiveCmioRequest(timeout time.Duration) (uint8, uint16, []byte, error) {
	args := m.Called(timeout)
	return args.Get(0).(uint8), args.Get(1).(uint16), args.Get(2).([]byte), args.Error(3)
}

func (m *MockBackend) GetRootHash(timeout time.Duration) (Hash, error) {
	args := m.Called(timeout)
	return args.Get(0).(Hash), args.Error(1)
}

func (m *MockBackend) WriteMemory(address uint64, data []byte, timeout time.Duration) error {
	args := m.Called(address, data, timeout)
	return args.Error(0)
}

func (m *MockBackend) Delete() {
	m.Called()
}

func (m *MockBackend) ForkServer(timeout time.Duration) (Backend, string, uint32, error) {
	args := m.Called(timeout)
	return args.Get(0).(Backend), args.String(1), args.Get(2).(uint32), args.Error(3)
}

func (m *MockBackend) ShutdownServer(timeout time.Duration) error {
	args := m.Called(timeout)
	return args.Error(0)
}

func (m *MockBackend) NewMachineRuntimeConfig() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockBackend) CmioRxBufferSize() uint64 {
	args := m.Called()
	return args.Get(0).(uint64)
}

func (m *MockBackend) RunAndCollectRootHashes(mcycleEnd uint64, state *HashCollectorState, timeout time.Duration) (reason BreakReason, err error) {
	args := m.Called(mcycleEnd, state, timeout)
	return args.Get(0).(BreakReason), args.Error(1)
}

// Helper functions for setting up common mock scenarios

func randomFakeHash() Hash {
	hash := Hash{}
	_, _ = rand.Read(hash[:])
	return hash
}

// SetupAccepted configures the mock for a successful advance/inspect operation
func (m *MockBackend) SetupAccepted(reqType requestType) {
	hash := randomFakeHash()
	m.On("SendCmioResponse", uint16(reqType), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil)
	m.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	m.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	m.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), hash[:], nil)
	m.On("CmioRxBufferSize").Return(uint64(1024))
}

// SetupRejected configures the mock for a rejected advance/inspect operation
func (m *MockBackend) SetupRejected(reqType requestType) {
	hash := randomFakeHash()
	m.On("SendCmioResponse", uint16(reqType), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil)
	m.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	m.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	m.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonRejected), hash[:], nil)
	m.On("CmioRxBufferSize").Return(uint64(1024))
}

// SetupException configures the mock for an exception during advance/inspect
func (m *MockBackend) SetupException(reqType requestType) {
	m.On("SendCmioResponse", uint16(reqType), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil)
	m.On("Run", mock.AnythingOfType("uint64"), mock.AnythingOfType("time.Duration")).Return(YieldedManually, nil)
	m.On("ReadMCycle", mock.AnythingOfType("time.Duration")).Return(uint64(0), nil)
	m.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonException), []byte("exception data"), nil)
	m.On("CmioRxBufferSize").Return(uint64(1024))
}

// SetupForLoad configures the mock for successful machine loading
func (m *MockBackend) SetupForLoad() {
	hash := randomFakeHash()
	m.On("NewMachineRuntimeConfig").Return(`{"concurrency":{"update_merkle_tree":1}}`, nil).Once()
	m.On("Load", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(nil).Once()
	m.On("IsAtManualYield", mock.AnythingOfType("time.Duration")).Return(true, nil).Once()
	m.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonAccepted), hash[:], nil).Once()
}

// SetupForCleanup configures the mock for cleanup operations
func (m *MockBackend) SetupForCleanup() {
	m.On("ShutdownServer", mock.AnythingOfType("time.Duration")).Return(nil)
	m.On("Delete").Return()
}

// SetupRuntimeConfigError configures the mock to return an error when creating runtime config
func (m *MockBackend) SetupRuntimeConfigError(err error) {
	m.On("NewMachineRuntimeConfig").Return("", err)
}

// SetupLoadError configures the mock to return an error when loading
func (m *MockBackend) SetupLoadError(err error) {
	m.On("Load", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(err)
}

// SetupNotAtManualYield configures the mock to return false for IsAtManualYield
func (m *MockBackend) SetupNotAtManualYield() {
	m.On("IsAtManualYield", mock.AnythingOfType("time.Duration")).Return(false, nil)
}

// SetupForHash configures the mock for successful hash retrieval
func (m *MockBackend) SetupForHash(hash Hash) {
	m.On("GetRootHash", mock.AnythingOfType("time.Duration")).Return(hash, nil)
}

// NewMockBackend creates a new MockBackend with default setup
func NewMockBackend() *MockBackend {
	return &MockBackend{}
}

// MockBackendFactory creates a backend factory that returns the provided mock
func MockBackendFactory(backend *MockBackend) BackendFactory {
	return func(_ string, _ time.Duration) (Backend, string, uint32, error) {
		return backend, "127.0.0.1:12345", 12345, nil
	}
}

// FailingMockBackendFactory creates a backend factory that always fails
func FailingMockBackendFactory(err error) BackendFactory {
	return func(_ string, _ time.Duration) (Backend, string, uint32, error) {
		return nil, "", 0, err
	}
}

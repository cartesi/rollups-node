// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func TestMachine(t *testing.T) {
	suite.Run(t, new(MachineSuite))
}

type MachineSuite struct {
	suite.Suite
	logger *slog.Logger
}

func (s *MachineSuite) SetupSuite() {
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Test Load function with various scenarios
func (s *MachineSuite) TestLoad() {
	require := s.Require()
	ctx := context.Background()

	// Test with nil logger
	machine, err := Load(ctx, nil, DefaultConfig("testdata/nonexistent"))
	require.Error(err)
	require.Nil(machine)
	require.Contains(err.Error(), "logger must not be nil")

	// Test with nil MachineConfig
	machine, err = Load(ctx, s.logger, nil)
	require.Error(err)
	require.Nil(machine)
	require.Contains(err.Error(), "MachineConfig must not be nil")

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	machine, err = Load(canceledCtx, s.logger, DefaultConfig("testdata/nonexistent"))
	require.Error(err)
	require.Nil(machine)
	require.ErrorIs(err, ErrCanceled)

	// Test with timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	machine, err = Load(timeoutCtx, s.logger, DefaultConfig("testdata/nonexistent"))
	require.ErrorIs(err, ErrDeadlineExceeded)
	require.Nil(machine)

	// Test with failing backend factory
	config := DefaultConfig("some/path")
	config.BackendFactoryFn = FailingMockBackendFactory(errors.New("backend creation failed"))
	machine, err = Load(ctx, s.logger, config)
	require.Error(err)
	require.Nil(machine)
	require.ErrorIs(err, ErrMachineInternal)
	require.Contains(err.Error(), "backend creation failed")

	// Test with backend that fails to create runtime config
	mockBackend := NewMockBackend()
	mockBackend.SetupRuntimeConfigError(errors.New("runtime config failed"))
	mockBackend.SetupForCleanup()
	config = DefaultConfig("some/path")
	config.BackendFactoryFn = MockBackendFactory(mockBackend)
	machine, err = Load(ctx, s.logger, config)
	require.Error(err)
	require.Nil(machine)
	require.ErrorIs(err, ErrMachineInternal)
	require.Contains(err.Error(), "could not instantiate new machine runtime config")
	mockBackend.AssertExpectations(s.T())

	// Test with backend that fails to load machine
	mockBackend = NewMockBackend()
	mockBackend.On("NewMachineRuntimeConfig").Return(`{"concurrency":{"update_merkle_tree":1}}`, nil)
	mockBackend.SetupLoadError(errors.New("load failed"))
	mockBackend.SetupForCleanup()
	config = DefaultConfig("some/path")
	config.BackendFactoryFn = MockBackendFactory(mockBackend)
	machine, err = Load(ctx, s.logger, config)
	require.Error(err)
	require.Nil(machine)
	require.ErrorIs(err, ErrMachineInternal)
	require.Contains(err.Error(), "could not load the machine")
	mockBackend.AssertExpectations(s.T())

	// Test with machine not at manual yield
	mockBackend = NewMockBackend()
	mockBackend.On("NewMachineRuntimeConfig").Return(`{"concurrency":{"update_merkle_tree":1}}`, nil)
	mockBackend.On("Load",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
	).Return(nil).Once()
	mockBackend.SetupNotAtManualYield()
	mockBackend.SetupForCleanup()
	config = DefaultConfig("some/path")
	config.BackendFactoryFn = MockBackendFactory(mockBackend)
	machine, err = Load(ctx, s.logger, config)
	require.Error(err)
	require.Nil(machine)
	require.ErrorIs(err, ErrNotAtManualYield)
	mockBackend.AssertExpectations(s.T())

	// Test with machine last request not accepted
	mockBackend = NewMockBackend()
	mockBackend.On("NewMachineRuntimeConfig").Return(`{"concurrency":{"update_merkle_tree":1}}`, nil)
	mockBackend.On("Load",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
	).Return(nil).Once()
	mockBackend.On("IsAtManualYield", mock.AnythingOfType("time.Duration")).Return(true, nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(ManualYieldReasonRejected), make([]byte, HashSize), nil).Once()
	mockBackend.SetupForCleanup()
	config = DefaultConfig("some/path")
	config.BackendFactoryFn = MockBackendFactory(mockBackend)
	machine, err = Load(ctx, s.logger, config)
	require.Error(err)
	require.Nil(machine)
	require.ErrorIs(err, ErrRejected)
	mockBackend.AssertExpectations(s.T())

	// Test successful load
	mockBackend = NewMockBackend()
	mockBackend.SetupForLoad()
	mockBackend.SetupForCleanup()
	config = DefaultConfig("some/path")
	config.BackendFactoryFn = MockBackendFactory(mockBackend)
	machine, err = Load(ctx, s.logger, config)
	require.NoError(err)
	require.NotNil(machine)
	require.Equal("127.0.0.1:12345", machine.Address())

	// Clean up
	err = machine.Close()
	require.NoError(err)
	mockBackend.AssertExpectations(s.T())
}

// Test DefaultConfig function
func (s *MachineSuite) TestDefaultConfig() {
	require := s.Require()

	// Test default config creation
	config := DefaultConfig("some/path")
	require.NotNil(config)
	require.Equal("some/path", config.Path)
	require.Equal("127.0.0.1:0", config.Address)
	require.Nil(config.RuntimeConfig)
	require.NotNil(config.BackendFactoryFn)

	// Test execution parameters are set
	require.Greater(config.ExecutionParameters.AdvanceIncCycles, uint64(0))
	require.Greater(config.ExecutionParameters.AdvanceMaxCycles, uint64(0))
	require.Greater(config.ExecutionParameters.InspectIncCycles, uint64(0))
	require.Greater(config.ExecutionParameters.InspectMaxCycles, uint64(0))
	require.Greater(config.ExecutionParameters.AdvanceIncDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.AdvanceMaxDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.InspectIncDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.InspectMaxDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.LoadDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.StoreDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.FastDeadline, time.Duration(0))
}

// Test machine interface compliance
func (s *MachineSuite) TestMachineInterface() {
	require := s.Require()
	ctx := context.Background()

	// Create a mock machine
	mockMachine := &MockMachine{
		AddressReturn:         "127.0.0.1:12345",
		HashReturn:            Hash{1, 2, 3, 4, 5},
		OutputsHashReturn:     Hash{6, 7, 8, 9, 10},
		AdvanceAcceptedReturn: true,
		AdvanceOutputsReturn:  []Output{[]byte("output1"), []byte("output2")},
		AdvanceReportsReturn:  []Report{[]byte("report1")},
		AdvanceHashReturn:     Hash{11, 12, 13, 14, 15},
		InspectAcceptedReturn: true,
		InspectReportsReturn:  []Report{[]byte("inspect report")},
	}

	// Test that MockMachine implements Machine interface
	var machine Machine = mockMachine

	// Test Address
	address := machine.Address()
	require.Equal("127.0.0.1:12345", address)

	// Test Hash
	hash, err := machine.Hash(ctx)
	require.NoError(err)
	require.Equal(Hash{1, 2, 3, 4, 5}, hash)

	// Test OutputsHash
	outputsHash, err := machine.OutputsHash(ctx)
	require.NoError(err)
	require.Equal(Hash{6, 7, 8, 9, 10}, outputsHash)

	// Test Advance
	accepted, outputs, reports, _, _, advanceHash, err := machine.Advance(ctx, []byte("input"), false)
	require.NoError(err)
	require.True(accepted)
	require.Len(outputs, 2)
	require.Equal([]byte("output1"), outputs[0])
	require.Equal([]byte("output2"), outputs[1])
	require.Len(reports, 1)
	require.Equal([]byte("report1"), reports[0])
	require.Equal(Hash{11, 12, 13, 14, 15}, advanceHash)

	// Test Inspect
	accepted, inspectReports, err := machine.Inspect(ctx, []byte("query"))
	require.NoError(err)
	require.True(accepted)
	require.Len(inspectReports, 1)
	require.Equal([]byte("inspect report"), inspectReports[0])

	// Test Store
	err = machine.Store(ctx, "/tmp/test")
	require.NoError(err)

	// Test Close
	err = machine.Close()
	require.NoError(err)

	// Test Fork
	forkedMachine, err := machine.Fork(ctx)
	require.NoError(err)
	require.Nil(forkedMachine) // MockMachine returns nil by default
}

// Test machine interface with errors
func (s *MachineSuite) TestMachineInterfaceErrors() {
	require := s.Require()
	ctx := context.Background()

	// Create a mock machine that returns errors
	mockMachine := &MockMachine{
		ForkError:        errors.New("fork error"),
		HashError:        errors.New("hash error"),
		OutputsHashError: errors.New("outputs hash error"),
		AdvanceError:     errors.New("advance error"),
		InspectError:     errors.New("inspect error"),
		StoreError:       errors.New("store error"),
		CloseError:       errors.New("close error"),
	}

	var machine Machine = mockMachine

	// Test Fork error
	_, err := machine.Fork(ctx)
	require.Error(err)
	require.Contains(err.Error(), "fork error")

	// Test Hash error
	_, err = machine.Hash(ctx)
	require.Error(err)
	require.Contains(err.Error(), "hash error")

	// Test OutputsHash error
	_, err = machine.OutputsHash(ctx)
	require.Error(err)
	require.Contains(err.Error(), "outputs hash error")

	// Test Advance error
	_, _, _, _, _, _, err = machine.Advance(ctx, []byte("input"), false)
	require.Error(err)
	require.Contains(err.Error(), "advance error")

	// Test Inspect error
	_, _, err = machine.Inspect(ctx, []byte("query"))
	require.Error(err)
	require.Contains(err.Error(), "inspect error")

	// Test Store error
	err = machine.Store(ctx, "/tmp/test")
	require.Error(err)
	require.Contains(err.Error(), "store error")

	// Test Close error
	err = machine.Close()
	require.Error(err)
	require.Contains(err.Error(), "close error")
}

// MockMachine implements the Machine interface for testing
type MockMachine struct {
	ForkReturn Machine
	ForkError  error

	HashReturn Hash
	HashError  error

	OutputsHashReturn Hash
	OutputsHashError  error

	OutputsHashProofReturn []Hash
	OutputsHashProofError  error

	CheckpointHashError error

	AdvanceAcceptedReturn  bool
	AdvanceOutputsReturn   []Output
	AdvanceReportsReturn   []Report
	AdvanceHashesReturn    []Hash
	AdvanceRemainingReturn uint64
	AdvanceHashReturn      Hash
	AdvanceError           error

	InspectAcceptedReturn bool
	InspectReportsReturn  []Report
	InspectError          error

	StoreError error

	CloseError error

	AddressReturn string
}

func (m *MockMachine) Fork(_ context.Context) (Machine, error) {
	return m.ForkReturn, m.ForkError
}

func (m *MockMachine) Hash(_ context.Context) (Hash, error) {
	return m.HashReturn, m.HashError
}

func (m *MockMachine) OutputsHash(_ context.Context) (Hash, error) {
	return m.OutputsHashReturn, m.OutputsHashError
}

func (m *MockMachine) OutputsHashProof(_ context.Context) ([]Hash, error) {
	return m.OutputsHashProofReturn, m.OutputsHashProofError
}

func (m *MockMachine) WriteCheckpointHash(_ context.Context, _ Hash) error {
	return m.CheckpointHashError
}

func (m *MockMachine) Advance(_ context.Context, _ []byte, _ bool) (
	bool, []Output, []Report, []Hash, uint64, Hash, error,
) {
	return m.AdvanceAcceptedReturn,
		m.AdvanceOutputsReturn,
		m.AdvanceReportsReturn,
		m.AdvanceHashesReturn,
		m.AdvanceRemainingReturn,
		m.AdvanceHashReturn,
		m.AdvanceError
}

func (m *MockMachine) Inspect(_ context.Context,
	_ []byte,
) (bool, []Report, error) {
	return m.InspectAcceptedReturn, m.InspectReportsReturn, m.InspectError
}

func (m *MockMachine) Store(_ context.Context, _ string) error {
	return m.StoreError
}

func (m *MockMachine) Close() error {
	return m.CloseError
}

func (m *MockMachine) Address() string {
	return m.AddressReturn
}

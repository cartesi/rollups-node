// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/ethereum/go-ethereum/crypto"
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

	// Test with IsAtManualYield returning error
	mockBackend = NewMockBackend()
	mockBackend.On("NewMachineRuntimeConfig").Return(`{"concurrency":{"update_merkle_tree":1}}`, nil)
	mockBackend.On("Load",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
	).Return(nil).Once()
	mockBackend.On("IsAtManualYield", mock.AnythingOfType("time.Duration")).Return(false, errors.New("yield check failed"))
	mockBackend.SetupForCleanup()
	config = DefaultConfig("some/path")
	config.BackendFactoryFn = MockBackendFactory(mockBackend)
	machine, err = Load(ctx, s.logger, config)
	require.Error(err)
	require.Nil(machine)
	require.ErrorIs(err, ErrMachineInternal)
	require.Contains(err.Error(), "yield check failed")
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

	// Test with an unsupported manual yield reason.
	mockBackend = NewMockBackend()
	mockBackend.On("NewMachineRuntimeConfig").Return(`{"concurrency":{"update_merkle_tree":1}}`, nil)
	mockBackend.On("Load",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
	).Return(nil).Once()
	mockBackend.On("IsAtManualYield", mock.AnythingOfType("time.Duration")).Return(true, nil).Once()
	mockBackend.On("ReceiveCmioRequest", mock.AnythingOfType("time.Duration")).Return(
		uint8(0), uint16(9), make([]byte, HashSize), nil).Once()
	mockBackend.SetupForCleanup()
	config = DefaultConfig("some/path")
	config.BackendFactoryFn = MockBackendFactory(mockBackend)
	machine, err = Load(ctx, s.logger, config)
	require.Nil(machine)
	require.ErrorIs(err, ErrUnexpectedYield)
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
	require.Equal(testMachineAddress, machine.Address())

	// Clean up
	err = machine.Close()
	require.NoError(err)
	mockBackend.AssertExpectations(s.T())
}

// getProofYieldProgram writes the prepared manual yield request to HTIF
// tohost, leaving the machine at the accepted manual yield that Load requires.
var getProofYieldProgram = []uint32{
	0x400082b7, // lui t0,0x40008: load the HTIF base address
	0x0002b423, // sd zero,8(t0): clear fromhost
	0x0132b023, // sd x19,0(t0): write the prepared request to tohost
}

const (
	getProofRAMStart  = uint64(0x80000000) // RISC-V RAM base for the test guest
	getProofRAMLength = 4096
)

// TestGetProof proves a 32-byte memory span of a real emulator machine and
// verifies the proof against the machine root and the span's content.
func (s *MachineSuite) TestGetProof() {
	require := s.Require()
	ctx := context.Background()

	config, err := json.Marshal(map[string]any{
		"processor": map[string]any{"registers": map[string]any{"pc": getProofRAMStart}},
		"ram":       map[string]any{"length": getProofRAMLength},
		"cmio": map[string]any{
			"rx_buffer": map[string]any{},
			"tx_buffer": map[string]any{},
		},
	})
	require.NoError(err)

	emu, err := emulator.CreateMachine(string(config), "", "")
	require.NoError(err)
	defer emu.Delete()
	defer func() { _ = emu.Destroy() }()

	program := make([]byte, 4*len(getProofYieldProgram))
	for i, instruction := range getProofYieldProgram {
		binary.LittleEndian.PutUint32(program[4*i:], instruction)
	}
	require.NoError(emu.WriteMemory(getProofRAMStart, program))

	// The pre-set registers place the machine at the accepted manual yield
	// that Load requires, without running the guest.
	request := htifDeviceYield<<htifDeviceShift |
		uint64(emulator.YieldManual)<<htifCommandShift |
		uint64(emulator.ManualYieldReasonAccepted)<<htifReasonShift
	registers := []struct {
		id    emulator.RegID
		value uint64
	}{
		{emulator.REG_X19, request},
		{emulator.REG_IFLAGS_Y, 1},
		{emulator.REG_HTIF_TOHOST_DEV, htifDeviceYield},
		{emulator.REG_HTIF_TOHOST_CMD, uint64(emulator.YieldManual)},
		{emulator.REG_HTIF_TOHOST_REASON, uint64(emulator.ManualYieldReasonAccepted)},
		{emulator.REG_HTIF_TOHOST_DATA, 0},
	}
	for _, register := range registers {
		require.NoError(emu.WriteReg(register.id, register.value))
	}

	dataBlockAddress := iflagsYAddress &^ ((uint64(1) << HashLog2Size) - 1)
	dataBlock, err := emu.ReadMemory(dataBlockAddress, uint64(1)<<HashLog2Size)
	require.NoError(err)

	stored := filepath.Join(s.T().TempDir(), "stored")
	require.NoError(emu.Store(stored))

	machine, err := Load(ctx, s.logger, DefaultConfig(stored))
	require.NoError(err)
	defer func() { _ = machine.Close() }()

	root, err := machine.Hash(ctx)
	require.NoError(err)

	proof, err := machine.GetProof(ctx, dataBlockAddress, int32(HashLog2Size), machineMemoryLog2Size)
	require.NoError(err)
	require.Equal(dataBlockAddress, proof.TargetAddress)
	require.Equal(int32(HashLog2Size), proof.Log2TargetSize)
	require.Equal(machineMemoryLog2Size, proof.Log2RootSize)
	require.Len(proof.Siblings, memoryProofSiblingCount)
	require.Equal(root, proof.RootHash)
	require.Equal(Hash(crypto.Keccak256Hash(dataBlock)), proof.TargetHash)
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
	require.Zero(config.ExecutionParameters.AdvanceMaxCycles)
	require.Greater(config.ExecutionParameters.InspectIncCycles, uint64(0))
	require.Zero(config.ExecutionParameters.InspectMaxCycles)
	require.Greater(config.ExecutionParameters.AdvanceIncDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.AdvanceMaxDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.InspectIncDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.InspectMaxDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.LoadDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.StoreDeadline, time.Duration(0))
	require.Greater(config.ExecutionParameters.FastDeadline, time.Duration(0))
}

func (s *MachineSuite) TestConfiguredHardCycleCeilingMatchesMachineInputSpan() {
	s.Require().Equal(uint64(1)<<Log2MaxMCyclesPerAdvanceState, model.MaxExecutionCycles)
	// Cross-source check: the node's span must equal the constant the linked
	// emulator exposes for its machine-enforced imcyclemax window. This is the
	// same equality NewLibCartesiBackend refuses to start without.
	s.Require().Equal(emulator.Log2MaxMCyclesPerAdvanceState, model.Log2MaxExecutionCycles)
}

func (s *MachineSuite) TestExecutionLimitClassification() {
	for _, sentinel := range []error{
		ErrPayloadLengthLimitExceeded,
		ErrOutputsLimitExceeded,
		ErrReportsLimitExceeded,
		ErrReachedLimitMcycle,
		ErrMcycleOverflow,
	} {
		s.Require().True(IsExecutionLimitError(fmt.Errorf("context: %w", sentinel)))
	}
	s.Require().False(IsExecutionLimitError(ErrDeadlineExceeded))
}

func (s *MachineSuite) TestMachineOverflowSharesTheExecutionLimitUmbrella() {
	s.Require().ErrorIs(ErrMcycleOverflow, ErrReachedLimitMcycle)
}

func (s *MachineSuite) TestCompletionStatusIsCompleted() {
	for _, status := range []CompletionStatus{
		CompletionStatusAccepted,
		CompletionStatusRejected,
		CompletionStatusException,
		CompletionStatusHalted,
		CompletionStatusOverflow,
		CompletionStatusUnexpectedYield,
	} {
		s.Require().True(status.IsCompleted())
	}
	s.Require().False(CompletionStatusUnknown.IsCompleted())
	s.Require().False(CompletionStatus(255).IsCompleted())
}

// Test machine interface compliance
func (s *MachineSuite) TestMachineInterface() {
	require := s.Require()
	ctx := context.Background()

	// Create a mock machine
	mockMachine := &MockMachine{
		AddressReturn: testMachineAddress,
		HashReturn:    Hash{1, 2, 3, 4, 5},
		StateProofReturn: &StateProof{
			MachineHash: Hash{1, 2, 3, 4, 5},
		},
		CompletionStatusReturn: CompletionStatusAccepted,
		AdvanceOutputsReturn:   []Output{[]byte("output1"), []byte("output2")},
		AdvanceReportsReturn:   []Report{[]byte("report1")},
		InspectResponseReturn: &InspectResponse{
			Status:  CompletionStatusAccepted,
			Reports: []Report{[]byte("inspect report")},
		},
	}

	// Test that MockMachine implements Machine interface
	var machine Machine = mockMachine

	// Test Address
	address := machine.Address()
	require.Equal(testMachineAddress, address)

	// Test Hash
	hash, err := machine.Hash(ctx)
	require.NoError(err)
	require.Equal(Hash{1, 2, 3, 4, 5}, hash)

	// Test state proof
	acceptedState, err := machine.StateProof(ctx)
	require.NoError(err)
	require.Equal(Hash{1, 2, 3, 4, 5}, acceptedState.MachineHash)

	// Test Advance
	advanceResp, err := machine.Advance(ctx, []byte("input"), Hash{}, false)
	require.NoError(err)
	require.Equal(CompletionStatusAccepted, advanceResp.Status)
	require.Len(advanceResp.Outputs, 2)
	require.Equal([]byte("output1"), advanceResp.Outputs[0])
	require.Equal([]byte("output2"), advanceResp.Outputs[1])
	require.Len(advanceResp.Reports, 1)
	require.Equal([]byte("report1"), advanceResp.Reports[0])

	// Test Inspect
	inspectResponse, err := machine.Inspect(ctx, []byte("query"))
	require.NoError(err)
	require.Equal(CompletionStatusAccepted, inspectResponse.Status)
	require.Len(inspectResponse.Reports, 1)
	require.Equal([]byte("inspect report"), inspectResponse.Reports[0])

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
		ForkError:       errors.New("fork error"),
		HashError:       errors.New("hash error"),
		StateProofError: errors.New("state proof error"),
		AdvanceError:    errors.New("advance error"),
		InspectError:    errors.New("inspect error"),
		StoreError:      errors.New("store error"),
		CloseError:      errors.New("close error"),
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

	// Test state proof error
	_, err = machine.StateProof(ctx)
	require.Error(err)
	require.Contains(err.Error(), "state proof error")

	// Test Advance error
	_, err = machine.Advance(ctx, []byte("input"), Hash{}, false)
	require.Error(err)
	require.Contains(err.Error(), "advance error")

	// Test Inspect error
	_, err = machine.Inspect(ctx, []byte("query"))
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

	StateProofReturn *StateProof
	StateProofError  error

	GetProofReturn *MemoryProof
	GetProofError  error

	CompletionStatusReturn CompletionStatus
	AdvanceOutputsReturn   []Output
	AdvanceReportsReturn   []Report
	AdvanceHashesReturn    []Hash
	AdvanceRemainingReturn uint64
	AdvanceError           error

	InspectResponseReturn *InspectResponse
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

func (m *MockMachine) StateProof(_ context.Context) (*StateProof, error) {
	return m.StateProofReturn, m.StateProofError
}

func (m *MockMachine) GetProof(_ context.Context, _ uint64, _, _ int32) (*MemoryProof, error) {
	return m.GetProofReturn, m.GetProofError
}

func (m *MockMachine) Advance(_ context.Context, _ []byte, _ Hash, _ bool) (*AdvanceResponse, error) {
	if m.AdvanceError != nil {
		return nil, m.AdvanceError
	}
	return &AdvanceResponse{
		Status:              m.CompletionStatusReturn,
		Outputs:             m.AdvanceOutputsReturn,
		Reports:             m.AdvanceReportsReturn,
		PeriodicStateHashes: m.AdvanceHashesReturn,
		PaddingRepetitions:  m.AdvanceRemainingReturn,
	}, nil
}

func (m *MockMachine) Inspect(_ context.Context, _ []byte) (*InspectResponse, error) {
	return m.InspectResponseReturn, m.InspectError
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

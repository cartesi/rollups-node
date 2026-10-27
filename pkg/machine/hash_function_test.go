// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testKeccakHashFunction = "keccak256"
	testShaHashFunction    = "sha256"
)

func TestLibCartesiBackendHashFunction(t *testing.T) {
	const timeout = 2 * time.Second
	readErr := errors.New("initial configuration unavailable")
	timeoutErr := errors.New("timeout setting failed")
	for _, tc := range []struct {
		name       string
		config     string
		value      string
		readErr    error
		timeoutErr error
		decodeErr  bool
	}{
		{name: testKeccakHashFunction, config: `{"hash_tree":{"hash_function":"keccak256"}}`, value: testKeccakHashFunction},
		{
			name:   testShaHashFunction,
			config: `{"hash_tree":{"hash_function":"sha256"},"hash_function":"keccak256"}`,
			value:  testShaHashFunction,
		},
		{name: "missing field", config: `{}`},
		{name: "malformed JSON", config: `{`, decodeErr: true},
		{name: "wrong field type", config: `{"hash_tree":{"hash_function":1}}`, decodeErr: true},
		{name: "read error", readErr: readErr},
		{name: "timeout error", timeoutErr: timeoutErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := new(MockRemoteMachine)
			timeoutCall := remote.On("SetTimeout", timeout.Milliseconds()).Return(tc.timeoutErr).Once()
			if tc.timeoutErr == nil {
				remote.On("GetInitialConfig").Return(tc.config, tc.readErr).Once().NotBefore(timeoutCall)
			}
			backend := &LibCartesiBackend{inner: remote}
			value, err := backend.HashFunction(timeout)
			switch {
			case tc.timeoutErr != nil:
				require.ErrorIs(t, err, timeoutErr)
				remote.AssertNotCalled(t, "GetInitialConfig")
			case tc.readErr != nil:
				require.ErrorIs(t, err, readErr)
			case tc.decodeErr:
				require.ErrorContains(t, err, "decode initial configuration")
			default:
				require.NoError(t, err)
			}
			require.Equal(t, tc.value, value)
			remote.AssertExpectations(t)
		})
	}
}

func TestLoadChecksHashFunctionBeforeYield(t *testing.T) {
	readErr := errors.New("hash function read failed")
	shutdownErr := errors.New("shutdown failed")
	for _, tc := range []struct {
		name        string
		algorithm   string
		readErr     error
		shutdownErr error
	}{
		{name: testKeccakHashFunction, algorithm: testKeccakHashFunction},
		{name: testShaHashFunction, algorithm: testShaHashFunction},
		{name: "unknown", algorithm: "other"},
		{name: "missing"},
		{name: "read error", algorithm: testKeccakHashFunction, readErr: readErr},
		{name: "sha256 cleanup error", algorithm: testShaHashFunction, shutdownErr: shutdownErr},
		{name: "read and cleanup errors", readErr: readErr, shutdownErr: shutdownErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := NewMockBackend()
			config := DefaultConfig("machine-image")
			runtimeConfig := `{}`
			config.RuntimeConfig = &runtimeConfig
			config.BackendFactoryFn = func(_ string, _ time.Duration) (Backend, string, uint32, error) {
				return backend, testMachineAddress, 0, nil
			}
			loadCall := backend.On("Load", config.Path, runtimeConfig, config.ExecutionParameters.LoadDeadline).
				Return(nil).Once()
			hashCall := backend.On("HashFunction", config.ExecutionParameters.FastDeadline).
				Return(tc.algorithm, tc.readErr).Once().NotBefore(loadCall)
			// Optional later calls let an incorrect admission fail an ordinary assertion.
			yieldCall := backend.On("IsAtManualYield", config.ExecutionParameters.FastDeadline).
				Return(true, nil).Maybe().NotBefore(hashCall)
			backend.On("ReceiveCmioRequest", config.ExecutionParameters.FastDeadline).
				Return(uint8(0), uint16(ManualYieldReasonAccepted), make([]byte, HashSize), nil).
				Maybe().NotBefore(yieldCall)
			shutdownCall := backend.On("ShutdownServer", config.ExecutionParameters.FastDeadline).
				Return(tc.shutdownErr).Once().NotBefore(hashCall)
			backend.On("Delete").Return().Once().NotBefore(shutdownCall)

			loaded, err := Load(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), config)
			if loaded != nil {
				t.Cleanup(func() { require.NoError(t, loaded.Close()) })
			}
			switch {
			case tc.readErr != nil:
				require.Nil(t, loaded)
				require.ErrorIs(t, err, ErrMachineInternal)
				require.ErrorIs(t, err, readErr)
			case tc.algorithm != testKeccakHashFunction:
				require.Nil(t, loaded)
				require.ErrorIs(t, err, ErrUnsupportedHashFunction)
				require.ErrorContains(t, err, "machine hash function")
				require.ErrorContains(t, err, `"`+tc.algorithm+`"`)
				require.ErrorContains(t, err, "expected keccak256")
			default:
				require.NoError(t, err)
				require.NotNil(t, loaded)
				require.NoError(t, loaded.Close())
				backend.AssertNumberOfCalls(t, "IsAtManualYield", 1)
				backend.AssertNumberOfCalls(t, "ReceiveCmioRequest", 1)
			}
			if tc.shutdownErr != nil {
				require.ErrorIs(t, err, shutdownErr)
				require.ErrorIs(t, err, ErrOrphanServer)
			}
			if tc.readErr != nil || tc.algorithm != testKeccakHashFunction {
				backend.AssertNotCalled(t, "IsAtManualYield", mock.Anything)
				backend.AssertNotCalled(t, "ReceiveCmioRequest", mock.Anything)
			}
			backend.AssertNumberOfCalls(t, "HashFunction", 1)
			backend.AssertNumberOfCalls(t, "ShutdownServer", 1)
			backend.AssertNumberOfCalls(t, "Delete", 1)
			backend.AssertExpectations(t)
		})
	}
}

func TestLoadCancellationBeforeHashFunctionRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	backend := NewMockBackend()
	config := DefaultConfig("machine-image")
	runtimeConfig := `{}`
	config.RuntimeConfig = &runtimeConfig
	config.BackendFactoryFn = MockBackendFactory(backend)
	backend.On("Load", config.Path, runtimeConfig, config.ExecutionParameters.LoadDeadline).
		Run(func(mock.Arguments) { cancel() }).Return(nil).Once()
	backend.SetupForCleanup()
	loaded, err := Load(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), config)
	require.Nil(t, loaded)
	require.ErrorIs(t, err, ErrCanceled)
	require.NotErrorIs(t, err, ErrMachineInternal)
	backend.AssertNotCalled(t, "HashFunction", mock.Anything)
	backend.AssertNumberOfCalls(t, "ShutdownServer", 1)
	backend.AssertNumberOfCalls(t, "Delete", 1)
	backend.AssertExpectations(t)
}

type hashFunctionCleanupBackend struct {
	Backend
	reads     int
	algorithm string
	shutdowns int
	deletes   int
}

func (b *hashFunctionCleanupBackend) HashFunction(timeout time.Duration) (string, error) {
	b.reads++
	name, err := b.Backend.HashFunction(timeout)
	b.algorithm = name
	return name, err
}

func (b *hashFunctionCleanupBackend) ShutdownServer(timeout time.Duration) error {
	b.shutdowns++
	return b.Backend.ShutdownServer(timeout)
}

func (b *hashFunctionCleanupBackend) Delete() {
	b.deletes++
	b.Backend.Delete()
}

func TestLoadStoredMachineHashFunction(t *testing.T) {
	const deadline = 2 * time.Second
	for _, algorithm := range []string{testKeccakHashFunction, testShaHashFunction} {
		t.Run(algorithm, func(t *testing.T) {
			configJSON := fmt.Sprintf(`{
				"ram":{"length":4096},
				"cmio":{"rx_buffer":{},"tx_buffer":{}},
				"hash_tree":{"hash_function":%q}
			}`, algorithm)
			local, err := emulator.CreateMachine(configJSON, "", "")
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, local.Destroy())
				local.Delete()
			})
			for _, register := range []struct {
				id    emulator.RegID
				value uint64
			}{
				{emulator.REG_IFLAGS_Y, 1},
				{emulator.REG_HTIF_TOHOST_DEV, emulator.HtifDeviceYield},
				{emulator.REG_HTIF_TOHOST_CMD, uint64(emulator.YieldManual)},
				{emulator.REG_HTIF_TOHOST_REASON, uint64(emulator.ManualYieldReasonAccepted)},
				{emulator.REG_HTIF_TOHOST_DATA, HashSize},
			} {
				require.NoError(t, local.WriteReg(register.id, register.value))
			}
			_, err = local.GetRootHash()
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "machine")
			require.NoError(t, local.Store(path))
			config := DefaultConfig(path)
			config.ExecutionParameters.FastDeadline = deadline
			config.ExecutionParameters.LoadDeadline = deadline
			var backend *hashFunctionCleanupBackend
			config.BackendFactoryFn = func(address string, timeout time.Duration) (Backend, string, uint32, error) {
				inner, bound, pid, err := NewLibCartesiBackend(address, timeout)
				if err != nil {
					return nil, bound, pid, err
				}
				backend = &hashFunctionCleanupBackend{Backend: inner}
				return backend, bound, pid, nil
			}
			t.Cleanup(func() {
				if backend != nil && backend.deletes == 0 {
					require.NoError(t, backend.ShutdownServer(deadline))
					backend.Delete()
				}
			})
			loaded, err := Load(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), config)
			if algorithm == testShaHashFunction {
				require.Nil(t, loaded)
				require.ErrorIs(t, err, ErrUnsupportedHashFunction)
				require.ErrorContains(t, err, `"sha256"`)
			} else {
				require.NoError(t, err)
				require.NotNil(t, loaded)
				require.NoError(t, loaded.Close())
			}
			require.NotNil(t, backend)
			require.Equal(t, 1, backend.reads)
			require.Equal(t, algorithm, backend.algorithm)
			require.Equal(t, 1, backend.shutdowns)
			require.Equal(t, 1, backend.deletes)
		})
	}
}

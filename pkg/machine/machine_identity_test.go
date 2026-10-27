// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/stretchr/testify/require"
)

func TestLibCartesiBackendMachineIdentity(t *testing.T) {
	const (
		timeoutStage  = "timeout"
		marchStage    = "marchid"
		nodeStage     = "uarch"
		pristineStage = "server pristine"
	)
	readErr := errors.New("identity observation unavailable")
	for _, failed := range []string{"", timeoutStage, marchStage, nodeStage, pristineStage} {
		name := failed
		if name == "" {
			name = "success"
		}
		t.Run(name, func(t *testing.T) {
			remote := new(MockRemoteMachine)
			getError := func(stage string) error {
				if failed == stage {
					return readErr
				}
				return nil
			}
			timeoutCall := remote.On("SetTimeout", identityTimeout.Milliseconds()).Return(getError(timeoutStage)).Once()
			if failed != timeoutStage {
				marchCall := remote.On("ReadReg", emulator.REG_MARCHID).
					Return(testOtherMarchID, getError(marchStage)).Once().NotBefore(timeoutCall)
				if failed != marchStage {
					const uarchAddress = uint64(0x400000)
					const uarchLog2Size = int32(22)
					nodeCall := remote.On("GetNodeHash", uarchAddress, uarchLog2Size).
						Return(emulator.Hash{1}, getError(nodeStage)).Once().NotBefore(marchCall)
					if failed != nodeStage {
						remote.On("GetUarchPristineHash").Return(emulator.Hash{2}, getError(pristineStage)).
							Once().NotBefore(nodeCall)
					}
				}
			}
			backend := &LibCartesiBackend{inner: remote}
			identity, err := backend.MachineIdentity(identityTimeout)
			if failed == "" {
				require.NoError(t, err)
				require.Equal(t, Identity{
					MarchID: testOtherMarchID, UarchStateHash: Hash{1}, UarchPristineStateHash: Hash{2},
				}, identity)
			} else {
				require.ErrorIs(t, err, readErr)
				require.Zero(t, identity)
			}
			remote.AssertExpectations(t)
		})
	}
}

func TestLoadPrtMachineIdentity(t *testing.T) {
	pristine := Hash(model.UarchPristineStateHash())
	good := Identity{MarchID: testRequiredMarchID, UarchStateHash: pristine, UarchPristineStateHash: pristine}
	wrongMarch, wrongNode, wrongServer := good, good, good
	wrongMarch.MarchID = testOtherMarchID
	wrongNode.UarchStateHash = Hash{}
	wrongServer.UarchPristineStateHash = Hash{}
	readErr, closeErr := errors.New("identity unavailable"), errors.New("shutdown failed")
	for _, tc := range []struct {
		name              string
		identity          Identity
		readErr, closeErr error
		want              error
		subject           string
	}{
		{name: "compatible", identity: good},
		{name: "loaded marchid", identity: wrongMarch, want: model.ErrIncompatibleStateTransition, subject: "loaded machine marchid"},
		{name: "actual uarch", identity: wrongNode, want: model.ErrIncompatibleStateTransition, subject: "loaded machine uarch state"},
		{name: "server pristine", identity: wrongServer, want: model.ErrIncompatibleStateTransition, subject: "machine server"},
		{name: "read and close errors", identity: good, readErr: readErr, closeErr: closeErr, want: ErrMachineInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := NewMockBackend()
			config := DefaultConfig("identity-image")
			config.RequirePrtMachineIdentity = true
			runtimeConfig := `{}`
			config.RuntimeConfig = &runtimeConfig
			config.BackendFactoryFn = func(string, time.Duration) (Backend, string, uint32, error) {
				return backend, testMachineAddress, 0, nil
			}
			backend.On("Load", config.Path, runtimeConfig, config.ExecutionParameters.LoadDeadline).Return(nil).Once()
			backend.On("HashFunction", config.ExecutionParameters.FastDeadline).Return(testKeccakHashFunction, nil).Once()
			backend.On("MachineIdentity", config.ExecutionParameters.FastDeadline).Return(tc.identity, tc.readErr).Once()
			backend.On("IsAtManualYield", config.ExecutionParameters.FastDeadline).Return(true, nil).Maybe()
			backend.On("ReceiveCmioRequest", config.ExecutionParameters.FastDeadline).
				Return(uint8(0), uint16(ManualYieldReasonAccepted), make([]byte, HashSize), nil).Maybe()
			backend.On("ShutdownServer", config.ExecutionParameters.FastDeadline).Return(tc.closeErr).Once()
			backend.On("Delete").Return().Once()
			loaded, err := Load(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), config)
			if tc.want != nil {
				require.Nil(t, loaded)
				require.ErrorIs(t, err, tc.want)
				require.ErrorContains(t, err, tc.subject)
			} else {
				require.NoError(t, err)
				require.NoError(t, loaded.Close())
			}
			if tc.readErr != nil {
				require.ErrorIs(t, err, tc.readErr)
			}
			if tc.closeErr != nil {
				require.ErrorIs(t, err, tc.closeErr)
				require.ErrorIs(t, err, ErrOrphanServer)
			}
			backend.AssertExpectations(t)
		})
	}
}

type machineIdentityCountingBackend struct {
	*hashFunctionCleanupBackend
	identityReads int
	identity      Identity
}

func (b *machineIdentityCountingBackend) MachineIdentity(timeout time.Duration) (Identity, error) {
	b.identityReads++
	identity, err := b.Backend.MachineIdentity(timeout)
	b.identity = identity
	return identity, err
}

func TestLoadStoredPrtMachineIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		altered bool
		flag    bool
	}{
		{"fresh PRT", false, true},
		{"altered PRT", true, true},
		{"altered flag off", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local, err := emulator.CreateMachine(`{
				"ram":{"length":4096},
				"cmio":{"rx_buffer":{},"tx_buffer":{}}
			}`, "", "")
			require.NoError(t, err)
			t.Cleanup(func() {
				err := local.Destroy()
				local.Delete()
				require.NoError(t, err)
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
			if tc.altered {
				require.NoError(t, local.WriteReg(emulator.REG_UARCH_X1, 1))
			}
			_, err = local.GetRootHash()
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "machine")
			require.NoError(t, local.Store(path))
			config := DefaultConfig(path)
			config.RequirePrtMachineIdentity = tc.flag
			config.ExecutionParameters.FastDeadline = identityTimeout
			config.ExecutionParameters.LoadDeadline = identityTimeout
			var backend *machineIdentityCountingBackend
			config.BackendFactoryFn = func(address string, timeout time.Duration) (Backend, string, uint32, error) {
				inner, bound, pid, err := NewLibCartesiBackend(address, timeout)
				if err != nil {
					return nil, bound, pid, err
				}
				backend = &machineIdentityCountingBackend{hashFunctionCleanupBackend: &hashFunctionCleanupBackend{Backend: inner}}
				return backend, bound, pid, nil
			}
			t.Cleanup(func() {
				if backend != nil && backend.deletes == 0 {
					err := backend.ShutdownServer(identityTimeout)
					backend.Delete()
					require.NoError(t, err)
				}
			})
			loaded, err := Load(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), config)
			if loaded != nil {
				t.Cleanup(func() { require.NoError(t, loaded.Close()) })
			}
			if tc.altered && tc.flag {
				require.Nil(t, loaded)
				require.ErrorIs(t, err, model.ErrIncompatibleStateTransition)
				require.ErrorContains(t, err, "uarch state")
			} else {
				require.NoError(t, err)
				require.NotNil(t, loaded)
				require.NoError(t, loaded.Close())
			}
			require.NotNil(t, backend)
			if tc.flag {
				require.Equal(t, 1, backend.identityReads)
				require.Equal(t, testRequiredMarchID, backend.identity.MarchID)
				pristine := Hash(model.UarchPristineStateHash())
				require.Equal(t, pristine, backend.identity.UarchPristineStateHash)
				if tc.altered {
					require.NotEqual(t, pristine, backend.identity.UarchStateHash)
				} else {
					require.Equal(t, pristine, backend.identity.UarchStateHash)
				}
			} else {
				require.Zero(t, backend.identityReads)
			}
			require.Equal(t, 1, backend.shutdowns)
			require.Equal(t, 1, backend.deletes)
		})
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/emulator"
	pkgmachine "github.com/cartesi/rollups-node/pkg/machine"
	"github.com/stretchr/testify/require"
)

// Exercise the native machine wrapper too: the HTIF word read by Advance and
// the final proof must describe the same real emulator state.
func TestInvalidOutputsRootFixtureAdvance(t *testing.T) {
	for _, kind := range []string{rootKindValue, rootKindLength} {
		for _, collect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/collect=%t", kind, collect), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "machine")
				require.NoError(t, runInvalidOutputsRoot([]string{"--kind", kind, "--output", path}))
				ctx := context.Background()
				logger := slog.New(slog.NewTextHandler(io.Discard, nil))
				machine, err := pkgmachine.Load(ctx, logger, pkgmachine.DefaultConfig(path))
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, machine.Close()) })
				before, err := machine.Hash(ctx)
				require.NoError(t, err)
				response, err := machine.Advance(ctx, []byte("input"), before, collect)
				require.NoError(t, err)
				want := pkgmachine.CompletionStatusAccepted
				if kind == rootKindLength {
					want = pkgmachine.CompletionStatusInvalidOutputsRoot
				}
				require.Equal(t, want, response.Status)
				proof, err := machine.StateProof(ctx)
				require.NoError(t, err)
				require.NotEqual(t, before, proof.MachineHash)
				require.Equal(t, pkgmachine.Hash{}, proof.TxBufferProof.DataBlock)
				if collect {
					require.NoError(t, model.ValidateInputHashCollectionSpan(
						uint64(len(response.PeriodicStateHashes)), response.PaddingRepetitions))
				}
			})
		}
	}
}

func TestInvalidOutputsRootFixtures(t *testing.T) {
	for _, kind := range []string{rootKindValue, rootKindLength, rootKindTemplate} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "machine")
			require.NoError(t, runInvalidOutputsRoot([]string{"--kind", kind, "--output", path}))
			machine, _, _, err := emulator.SpawnServer("127.0.0.1:0", machineTimeout)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, machine.ShutdownServer())
				machine.Delete()
			})
			require.NoError(t, machine.Load(path, ""))
			command, reason, initialRoot, err := machine.ReceiveCmioRequest()
			require.NoError(t, err)
			require.Equal(t, uint8(emulator.YieldManual), command)
			require.Equal(t, uint16(emulator.ManualYieldReasonAccepted), reason)
			expectedInitialRoot := make([]byte, outputsRootSize)
			if kind != rootKindTemplate {
				root := merkle.CreatePostContext()[merkle.TREE_DEPTH]
				expectedInitialRoot = root[:]
			}
			require.Equal(t, expectedInitialRoot, initialRoot)

			for range 2 {
				require.NoError(t, sendAdvanceAndRun(&machine.Machine, emulator.BreakReasonYieldedManually))
				command, reason, root, err := machine.ReceiveCmioRequest()
				require.NoError(t, err)
				require.Equal(t, uint8(emulator.YieldManual), command)
				require.Equal(t, uint16(emulator.ManualYieldReasonAccepted), reason)
				expectedLength := outputsRootSize
				if kind == rootKindLength {
					expectedLength--
				}
				require.Equal(t, make([]byte, expectedLength), root)
			}
		})
	}
}

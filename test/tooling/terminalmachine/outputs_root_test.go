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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

const (
	outputsRootKindFlag   = "--kind"
	outputsRootOutputFlag = "--output"
)

// Exercise the native machine wrapper too: the HTIF word read by Advance and
// the final proof must describe the same real emulator state.
func TestInvalidOutputsRootFixtureAdvance(t *testing.T) {
	for _, kind := range []string{rootKindValue, rootKindLength} {
		for _, collect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/collect=%t", kind, collect), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "machine")
				require.NoError(t, runInvalidOutputsRoot([]string{outputsRootKindFlag, kind, outputsRootOutputFlag, path}))
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
					require.Equal(t, pkgmachine.Hash(model.FinalStateEntry(proof.MachineHash)), response.FinalStateEntry)
					require.NoError(t, model.ValidateInputHashCollectionSpan(
						uint64(len(response.PeriodicStateHashes)), response.PaddingRepetitions))
				}
			})
		}
	}
}

func TestInvalidOutputsRootFixtures(t *testing.T) {
	for _, kind := range []string{rootKindValue, rootKindLength, rootKindTemplate, rootKindRecovery} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "machine")
			require.NoError(t, runInvalidOutputsRoot([]string{outputsRootKindFlag, kind, outputsRootOutputFlag, path}))
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

			for input := range 3 {
				require.NoError(t, sendAdvanceAndRun(&machine.Machine, emulator.BreakReasonYieldedManually))
				command, reason, root, err := machine.ReceiveCmioRequest()
				require.NoError(t, err)
				require.Equal(t, uint8(emulator.YieldManual), command)
				require.Equal(t, uint16(emulator.ManualYieldReasonAccepted), reason)
				expectedLength := outputsRootSize
				if kind == rootKindLength {
					expectedLength--
				}
				expectedRoot := make([]byte, expectedLength)
				if kind == rootKindRecovery && input > 0 {
					empty := independentEmptyOutputsRoot()
					expectedRoot = empty[:]
				}
				require.Equal(t, expectedRoot, root)
			}
		})
	}
}

// The oracle folds empty leaves independently of the production post-context builder.
func independentEmptyOutputsRoot() common.Hash {
	root := common.Hash{}
	for range merkle.TREE_DEPTH {
		root = crypto.Keccak256Hash(root[:], root[:])
	}
	return root
}

func TestInvalidOutputsRootRecoveryFixtureAdvance(t *testing.T) {
	for _, collect := range []bool{false, true} {
		t.Run(fmt.Sprintf("collect=%t", collect), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "machine")
			require.NoError(t, runInvalidOutputsRoot([]string{outputsRootKindFlag, rootKindRecovery, outputsRootOutputFlag, path}))
			ctx := t.Context()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			machine, err := pkgmachine.Load(ctx, logger, pkgmachine.DefaultConfig(path))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, machine.Close()) })
			empty := pkgmachine.Hash(independentEmptyOutputsRoot())
			before, err := machine.Hash(ctx)
			require.NoError(t, err)
			for input, want := range []pkgmachine.Hash{{}, empty, empty} {
				response, err := machine.Advance(ctx, []byte(fmt.Sprintf("input-%d", input)), before, collect)
				require.NoError(t, err)
				require.Equal(t, pkgmachine.CompletionStatusAccepted, response.Status)
				proof, err := machine.StateProof(ctx)
				require.NoError(t, err)
				require.NotEqual(t, before, proof.MachineHash)
				require.Equal(t, want, proof.TxBufferProof.DataBlock)
				if collect {
					require.Equal(t, pkgmachine.Hash(model.FinalStateEntry(proof.MachineHash)), response.FinalStateEntry)
					require.NoError(t, model.ValidateInputHashCollectionSpan(
						uint64(len(response.PeriodicStateHashes)), response.PaddingRepetitions))
				}
				before = proof.MachineHash
			}
		})
	}
}

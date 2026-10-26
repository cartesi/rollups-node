// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestEntryBuilderMatchesProtocolLeaves(t *testing.T) {
	for _, height := range []uint32{0, 6} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			var rows []model.EpochStateHash
			var runs []merkletest.Run
			var final common.Hash
			for input, periodic := range []uint64{0, 1, 63, 64, 65, 127, 128, 191, 0} {
				rejected := input == 8
				if !rejected {
					final = crypto.Keccak256Hash([]byte(fmt.Sprintf("final-%d", input)))
				}
				nextRows, nextRuns := syntheticEntryInput(t, periodic, height, final, rejected)
				rows = append(rows, nextRows...)
				runs = append(runs, nextRuns...)
			}
			const inputCount = 9
			builder := newEntryBuilder(final, height)
			for _, row := range rows {
				builder.appendRow(row)
			}
			tree, err := builder.finish(inputCount)
			require.NoError(t, err)
			epochPadding := (model.MaxAdvanceStatesPerEpoch - inputCount) * (model.InputHashCollectionCapacity << height)
			runs = append(runs, merkletest.Run{Hash: final, Count: epochPadding})
			assertEntryTreeMatchesLeaves(t, tree, runs, 48+height, false)
		})
	}
}

// syntheticEntryInput constructs emulator output from protocol samples directly.
// It hashes each complete bundle and a partial bundle padded with the final hash.
// Bundled output also includes the emulator's final all-final-state entry.
func syntheticEntryInput(
	t *testing.T, periodic uint64, height uint32, final common.Hash, rejected bool,
) ([]model.EpochStateHash, []merkletest.Run) {
	t.Helper()
	samples := make([]common.Hash, periodic)
	runs := make([]merkletest.Run, 0, len(samples)+1)
	for i := range samples {
		samples[i] = crypto.Keccak256Hash([]byte(fmt.Sprintf("sample-%s-%d", final.Hex(), i)))
		runs = append(runs, merkletest.Run{Hash: samples[i], Count: 1})
	}
	width := uint64(1) << height
	var raw [][32]byte
	var consumed uint64
	for periodic-consumed >= width {
		raw = append(raw, denseEntryHash(samples[consumed:consumed+width]))
		consumed += width
	}
	finalRoot := final
	for range height {
		finalRoot = crypto.Keccak256Hash(finalRoot[:], finalRoot[:])
	}
	if height != 0 {
		tail := make([]common.Hash, width)
		for i := range tail {
			tail[i] = final
		}
		copy(tail, samples[consumed:])
		raw = append(raw, denseEntryHash(tail))
	}
	raw = append(raw, finalRoot)
	keptCount := (periodic + width - 1) / width
	if rejected {
		keptCount = 0
	}
	kept := raw[:keptCount]
	padding := model.InputHashCollectionCapacity - keptCount
	rows := make([]model.EpochStateHash, 0, len(kept)+1)
	for _, hash := range kept {
		rows = append(rows, model.EpochStateHash{MachineHash: hash, Repetitions: 1})
	}
	if padding > 0 {
		rows = append(rows, model.EpochStateHash{MachineHash: finalRoot, Repetitions: padding})
	}
	span := model.InputHashCollectionCapacity << height
	runs = append(runs, merkletest.Run{Hash: final, Count: span - periodic})
	return rows, runs
}

func denseEntryHash(leaves []common.Hash) common.Hash {
	level := append([]common.Hash(nil), leaves...)
	for len(level) > 1 {
		next := make([]common.Hash, len(level)/2)
		for i := range next {
			next[i] = crypto.Keccak256Hash(level[2*i][:], level[2*i+1][:])
		}
		level = next
	}
	return level[0]
}

func assertEntryTreeMatchesLeaves(
	t *testing.T, tree *merkle.Tree, runs []merkletest.Run, height uint32, opaqueLast bool,
) {
	t.Helper()
	require.Equal(t, height, tree.Height)
	require.Equal(t, merkletest.Root(runs, height), tree.RootHash)
	left, right := merkletest.RootChildren(runs, height)
	actualLeft, actualRight, err := tree.Subtrees.Children()
	require.NoError(t, err)
	require.Equal(t, left, actualLeft.RootHash)
	require.Equal(t, right, actualRight.RootHash)
	proof, err := tree.ProveLast()
	if opaqueLast {
		require.ErrorIs(t, err, merkle.ErrOpaqueTree)
		return
	}
	require.NoError(t, err)
	node, siblings := merkletest.LastProof(runs, height)
	require.Equal(t, node, proof.Node)
	require.Equal(t, siblings, proof.Siblings)
	expectedPosition := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(height)), big.NewInt(1))
	require.Equal(t, expectedPosition, proof.Pos)
	require.True(t, proof.VerifyRoot(tree.RootHash))
	proofLeft, proofRight, err := proof.BuildRootChildren()
	require.NoError(t, err)
	require.Equal(t, left, proofLeft)
	require.Equal(t, right, proofRight)
}

func TestEntryBuilderEmptyEpochUsesActualFinalHash(t *testing.T) {
	final := common.HexToHash("0x1234")
	for _, height := range []uint32{0, 6} {
		tree, err := newEntryBuilder(final, height).finish(0)
		require.NoError(t, err)
		leaves := uint64(1) << (48 + height)
		assertEntryTreeMatchesLeaves(t, tree, []merkletest.Run{{Hash: final, Count: leaves}}, 48+height, false)
	}
	require.Equal(t, model.FinalStateEntry(final), merkle.TreeLeaf(final).Iterated(model.Log2StateHashesPerEntry).RootHash)
}

func TestEntryBuilderFullSpanAndFullEpoch(t *testing.T) {
	const height = 6
	const width = uint64(1) << height
	a := common.HexToHash("0xaaaa")
	b := common.HexToHash("0xbbbb")
	final := common.HexToHash("0xcccc")
	finalNext := common.HexToHash("0xdddd")
	repeatedA := a
	repeatedB := b
	for range height {
		repeatedA = crypto.Keccak256Hash(repeatedA[:], repeatedA[:])
		repeatedB = crypto.Keccak256Hash(repeatedB[:], repeatedB[:])
	}
	tail := make([]common.Hash, width)
	for i := range tail {
		tail[i] = b
	}
	tail[len(tail)-1] = final
	fullSpanRows := []model.EpochStateHash{
		{MachineHash: repeatedB, Repetitions: model.InputHashCollectionCapacity - 1},
		{MachineHash: denseEntryHash(tail), Repetitions: 1},
	}
	span := model.InputHashCollectionCapacity * width
	fullSpanRuns := []merkletest.Run{{Hash: b, Count: span - 1}, {Hash: final, Count: 1}}
	for _, followed := range []bool{false, true} {
		name := "full span followed by epoch padding"
		if followed {
			name = "full span followed by another input"
		}
		t.Run(name, func(t *testing.T) {
			inputCount := uint64(1)
			rows := append([]model.EpochStateHash(nil), fullSpanRows...)
			runs := append([]merkletest.Run(nil), fullSpanRuns...)
			epochFinal := final
			if followed {
				inputCount++
				epochFinal = finalNext
				nextRows, nextRuns := syntheticEntryInput(t, 5, height, finalNext, false)
				rows = append(rows, nextRows...)
				runs = append(runs, nextRuns...)
			}
			builder := newEntryBuilder(epochFinal, height)
			for _, row := range rows {
				builder.appendRow(row)
			}
			tree, err := builder.finish(inputCount)
			require.NoError(t, err)
			runs = append(runs, merkletest.Run{Hash: epochFinal, Count: (model.MaxAdvanceStatesPerEpoch - inputCount) * span})
			assertEntryTreeMatchesLeaves(t, tree, runs, 54, false)
		})
	}
	for _, fullSpan := range []bool{false, true} {
		name := "last input has final-state padding"
		if fullSpan {
			name = "last input fills its span"
		}
		t.Run(name, func(t *testing.T) {
			builder := newEntryBuilder(final, height)
			prefixEntries := (model.MaxAdvanceStatesPerEpoch - 1) * model.InputHashCollectionCapacity
			builder.appendRow(model.EpochStateHash{MachineHash: repeatedA, Repetitions: prefixEntries})
			rows, lastRuns := fullSpanRows, fullSpanRuns
			if !fullSpan {
				rows, lastRuns = syntheticEntryInput(t, 5, height, final, false)
			}
			for _, row := range rows {
				builder.appendRow(row)
			}
			tree, err := builder.finish(model.MaxAdvanceStatesPerEpoch)
			require.NoError(t, err)
			runs := make([]merkletest.Run, 0, 1+len(lastRuns))
			runs = append(runs, merkletest.Run{Hash: a, Count: prefixEntries * width})
			runs = append(runs, lastRuns...)
			assertEntryTreeMatchesLeaves(t, tree, runs, 54, fullSpan)
		})
	}
}

func TestEntryBuilderKeepsFirstAppendError(t *testing.T) {
	builder := newEntryBuilder(common.Hash{}, 6)
	builder.appendRow(model.EpochStateHash{Repetitions: 0})
	first := builder.appendErr
	require.ErrorContains(t, first, "invalid repetitions")
	builder.appendRow(model.EpochStateHash{Repetitions: 1})
	_, err := builder.finish(1)
	require.ErrorIs(t, err, first)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

func TestInputHashCollectionDimensions(t *testing.T) {
	t.Parallel()
	require.Equal(t, uint64(24), Log2MaxAdvanceStatesPerEpoch)
	require.Equal(t, uint64(1)<<24, MaxAdvanceStatesPerEpoch)
	require.Equal(t, uint64(24), Log2InputHashCollectionCapacity)
	require.Equal(t, uint64(1)<<24, InputHashCollectionCapacity)
	require.Equal(t, uint64(54), Log2EpochComputationHashLeafCount)
	require.Equal(t, uint64(20), Log2MaxUarchCyclesPerMCycle)
	require.Equal(t, uint64(38), Log2RootTournamentStride)
	require.Equal(t, uint64(18), Log2MCycleComputationHashPeriod)
	require.Equal(t, uint64(24), Log2MCyclesPerStateHashEntry)
	require.Equal(t, uint64(6), Log2StateHashesPerEntry)
	require.Equal(t, uint64(30), Log2InputComputationHashLeafCount)
}

func TestValidateInputHashCollectionSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		hashes  uint64
		padding uint64
		wantErr bool
	}{
		{name: "empty", padding: InputHashCollectionCapacity},
		{name: "partial", hashes: 2, padding: InputHashCollectionCapacity - 2},
		{name: "full", hashes: InputHashCollectionCapacity},
		{name: "short", hashes: 2, padding: InputHashCollectionCapacity - 3, wantErr: true},
		{name: "long", hashes: 2, padding: InputHashCollectionCapacity - 1, wantErr: true},
		{name: "overflow", hashes: InputHashCollectionCapacity + 1, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateInputHashCollectionSpan(test.hashes, test.padding)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// denseEntryRoot builds every level from explicit leaves with a separate Keccak implementation.
func denseEntryRoot(leaves [][32]byte) [32]byte {
	level := append([][32]byte(nil), leaves...)
	for len(level) > 1 {
		next := make([][32]byte, len(level)/2)
		for index := range next {
			hasher := sha3.NewLegacyKeccak256()
			_, _ = hasher.Write(level[2*index][:])
			_, _ = hasher.Write(level[2*index+1][:])
			hasher.Sum(next[index][:0])
		}
		level = next
	}
	return level[0]
}

func TestRepeatedStateHashRootMatchesDenseTree(t *testing.T) {
	hash := common.HexToHash("0x123456789abcdef")
	require.Equal(t, hash, RepeatedStateHashRoot(hash, 0))
	leaves := make([][32]byte, 64)
	for index := range leaves {
		leaves[index] = hash
	}
	require.Equal(t, common.Hash(denseEntryRoot(leaves)), RepeatedStateHashRoot(hash, 6))
	require.Equal(t, RepeatedStateHashRoot(hash, Log2StateHashesPerEntry), FinalStateEntry(hash))
}

// denseTerminalEntries follows sample pushes, bundle completion, and final padding.
func denseTerminalEntries(periodic [][32]byte, final [32]byte, width int) (raw, kept [][32]byte) {
	leaves := append(append([][32]byte(nil), periodic...), final)
	for start := 0; start < len(leaves); start += width {
		bundle := make([][32]byte, width)
		for index := range bundle {
			bundle[index] = final
			if start+index < len(leaves) {
				bundle[index] = leaves[start+index]
			}
		}
		entry := denseEntryRoot(bundle)
		raw = append(raw, entry)
		if start < len(periodic) {
			kept = append(kept, entry)
		}
	}
	if width > 1 {
		padding := make([][32]byte, width)
		for index := range padding {
			padding[index] = final
		}
		raw = append(raw, denseEntryRoot(padding))
	}
	return raw, kept
}

func TestCanonicalInputHashCollectionMatchesDenseSamples(t *testing.T) {
	final := [32]byte{0xff}
	for _, geometry := range [][2]uint64{{18, 6}} {
		period, width := uint64(1)<<geometry[0], 1<<geometry[1]
		sampleCounts := []uint64{0, 1, 63, 64, 65, 127, 128, 191}
		deltas := make([]uint64, 1, 1+3*len(sampleCounts))
		for _, samples := range sampleCounts {
			boundary := (samples + 1) * period
			deltas = append(deltas, boundary-1, boundary, boundary+1)
		}
		for _, delta := range deltas {
			t.Run(fmt.Sprintf("period=%d/k=%d/delta=%d", geometry[0], geometry[1], delta), func(t *testing.T) {
				var periodic [][32]byte
				for cycle := period; cycle < delta; cycle += period {
					var leaf [32]byte
					binary.BigEndian.PutUint64(leaf[:8], cycle)
					periodic = append(periodic, leaf)
				}
				raw, wantKept := denseTerminalEntries(periodic, final, width)
				before := append([][32]byte(nil), raw...)
				kept, padding, gotFinal, err := CanonicalInputHashCollection(raw, delta)
				require.NoError(t, err)
				require.Equal(t, wantKept, append([][32]byte(nil), kept...))
				require.Equal(t, InputHashCollectionCapacity-uint64(len(wantKept)), padding)
				require.Equal(t, raw[len(raw)-1], gotFinal)
				require.Equal(t, before, raw)
			})
		}
	}
}

func TestCanonicalInputHashCollectionFoldsFinalOnlyEntries(t *testing.T) {
	final, sampled := [32]byte{0xff}, [32]byte{0x11}
	const period = uint64(1) << 18
	for _, test := range []struct {
		name  string
		delta uint64
		raw   [][32]byte
		kept  [][32]byte
	}{
		{name: "immediate fixed point", raw: [][32]byte{final, final}},
		{name: "full periodic bundle", delta: 64*period + 1,
			raw: [][32]byte{sampled, final, final}, kept: [][32]byte{sampled}},
		{name: "periodic entry equals final", delta: period + 1,
			raw: [][32]byte{final, final}, kept: [][32]byte{final}},
	} {
		t.Run(test.name, func(t *testing.T) {
			kept, padding, gotFinal, err := CanonicalInputHashCollection(test.raw, test.delta)
			require.NoError(t, err)
			require.Equal(t, test.kept, append([][32]byte(nil), kept...))
			require.Equal(t, InputHashCollectionCapacity-uint64(len(test.kept)), padding)
			require.Equal(t, final, gotFinal)
		})
	}
	_, _, _, err := CanonicalInputHashCollection([][32]byte{sampled, final}, 0)
	require.ErrorContains(t, err, "discarded state hash entry differs")
	_, _, _, err = CanonicalInputHashCollection([][32]byte{sampled, sampled, final}, 64*period+1)
	require.ErrorContains(t, err, "discarded state hash entry differs")
	for _, raw := range [][][32]byte{nil, {final}, {final, final, final}} {
		_, _, _, err := CanonicalInputHashCollection(raw, 0)
		require.ErrorContains(t, err, "state hash entries, expected 2")
	}
}

func TestCanonicalInputHashCollectionWindowCounts(t *testing.T) {
	const totalSamples = uint64(1) << 30
	for _, test := range []struct {
		removed, padding, extra uint64
	}{
		{128, 2, 0}, {127, 1, 0}, {65, 1, 0}, {64, 1, 1}, {63, 0, 1}, {1, 0, 1},
	} {
		t.Run(fmt.Sprintf("samples=%d", totalSamples-test.removed), func(t *testing.T) {
			delta := ((totalSamples - test.removed) << 18) + 1
			raw, kept, padding, err := canonicalInputHashCollectionCounts(delta)
			require.NoError(t, err)
			require.Equal(t, InputHashCollectionCapacity+test.extra, raw)
			require.Equal(t, InputHashCollectionCapacity-test.padding, kept)
			require.Equal(t, test.padding, padding)
		})
	}
	raw, kept, padding, err := canonicalInputHashCollectionCounts(MaxExecutionCycleSpan)
	require.NoError(t, err)
	require.Equal(t, InputHashCollectionCapacity+1, raw)
	require.Equal(t, InputHashCollectionCapacity, kept)
	require.Zero(t, padding)
}

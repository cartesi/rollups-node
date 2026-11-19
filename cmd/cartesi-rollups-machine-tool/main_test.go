// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestReplayCommandExecuteRejectedFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "default", want: false},
		{name: "explicit false", args: []string{"--execute-rejected=false"}, want: false},
		{name: "enabled", args: []string{"--execute-rejected"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got replayOptions
			cmd := newReplayCommandWithRunner(
				func(_ context.Context, _ *cobra.Command, opts replayOptions) error {
					got = opts
					return nil
				},
			)
			cmd.SetArgs(append([]string{
				"--template", "template",
				"--application", "app",
				"--store", "store",
				"--to-input-index", "0",
			}, tt.args...))

			require.NoError(t, cmd.ExecuteContext(t.Context()))
			require.Equal(t, tt.want, got.ExecuteRejected)
		})
	}
}

func TestVerifyAccountsDriveMachineProof(t *testing.T) {
	t.Parallel()

	const (
		log2DriveSize int32  = 9
		driveStart    uint64 = 0x1200
	)
	driveRoot := crypto.Keccak256Hash([]byte("accounts drive"))
	siblings := make([]machine.Hash, int(machineMemoryLog2Size-log2DriveSize))
	for i := range siblings {
		siblings[i] = machine.Hash(crypto.Keccak256Hash([]byte{byte(i)}))
	}

	machineRoot := machine.Hash(driveRoot)
	index := driveStart >> uint64(log2DriveSize)
	for _, sibling := range siblings {
		if index&1 == 0 {
			machineRoot = machine.Hash(crypto.Keccak256Hash(machineRoot[:], sibling[:]))
		} else {
			machineRoot = machine.Hash(crypto.Keccak256Hash(sibling[:], machineRoot[:]))
		}
		index >>= 1
	}

	validProof := func() *machine.MemoryProof {
		return &machine.MemoryProof{
			Log2RootSize:   machineMemoryLog2Size,
			Log2TargetSize: log2DriveSize,
			TargetAddress:  driveStart,
			TargetHash:     machine.Hash(driveRoot),
			Siblings:       append([]machine.Hash(nil), siblings...),
			RootHash:       machineRoot,
		}
	}

	t.Run("valid proof", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, verifyAccountsDriveMachineProof(
			validProof(), driveStart, log2DriveSize, machineRoot, driveRoot,
		))
	})

	t.Run("nil proof", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, verifyAccountsDriveMachineProof(
			nil, driveStart, log2DriveSize, machineRoot, driveRoot,
		), "no accounts-drive proof")
	})

	tests := []struct {
		name   string
		mutate func(*machine.MemoryProof)
		want   string
	}{
		{
			name:   "wrong root log2 size",
			mutate: func(proof *machine.MemoryProof) { proof.Log2RootSize-- },
			want:   "root log2 size",
		},
		{
			name:   "wrong target log2 size",
			mutate: func(proof *machine.MemoryProof) { proof.Log2TargetSize++ },
			want:   "target log2 size",
		},
		{
			name:   "wrong target address",
			mutate: func(proof *machine.MemoryProof) { proof.TargetAddress++ },
			want:   "target address",
		},
		{
			name: "missing sibling",
			mutate: func(proof *machine.MemoryProof) {
				proof.Siblings = proof.Siblings[:len(proof.Siblings)-1]
			},
			want: "siblings",
		},
		{
			name:   "wrong target hash",
			mutate: func(proof *machine.MemoryProof) { proof.TargetHash[0] ^= 0xff },
			want:   "accounts-drive root mismatch",
		},
		{
			name:   "wrong root hash",
			mutate: func(proof *machine.MemoryProof) { proof.RootHash[0] ^= 0xff },
			want:   "machine proof root mismatch",
		},
		{
			name:   "tampered sibling",
			mutate: func(proof *machine.MemoryProof) { proof.Siblings[0][0] ^= 0xff },
			want:   "siblings do not reconstruct",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			proof := validProof()
			tt.mutate(proof)
			require.ErrorContains(t, verifyAccountsDriveMachineProof(
				proof, driveStart, log2DriveSize, machineRoot, driveRoot,
			), tt.want)
		})
	}
}

func TestFindStoredDrive_MemoryRanges_ReturnsBackingFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config string
	}{
		{
			name: "flash drive without nvram field",
			config: `{"config":{"flash_drive":[
				{"start":65536,"length":8192,"backing_store":{"data_filename":"./accounts.bin"}}
			]}}`,
		},
		{
			name: "nvram without flash drive field",
			config: `{"config":{"nvram":[
				{"start":65536,"length":8192,"backing_store":{"data_filename":"./accounts.bin"}}
			]}}`,
		},
		{
			name: "nvram with empty flash drive list",
			config: `{"config":{"flash_drive":[],"nvram":[
				{"start":65536,"length":8192,"backing_store":{"data_filename":"accounts.bin"}}
			]}}`,
		},
		{
			name: "nvram among unrelated flash drives and nvrams",
			config: `{"config":{"flash_drive":[
				{"start":131072,"length":8192,"backing_store":{"data_filename":"./root.bin"}}
			],"nvram":[
				{"start":262144,"length":8192,"backing_store":{"data_filename":"./other.bin"}},
				{"start":65536,"length":8192,"backing_store":{"data_filename":"./accounts.bin"}}
			]}}`,
		},
		{
			name: "flash drive in mixed configuration",
			config: `{"config":{"flash_drive":[
				{"start":65536,"length":8192,"backing_store":{"data_filename":"./accounts.bin"}}
			],"nvram":[
				{"start":131072,"length":8192,"backing_store":{"data_filename":"./other.bin"}}
			]}}`,
		},
		{
			name: "nvram larger than requested range",
			config: `{"config":{"nvram":[
				{"start":65536,"length":16384,"backing_store":{"data_filename":"./accounts.bin"}}
			]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			snapshot := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(snapshot, "config.json"), []byte(tt.config), 0600))

			path, err := findStoredDrive(snapshot, 65536, 8192)

			require.NoError(t, err)
			require.Equal(t, filepath.Join(snapshot, "accounts.bin"), path)
		})
	}
}

func TestFindStoredDrive_NoMatchingRange_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config string
	}{
		{
			name:   "missing memory ranges",
			config: `{"config":{}}`,
		},
		{
			name:   "empty memory ranges",
			config: `{"config":{"flash_drive":[],"nvram":[]}}`,
		},
		{
			name: "flash drive at wrong address",
			config: `{"config":{"flash_drive":[
				{"start":131072,"length":8192,"backing_store":{"data_filename":"./accounts.bin"}}
			]}}`,
		},
		{
			name: "nvram at wrong address",
			config: `{"config":{"nvram":[
				{"start":131072,"length":8192,"backing_store":{"data_filename":"./accounts.bin"}}
			]}}`,
		},
		{
			name: "undersized flash drive",
			config: `{"config":{"flash_drive":[
				{"start":65536,"length":4096,"backing_store":{"data_filename":"./accounts.bin"}}
			]}}`,
		},
		{
			name: "undersized nvram",
			config: `{"config":{"nvram":[
				{"start":65536,"length":4096,"backing_store":{"data_filename":"./accounts.bin"}}
			]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			snapshot := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(snapshot, "config.json"), []byte(tt.config), 0600))

			path, err := findStoredDrive(snapshot, 65536, 8192)

			require.ErrorContains(t, err, "accounts drive not found in stored machine: start=0x10000 length=0x2000")
			require.Empty(t, path)
		})
	}
}

func TestFindStoredDrive_MissingConfig_ReturnsReadError(t *testing.T) {
	t.Parallel()

	path, err := findStoredDrive(t.TempDir(), 65536, 8192)

	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "read stored machine config")
	require.Empty(t, path)
}

func TestFindStoredDrive_InvalidConfig_ReturnsParseError(t *testing.T) {
	t.Parallel()

	snapshot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(snapshot, "config.json"), []byte(`{"config":`), 0600))

	path, err := findStoredDrive(snapshot, 65536, 8192)

	require.ErrorContains(t, err, "parse stored machine config")
	require.Empty(t, path)
}

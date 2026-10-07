// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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

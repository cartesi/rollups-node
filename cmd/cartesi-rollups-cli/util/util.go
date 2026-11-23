// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
)

const (
	machineArchiveVersion uint64 = 9           // archive format of emulator 0.23
	storedRootHashOffset         = int64(0x5c) // root hash offset in hash_tree.sht
)

// ResolveApplicationAddress returns the IApplication address corresponding
// to a name-or-address CLI argument.
//
//   - If the input is a 0x-prefixed string, it is treated as an Ethereum
//     address and returned directly. No DB connection is made. This lets the
//     CLI operate against an application that is NOT registered in any local
//     repository (remote use, ad-hoc inspection, foreclosure flow on a
//     reader-only host).
//   - Otherwise the input is treated as an application name and looked up
//     in the local repository. A DB connection is required and an error is
//     returned if the application is not found.
func ResolveApplicationAddress(ctx context.Context, nameOrAddress string) (common.Address, error) {
	if strings.HasPrefix(nameOrAddress, "0x") || strings.HasPrefix(nameOrAddress, "0X") {
		if !common.IsHexAddress(nameOrAddress) {
			return common.Address{}, fmt.Errorf("invalid Ethereum address %q", nameOrAddress)
		}
		return common.HexToAddress(nameOrAddress), nil
	}
	dsn, err := config.GetDatabaseConnection()
	if err != nil {
		return common.Address{}, fmt.Errorf(
			"resolving application %q by name requires the database; pass the application address (0x…) "+
				"instead to skip the local repository: %w", nameOrAddress, err)
	}
	repo, err := cli.OpenRepository(ctx, dsn)
	if err != nil {
		return common.Address{}, err
	}
	defer repo.Close()
	app, err := repo.GetApplication(ctx, nameOrAddress)
	if err != nil {
		return common.Address{}, err
	}
	if app == nil {
		return common.Address{}, fmt.Errorf("application %q not found in the database", nameOrAddress)
	}
	return app.IApplicationAddress, nil
}

// ReadArchiveVersion reads the archive version from the stored machine's
// config.json.
func ReadArchiveVersion(machineDir string) (uint64, error) {
	configPath := path.Join(machineDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0, fmt.Errorf("read machine config: %w", err)
	}

	var config struct {
		ArchiveVersion uint64 `json:"archive_version"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return 0, fmt.Errorf("decode machine config: %w", err)
	}
	return config.ArchiveVersion, nil
}

// Reads the Cartesi Machine hash from machineDir. Returns it as a commonHash
// or an error
func ReadRootHash(machineDir string) (common.Hash, error) {
	zero := common.Hash{}

	// assert compatible archive version before reading root hash
	archiveVersion, err := ReadArchiveVersion(machineDir)
	if err != nil {
		return zero, err
	}
	if archiveVersion != machineArchiveVersion {
		return zero, fmt.Errorf("unsupported machine archive version %d; expected %d",
			archiveVersion, machineArchiveVersion)
	}

	path := path.Join(machineDir, "hash_tree.sht")
	f, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer f.Close()

	_, err = f.Seek(storedRootHashOffset, io.SeekStart)
	if err != nil {
		return zero, err
	}

	hash := common.Hash{}
	_, err = io.ReadFull(f, hash[:])
	if err != nil {
		return zero, err
	}
	return hash, nil
}

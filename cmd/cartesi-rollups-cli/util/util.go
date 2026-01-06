// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package util

import (
	"io"
	"os"
	"path"

	"github.com/ethereum/go-ethereum/common"
)

// Reads the Cartesi Machine hash from machineDir. Returns it as a commonHash
// or an error
func ReadRootHash(machineDir string) (common.Hash, error) {
	zero := common.Hash{}
	path := path.Join(machineDir, "hash_tree.sht")
	f, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer f.Close()

	// root hash is located at this offset (0x60). Double check its value
	// with the cartesi-machine-stored-hash tool.
	_, err = f.Seek(0x60, io.SeekStart)
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

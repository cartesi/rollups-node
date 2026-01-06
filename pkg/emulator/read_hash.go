// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/ethereum/go-ethereum/common"
)

// Reads the Cartesi Machine hash from machineDir. Returns it as a hex string or
// an error
func ReadHashHex(machineDir string) (string, error) {
	path := path.Join(machineDir, "hash_tree.sht")
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// root hash is located at this offset (0x60). Double check its value
	// with the cartesi-machine-stored-hash tool.
	_, err = f.Seek(0x60, io.SeekStart)
	if err != nil {
		return "", err
	}

	// read only 0x20 bytes from it, there are more hash values after it
	rawHash := make([]byte, 0x20)
	n, err := f.Read(rawHash)
	if err != nil {
		return "", err
	}
	if n != common.HashLength {
		return "", fmt.Errorf(
			"read hash: wrong size; expected %v bytes but read %v",
			common.HashLength,
			n,
		)
	}
	return common.Bytes2Hex(rawHash), nil
}

// Reads the Cartesi Machine hash from machineDir. Returns it as a commonHash
// or an error
func ReadHash(machineDir string) (common.Hash, error) {
	s, err := ReadHashHex(machineDir)
	if err != nil {
		return common.Hash{}, err
	}
	return common.HexToHash(s), nil
}

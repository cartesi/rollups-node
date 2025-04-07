// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package util

import (
	"fmt"
	"os"
	"path"

	"github.com/ethereum/go-ethereum/common"
)

// Reads the Cartesi Machine hash from machineDir. Returns it as a hex string or
// an error
func ReadHash(machineDir string) (string, error) {
	path := path.Join(machineDir, "hash")
	hash, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read hash: %w", err)
	} else if len(hash) != common.HashLength {
		return "", fmt.Errorf(
			"read hash: wrong size; expected %v bytes but read %v",
			common.HashLength,
			len(hash),
		)
	}
	return common.Bytes2Hex(hash), nil
}

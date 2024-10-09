// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package machine provides mechanisms to handle Cartesi Machine Snapshots
// for development and tests
package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const SNAPSHOT_CONTAINER_PATH = "/usr/share/cartesi/snapshot"

func runCommand(name string, args ...string) error {
	slog.Debug("Running command", "name", name, "args", strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("'%v %v' failed with %w: %v",
			name, strings.Join(args, " "), err, string(output),
		)
	}
	return nil
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)

	// Check if the file exists by examining the error returned by os.Stat
	return !errors.Is(err, fs.ErrNotExist)
}

func Save(destDir string) error {

	// Remove previous snapshot dir
	if fileExists(destDir) {
		slog.Info("Removing previous snapshot")
		err := os.RemoveAll(destDir)
		if err != nil {
			return err
		}
	}

	err := runCommand("cartesi-machine", "--ram-length=128Mi", "--store="+destDir,
		"--", "ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --verbose=1")
	if err != nil {
		return err
	}

	slog.Info("Cartesi machine snapshot saved on",
		"destination-dir", destDir)
	return nil
}

func CreateDefaultMachineSnapshot() (string, error) {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", err
	}
	if err = Save(tmpDir); err != nil {
		return "", err
	}
	return tmpDir, nil
}

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

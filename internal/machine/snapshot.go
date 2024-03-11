// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package machine provides mechanisms to handle Cartesi Machine Snapshots
// to run Node locally for development and tests
package machine

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"strings"
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

func Save(sourceDockerImage string, destDir string, tempContainerName string) error {

	// Remove previous snapshot dir
	if fileExists(destDir) {
		slog.Info("Removing previous snapshot")
		err := os.RemoveAll(destDir)
		if err != nil {
			return err
		}
	}

	// Copy machine snapshot from Docker Container
	err := runCommand("docker", "create", "--name", tempContainerName, sourceDockerImage)
	if err != nil {
		return err
	}

	defer func() {
		err := runCommand("docker", "rm", tempContainerName)
		if err != nil {
			slog.Warn("Error trying to delete container",
				"container", tempContainerName,
				"error", err)
		}
	}()

	fromDir := fmt.Sprintf("%v:%v", tempContainerName, SNAPSHOT_CONTAINER_PATH)
	err = runCommand("docker", "cp", fromDir, destDir)
	if err != nil {
		return err
	}

	slog.Info("Cartesi machine snapshot saved",
		"docker-image", sourceDockerImage,
		"destination-dir", destDir)
	return nil
}

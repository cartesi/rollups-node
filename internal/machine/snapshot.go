// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package machine provides mechanisms to handle Cartesi Machine Snapshots
// to run Node locally for development and tests
package machine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cartesi/rollups-node/internal/config"
)

func runCommand(name string, args ...string) error {
	config.InfoLogger.Printf("%v %v", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("'%v %v' failed with %v: %v",
			name, strings.Join(args, " "), err, string(output))
	}
	return nil
}

func getAbsolutePath(relativePath string) (string, error) {
	// Get the absolute path for the current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Join the current directory and the relative path to get the absolute path
	absolutePath := filepath.Join(currentDir, relativePath)

	// Clean the path to remove any redundant elements
	absolutePath, err = filepath.Abs(absolutePath)
	if err != nil {
		return "", err
	}

	return absolutePath, nil
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)

	// Check if the file exists by examining the error returned by os.Stat
	return !os.IsNotExist(err)
}

func Save(sourceDockerImage string, destDir string, tempContainerName string) error {

	// Create Snapshot dir
	err := os.MkdirAll(destDir, os.ModePerm)
	if err != nil {
		return err
	}

	// Copy machine snapshot from Docker Container
	err = runCommand("docker", "create", "--name", tempContainerName, sourceDockerImage)
	if err != nil {
		return err
	}

	defer func() {
		err := runCommand("docker", "rm", tempContainerName)
		if err != nil {
			config.ErrorLogger.Printf("Error trying to delete %v: %v", tempContainerName, err)
		}
	}()

	err = runCommand("docker", "cp", fmt.Sprintf("%v:/var/opt/cartesi/machine-snapshots/0_0",
		tempContainerName), filepath.Join(destDir, "0_0"))
	if err != nil {
		return err
	}

	// Create 'latest' symlink
	symLinkPath := filepath.Join(destDir, "latest")
	if fileExists(symLinkPath) {
		err = os.Remove(symLinkPath)
		if err != nil {
			return err
		}
	}

	fullPath, pathErr := getAbsolutePath(filepath.Join(destDir, "0_0"))
	if pathErr != nil {
		return pathErr
	}

	err = os.Symlink(fullPath, symLinkPath)
	if err != nil {
		return err
	}

	config.InfoLogger.Printf("Cartesi machine snapshot from %v saved to %v",
		sourceDockerImage, destDir)
	return nil

}

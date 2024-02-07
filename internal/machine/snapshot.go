// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package machine provides mechanisms to handle Cartesi Machine Snapshots
// to run Node locally for development and tests
package machine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

const SNAPSHOT_CONTAINER_PATH = "/usr/share/cartesi/snapshot"

func runCommand(name string, args ...string) error {
	fmt.Printf("%v %v\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("'%v %v' failed with %v: %v",
			name,
			strings.Join(args, " "),
			err,
			string(output),
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
		fmt.Println("removing previous snapshot")
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
			fmt.Printf("Error trying to delete %v: %v\n", tempContainerName, err)
		}
	}()

	fromDir := fmt.Sprintf("%v:%v", tempContainerName, SNAPSHOT_CONTAINER_PATH)
	err = runCommand("docker", "cp", fromDir, destDir)
	if err != nil {
		return err
	}

	fmt.Printf("Cartesi machine snapshot from %v saved to %v\n", sourceDockerImage, destDir)
	return nil

}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var full = &cobra.Command{
	Use:   "full",
	Short: "Starts the node in full mode with reader and validator capabilities",
	Run:   runFullNode,
}

func runFullNode(cmd *cobra.Command, args []string) {
	proc := exec.Command("cartesi-rollups-graphql-server")

	stdoutPipe, err := proc.StdoutPipe()
	if err != nil {
		log.Fatal("Error creating stdout pipe:", err)
	}

	stderrPipe, err := proc.StderrPipe()
	if err != nil {
		log.Fatal("Error creating stderr pipe:", err)
	}

	if err := proc.Start(); err != nil {
		log.Fatal("Error starting sub-process:", err)
	}

	// Create goroutines to display the sub-process's stdout and stderr.
	go func() {
		io.Copy(os.Stdout, stdoutPipe)
	}()

	go func() {
		io.Copy(os.Stderr, stderrPipe)
	}()

	if err := proc.Wait(); err != nil {
		log.Fatal("Error waiting for sub-process:", err)
	}

	log.Println("Sub-process finished")
}

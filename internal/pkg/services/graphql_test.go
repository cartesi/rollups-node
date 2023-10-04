// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/pkg/logger"
)

func setup() {
	logger.Init("warning", false)
	setRustBinariesPath()
}
func TestGraphQLService(t *testing.T) {

	t.Run("it stops when the context is cancelled", func(t *testing.T) {
		setup()
		service := GraphQLService{}
		ctx, cancel := context.WithCancel(context.Background())
		exit := make(chan error)

		go func() {
			if err := service.Start(ctx); err != nil {
				exit <- err
			}
		}()

		<-time.After(100 * time.Millisecond)
		cancel()

		err := <-exit
		exitError, ok := err.(*exec.ExitError)
		if !ok || !assertExitErrorWasCausedBy(exitError, syscall.SIGTERM) {
			t.Logf("service exited for the wrong reason: %v", err)
			t.FailNow()
		}
	})
}

func setRustBinariesPath() {
	rustBinPath, _ := filepath.Abs("../../../offchain/target/debug")
	os.Setenv("PATH", os.Getenv("PATH")+":"+rustBinPath)
}

func assertExitErrorWasCausedBy(err *exec.ExitError, signal syscall.Signal) bool {
	status := err.Sys().(syscall.WaitStatus)
	return status.Signal() == signal
}

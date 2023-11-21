// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/logger"
)

func setup() {
	logger.Init("warning", false)
	setRustBinariesPath()
}

func TestService(t *testing.T) {

	t.Run("it stops when the context is cancelled", func(t *testing.T) {
		setup()
		service := Service{
			name:       "graphql-server",
			binaryName: "cartesi-rollups-graphql-server",
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// start service in goroutine
		result := make(chan error)
		go func() {
			result <- service.Start(ctx)
		}()

		// wait for a while
		time.Sleep(100 * time.Millisecond)

		//shutdown
		cancel()
		if err := <-result; err != nil {
			t.Errorf("service exited for the wrong reason: %v", err)
		}
	})
}

func setRustBinariesPath() {
	rustBinPath, _ := filepath.Abs("../../offchain/target/debug")
	os.Setenv("PATH", os.Getenv("PATH")+":"+rustBinPath)
}

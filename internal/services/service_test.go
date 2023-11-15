// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/logger"
)

func setup() {
	logger.Init("debug", false)
	buildFakeService()
}

func TestService(t *testing.T) {
	setup()

	t.Run("it stops when the context is cancelled", func(t *testing.T) {
		service := Service{
			name:       "fake-service",
			binaryName: "fake-service",
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		startErr := make(chan error)
		success := make(chan struct{})
		go func() {
			if err := service.Start(ctx); err != nil {
				startErr <- err
			}
			success <- struct{}{}
		}()

		<-time.After(100 * time.Millisecond)
		cancel()

		select {
		case err := <-startErr:
			t.Errorf("service exited for the wrong reason: %v", err)
		case <-success:
			return
		}
	})

	t.Run("it stops when timeout is reached and it isn't ready yet", func(t *testing.T) {
		service := Service{
			name:            "fake-service",
			binaryName:      "fake-service",
			healthcheckPort: "0000", //wrong port
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		startErr := make(chan error, 1)
		go func() {
			if err := service.Start(ctx); err != nil {
				startErr <- err
			}
		}()

		readyErr := make(chan error, 1)
		success := make(chan struct{}, 1)
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 1*time.Second)
		defer timeoutCancel()
		go func() {
			if err := service.Ready(timeoutCtx); err == nil {
				readyErr <- fmt.Errorf("expected service to timeout")
			}
			success <- struct{}{}
		}()

		select {
		case err := <-startErr:
			t.Errorf("service failed to start: %v", err)
		case err := <-readyErr:
			t.Error(err)
		case <-success:
			return
		}
	})

	t.Run("it becomes ready soon after being started", func(t *testing.T) {
		service := Service{
			name:            "fake-service",
			binaryName:      "fake-service",
			healthcheckPort: "8090",
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		startErr := make(chan error, 1)
		go func() {
			if err := service.Start(ctx); err != nil {
				startErr <- err
			}
		}()

		readyErr := make(chan error, 1)
		success := make(chan struct{}, 1)
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 5*time.Second)
		defer timeoutCancel()
		go func() {
			if err := service.Ready(timeoutCtx); err != nil {
				readyErr <- err
			}
			success <- struct{}{}
		}()

		select {
		case err := <-startErr:
			t.Errorf("service failed to start: %v", err)
		case err := <-readyErr:
			t.Errorf("service wasn't ready in time. %v", err)
		case <-success:
			return
		}
	})
}

// Builds the fake-service binary and adds it to PATH
func buildFakeService() {
	rootDir, err := filepath.Abs("../../")
	if err != nil {
		panic(err)
	}

	cmd := exec.Command("go", "build", "-o", "build/fake-service", "test/fakeservice/main.go")
	cmd.Dir = rootDir
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	execPath := filepath.Join(rootDir, "build")
	os.Setenv("PATH", os.Getenv("PATH")+":"+execPath)
}

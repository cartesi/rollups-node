// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
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

const (
	servicePort   = "55555"
	serviceAdress = "0.0.0.0:" + servicePort
)

func TestService(t *testing.T) {
	setup()

	t.Run("it stops when the context is cancelled", func(t *testing.T) {
		service := Service{
			name:            "fake-service",
			binaryName:      "fake-service",
			healthcheckPort: servicePort,
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// start service in goroutine
		result := make(chan error)
		go func() {
			result <- service.Start(ctx)
		}()

		// wait for a little
		time.Sleep(100 * time.Millisecond)

		// shutdown
		cancel()
		if err := <-result; err != nil {
			t.Errorf("service exited for the wrong reason: %v", err)
		}
	})

	t.Run("it stops when timeout is reached and it isn't ready yet", func(t *testing.T) {
		service := Service{
			name:            "fake-service",
			binaryName:      "fake-service",
			healthcheckPort: "0000", // wrong port
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// start service in goroutine
		result := make(chan error, 1)
		go func() {
			result <- service.Start(ctx)
		}()

		// expect timeout because of wrong port
		if err := service.Ready(ctx, 500*time.Millisecond); err == nil {
			t.Errorf("expected service to timeout")
		}

		// shutdown
		cancel()
		if err := <-result; err != nil {
			t.Errorf("service exited for the wrong reason: %v", err)
		}
	})

	t.Run("it becomes ready soon after being started", func(t *testing.T) {
		service := Service{
			name:            "fake-service",
			binaryName:      "fake-service",
			healthcheckPort: servicePort,
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// start service in goroutine
		result := make(chan error)
		go func() {
			result <- service.Start(ctx)
		}()

		// wait for service to be ready
		if err := service.Ready(ctx, 500*time.Millisecond); err != nil {
			t.Errorf("service timed out")
		}

		// shutdown
		cancel()
		if err := <-result; err != nil {
			t.Errorf("service exited for the wrong reason: %v", err)
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
	os.Setenv("SERVICE_ADDRESS", serviceAdress)
}

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

const (
	servicePort   = "44444"
	serviceAdress = "0.0.0.0:" + servicePort
)

var tmpDir string

func setup() {
	logger.Init("warning", false)
	buildFakeService()
}

func tearDown() {
	deleteTempDir()
}

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

	tearDown()
}

// Builds the fake-service binary and adds it to PATH
func buildFakeService() {
	temporaryDir, err := os.MkdirTemp("", "")
	if err != nil {
		panic(err)
	}
	tmpDir = temporaryDir

	cmd := exec.Command(
		"go",
		"build",
		"-o",
		filepath.Join(tmpDir, "fake-service"),
		"fakeservice/main.go",
	)
	if err := cmd.Run(); err != nil {
		panic(err)
	}
	os.Setenv("PATH", os.Getenv("PATH")+":"+tmpDir)
}

func deleteTempDir() {
	err := os.RemoveAll(tmpDir)
	if err != nil {
		panic(err)
	}
}

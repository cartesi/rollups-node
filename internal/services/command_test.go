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

	"github.com/stretchr/testify/suite"
)

type CommandServiceSuite struct {
	suite.Suite
	tmpDir      string
	servicePort int
}

func (s *CommandServiceSuite) SetupSuite() {
	s.buildFakeService()
	s.servicePort = 55555
}

func (s *CommandServiceSuite) TearDownSuite() {
	err := os.RemoveAll(s.tmpDir)
	if err != nil {
		panic(err)
	}
}

func (s *CommandServiceSuite) SetupTest() {
	s.servicePort++
	serviceAddress := "0.0.0.0:" + fmt.Sprint(s.servicePort)
	os.Setenv("SERVICE_ADDRESS", serviceAddress)
}

func (s *CommandServiceSuite) TestItStopsWhenContextIsCancelled() {
	service := CommandService{
		Name:            "fake-service",
		Path:            "fake-service",
		HealthcheckPort: s.servicePort,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start service in goroutine
	result := make(chan error)
	ready := make(chan struct{})
	go func() {
		result <- service.Start(ctx, ready)
	}()

	// assert service started successfully
	select {
	case err := <-result:
		s.FailNow("service failed to start", err)
	case <-ready:
	}

	cancel()

	err := <-result
	s.ErrorIs(err, context.Canceled, "service exited for the wrong reason: %v", err)
}

// Service should stop if timeout is reached and it isn't ready yet
func (s *CommandServiceSuite) TestItTimesOut() {
	service := CommandService{
		Name:            "fake-service",
		Path:            "fake-service",
		HealthcheckPort: 0, // wrong port
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start service in goroutine˝
	result := make(chan error)
	ready := make(chan struct{})
	go func() {
		result <- service.Start(ctx, ready)
	}()

	// expect timeout because of wrong port
	select {
	case <-ready:
		s.FailNow("service should have timed out")
	case <-time.After(2 * time.Second):
		cancel()
		err := <-result
		s.ErrorIs(err, context.Canceled, "service exited for the wrong reason: %v", err)
	}
}

func (s *CommandServiceSuite) TestItFailsToStartIfExecutableNotInPath() {
	service := CommandService{
		Name:            "fake-service",
		Path:            "wrong-path",
		HealthcheckPort: s.servicePort,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})

	err := service.Start(ctx, ready)

	s.ErrorIs(err, exec.ErrNotFound, "service exited for the wrong reason: %v", err)
}

// Builds the fake-service binary and adds it to PATH
func (s *CommandServiceSuite) buildFakeService() {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		panic(err)
	}
	s.tmpDir = tempDir

	cmd := exec.Command(
		"go",
		"build",
		"-o",
		filepath.Join(s.tmpDir, "fake-service"),
		"fakeservice/main.go",
	)
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	os.Setenv("PATH", os.Getenv("PATH")+":"+s.tmpDir)
}

func TestCommandService(t *testing.T) {
	suite.Run(t, new(CommandServiceSuite))
}

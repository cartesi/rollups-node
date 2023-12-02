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

type ServiceTestSuite struct {
	suite.Suite
	tmpDir      string
	servicePort int
}

func (s *ServiceTestSuite) SetupSuite() {
	s.buildFakeService()
	s.servicePort = 55555
}

func (s *ServiceTestSuite) TearDownSuite() {
	err := os.RemoveAll(s.tmpDir)
	if err != nil {
		panic(err)
	}
}

func (s *ServiceTestSuite) SetupTest() {
	s.servicePort++
	serviceAdress := "0.0.0.0:" + fmt.Sprint(s.servicePort)
	os.Setenv("SERVICE_ADDRESS", serviceAdress)
}

// Service should stop when context is cancelled
func (s *ServiceTestSuite) TestServiceStops() {
	service := Service{
		Name:            "fake-service",
		Path:            "fake-service",
		HealthcheckPort: s.servicePort,
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
	err := <-result
	s.Nil(err, "service exited for the wrong reason: %v", err)
}

// Service should stop if timeout is reached and it isn't ready yet
func (s *ServiceTestSuite) TestServiceTimeout() {
	service := Service{
		Name:            "fake-service",
		Path:            "fake-service",
		HealthcheckPort: 0, // wrong port
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start service in goroutine
	result := make(chan error, 1)
	go func() {
		result <- service.Start(ctx)
	}()

	// expect timeout because of wrong port
	err := service.Ready(ctx, 500*time.Millisecond)
	s.NotNil(err, "expected service to timeout")

	// shutdown
	cancel()
	s.Nil(<-result, "service exited for the wrong reason: %v", err)
}

// Service should be ready soon after starting
func (s *ServiceTestSuite) TestServiceReady() {
	service := Service{
		Name:            "fake-service",
		Path:            "fake-service",
		HealthcheckPort: s.servicePort,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// start service in goroutine
	result := make(chan error)
	go func() {
		result <- service.Start(ctx)
	}()

	// wait for service to be ready
	err := service.Ready(ctx, 1*time.Second)
	s.Nil(err, "service timed out")

	// shutdown
	cancel()
	s.Nil(<-result, "service exited for the wrong reason: %v", err)
}

// Builds the fake-service binary and adds it to PATH
func (s *ServiceTestSuite) buildFakeService() {
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

func TestServiceSuite(t *testing.T) {
	suite.Run(t, new(ServiceTestSuite))
}

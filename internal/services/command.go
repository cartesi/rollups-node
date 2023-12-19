// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
)

const (
	DefaultPollInterval = 100 * time.Millisecond
)

type CommandService struct {

	// Name that identifies the service.
	Name string

	// Port used to verify if the service is ready.
	HealthcheckPort int

	// Path to the service binary.
	Path string

	// Args to the service binary.
	Args []string

	// Environment variables.
	Env []string
}

func (s CommandService) Start(ctx context.Context, ready chan<- struct{}) error {
	cmd := exec.CommandContext(ctx, s.Path, s.Args...)
	cmd.Env = s.Env
	cmd.Stderr = commandLogger{s.Name}
	cmd.Stdout = commandLogger{s.Name}
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			msg := "failed to send SIGTERM to %v: %v\n"
			config.WarningLogger.Printf(msg, s, err)
		}
		return err
	}
	go s.pollTcp(ctx, ready)
	err := cmd.Run()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Blocks until the service is ready or the context is canceled.
func (s CommandService) pollTcp(ctx context.Context, ready chan<- struct{}) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		conn, err := net.Dial("tcp", fmt.Sprintf("0.0.0.0:%v", s.HealthcheckPort))
		if err == nil {
			config.DebugLogger.Printf("%s is ready\n", s)
			conn.Close()
			ready <- struct{}{}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(DefaultPollInterval):
		}
	}
}

func (s CommandService) String() string {
	return s.Name
}

type commandLogger struct {
	Name string
}

func (l commandLogger) Write(data []byte) (int, error) {
	config.InfoLogger.Printf("%v: %v", l.Name, string(data))
	return len(data), nil
}

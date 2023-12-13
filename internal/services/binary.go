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

	"github.com/cartesi/rollups-node/internal/logger"
)

// A service that executes a binary located at Path. Implements service.Service
// and fmt.Stringer
type BinaryService struct {

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

func (s BinaryService) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, s.Path, s.Args...)
	cmd.Env = s.Env
	cmd.Stderr = serviceLogger{s.Name}
	cmd.Stdout = serviceLogger{s.Name}
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			msg := "failed to send SIGTERM to %v: %v\n"
			logger.Warning.Printf(msg, s.Name, err)
		}
		return err
	}
	err := cmd.Run()
	if err != nil {
		exitCode := cmd.ProcessState.ExitCode()
		signal := cmd.ProcessState.Sys().(syscall.WaitStatus).Signal()
		if exitCode != 0 && signal != syscall.SIGTERM {
			// only return error if the service exits for reason other than shutdown
			return err
		}
	}
	return nil
}

func (s BinaryService) Ready(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		conn, err := net.Dial("tcp", fmt.Sprintf("0.0.0.0:%v", s.HealthcheckPort))
		if err == nil {
			logger.Debug.Printf("%s is ready\n", s.Name)
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(DefaultDialInterval):
		}
	}
}

func (s BinaryService) String() string {
	return s.Name
}

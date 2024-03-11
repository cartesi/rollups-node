// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ServerManager is a variation of CommandService used to manually stop
// the orphaned cartesi-machines left after server-manager exits.
// For more information, check https://github.com/cartesi/server-manager/issues/18
type ServerManager struct {
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

	// Bypass the log and write directly to stdout/stderr.
	BypassLog bool
}

const waitDelay = 200 * time.Millisecond

func (s ServerManager) Start(ctx context.Context, ready chan<- struct{}) error {
	cmd := exec.CommandContext(ctx, s.Path, s.Args...)
	cmd.Env = s.Env
	if s.BypassLog {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
	} else {
		cmd.Stderr = newLineWriter(commandLogger{s.Name})
		cmd.Stdout = newLineWriter(commandLogger{s.Name})
	}
	// Without a delay, cmd.Wait() will block forever waiting for the I/O pipes
	// to be closed
	cmd.WaitDelay = waitDelay
	cmd.Cancel = func() error {
		err := killChildProcesses(cmd.Process.Pid)
		if err != nil {
			slog.Warn("Failed to kill child processes", "service", s, "error", err)
		}
		err = cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			slog.Warn("Failed to send SIGTERM", "service", s, "error", err)
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

// Blocks until the service is ready or the context is canceled
func (s ServerManager) pollTcp(ctx context.Context, ready chan<- struct{}) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		conn, err := net.Dial("tcp", fmt.Sprintf("0.0.0.0:%v", s.HealthcheckPort))
		if err == nil {
			slog.Debug("Service is ready", "service", s)
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

func (s ServerManager) String() string {
	return s.Name
}

// Kills all child processes spawned by pid
func killChildProcesses(pid int) error {
	children, err := getChildrenPid(pid)
	if err != nil {
		return fmt.Errorf("failed to get child processes. %v", err)
	}
	for _, child := range children {
		err = syscall.Kill(child, syscall.SIGKILL)
		if err != nil {
			return fmt.Errorf("failed to kill child process: %v. %v\n", child, err)
		}
	}
	return nil
}

// Returns a list of processes whose parent is ppid
func getChildrenPid(ppid int) ([]int, error) {
	output, err := exec.Command("pgrep", "-P", fmt.Sprint(ppid)).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to exec pgrep: %v: %v", err, string(output))
	}

	var children []int
	pids := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, pid := range pids {
		childPid, err := strconv.Atoi(pid)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pid: %v", err)
		}
		children = append(children, childPid)
	}
	return children, nil
}

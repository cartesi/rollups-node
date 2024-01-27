// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/linewriter"
)

const (
	DefaultPollInterval = 100 * time.Millisecond
)

// CommandService encapsulates the execution of an executable via exec.Command.
// It assumes the executable accepts TCP connections at HealthcheckPort,
// which it uses to determine if it is ready or not.
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
	cmd.Stderr = linewriter.New(commandLogger{s.Name})
	cmd.Stdout = linewriter.New(commandLogger{s.Name})
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(syscall.SIGTERM)
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

// Blocks until the service is ready or the context is canceled.
func (s CommandService) pollTcp(ctx context.Context, ready chan<- struct{}) {
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

func (s CommandService) String() string {
	return s.Name
}

// A wrapper around slog.Default that writes log output from a services.CommandService
// to the correct log level
type commandLogger struct {
	Name string
}

func (l commandLogger) Write(data []byte) (int, error) {
	// If data does has no alphanumeric characters, ignore it.
	if match := alphanumericRegex.Find(data); match == nil {
		return 0, nil
	}
	msg := strings.TrimSpace(string(data))
	level := l.logLevelForMessage(msg)
	slog.Log(context.Background(), level, msg, "service", l.Name)
	return len(msg), nil
}

var (
	errorRegex        = regexp.MustCompile(`(?i)(error|fatal)`)
	warnRegex         = regexp.MustCompile(`(?i)warn`)
	infoRegex         = regexp.MustCompile(`(?i)info`)
	debugRegex        = regexp.MustCompile(`(?i)(debug|trace)`)
	alphanumericRegex = regexp.MustCompile("[a-zA-Z0-9]")
)

// Uses regular expressions to determine the correct log level. If there is no match,
// returns slog.LevelInfo
func (l commandLogger) logLevelForMessage(msg string) slog.Level {
	if match := infoRegex.FindString(msg); len(match) > 0 {
		return slog.LevelInfo
	} else if match = debugRegex.FindString(msg); len(match) > 0 {
		return slog.LevelDebug
	} else if match = warnRegex.FindString(msg); len(match) > 0 {
		return slog.LevelWarn
	} else if match = errorRegex.FindString(msg); len(match) > 0 {
		return slog.LevelError
	}
	return slog.LevelInfo
}

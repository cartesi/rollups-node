// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cartesimachine

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/cartesi/rollups-node/internal/services/linewriter"
	"github.com/cartesi/rollups-node/pkg/emulator"
)

type MachineLogLevel string

const (
	MachineLogLevelTrace MachineLogLevel = "trace"
	MachineLogLevelDebug MachineLogLevel = "debug"
	MachineLogLevelInfo  MachineLogLevel = "info"
	MachineLogLevelWarn  MachineLogLevel = "warn"
	MachineLogLevelError MachineLogLevel = "error"
	MachineLogLevelFatal MachineLogLevel = "fatal"
)

func ParseMachineLogLevel(level string) (MachineLogLevel, error) {
	switch level {
	case string(MachineLogLevelTrace):
		return MachineLogLevelTrace, nil
	case string(MachineLogLevelDebug):
		return MachineLogLevelDebug, nil
	case string(MachineLogLevelInfo):
		return MachineLogLevelInfo, nil
	case string(MachineLogLevelWarn):
		return MachineLogLevelWarn, nil
	case string(MachineLogLevelError):
		return MachineLogLevelError, nil
	case string(MachineLogLevelFatal):
		return MachineLogLevelFatal, nil
	default:
		return "", fmt.Errorf("invalid remote machine log level")
	}
}

var (
	ErrNilLogger = errors.New("logger must not be nil")
)

// StartServer starts a JSON RPC remote cartesi machine server.
//
// It configures the server's logging verbosity and initializes its address to 127.0.0.1:port.
// If verbosity is an invalid LogLevel, a default value will be used instead.
// If port is 0, a random valid port will be used instead.
//
// StartServer also redirects the server's stdout and stderr to the provided io.Writers.
//
// It returns the server's address.
func StartServer(logger *slog.Logger, verbosity MachineLogLevel, port uint32, stdout, stderr io.Writer) (string, error) {
	if logger == nil {
		return "", ErrNilLogger
	}

	// Configures the command's arguments.
	args := []string{}
	if verbosity.valid() {
		args = append(args, "--log-level="+string(verbosity))
	}
	if port != 0 {
		args = append(args, fmt.Sprintf("--server-address=127.0.0.1:%d", port))
	}

	// Creates the command.
	cmd := exec.Command("jsonrpc-remote-cartesi-machine", args...)

	// Redirects stdout and stderr.
	interceptor := portInterceptor{
		inner: stderr,
		port:  make(chan uint32),
		found: new(bool),
	}
	cmd.Stdout = stdout
	cmd.Stderr = linewriter.New(interceptor)

	// Starts the server.
	logger.Info("Starting remote machine server", "command", cmd.String())
	if err := cmd.Start(); err != nil {
		return "", err
	}

	// Waits for the interceptor to write the port to the channel.
	if actualPort := <-interceptor.port; port == 0 {
		port = actualPort
	} else if port != actualPort {
		return "", fmt.Errorf("mismatching ports (%d != %d)", port, actualPort)
	}

	address := fmt.Sprintf("127.0.0.1:%d", port)
	return address, nil
}

// StopServer shuts down the JSON RPC remote cartesi machine server hosted at address.
func StopServer(address string, logger *slog.Logger) error {
	if logger == nil {
		return ErrNilLogger
	}

	logger.Info("Stopping server at", "address", address)
	remote, err := emulator.ConnectServer(address)
	if err != nil {
		return err
	}
	return remote.ShutdownServer()
}

// ------------------------------------------------------------------------------------------------

func (verbosity MachineLogLevel) valid() bool {
	return verbosity == MachineLogLevelTrace ||
		verbosity == MachineLogLevelDebug ||
		verbosity == MachineLogLevelInfo ||
		verbosity == MachineLogLevelWarn ||
		verbosity == MachineLogLevelError ||
		verbosity == MachineLogLevelFatal
}

// portInterceptor sends the server's port through the port channel as soon as it reads it.
// It then closes the channel and keeps on writing to the inner writer.
//
// It expects to be wrapped by a linewriter.LineWriter.
type portInterceptor struct {
	inner io.Writer
	port  chan uint32
	found *bool
}

var portRegex = regexp.MustCompile("remote machine server bound to [^:]+:([0-9]+)")

func (writer portInterceptor) Write(p []byte) (n int, err error) {
	if *writer.found {
		return writer.inner.Write(p)
	} else {
		matches := portRegex.FindStringSubmatch(string(p))
		if matches != nil {
			port, err := strconv.ParseUint(matches[1], 10, 32)
			if err != nil {
				return 0, err
			}
			*writer.found = true
			writer.port <- uint32(port)
			close(writer.port)
		}
		return writer.inner.Write(p)
	}
}

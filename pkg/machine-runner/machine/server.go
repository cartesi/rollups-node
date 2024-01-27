// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/cartesi/rollups-node/internal/linewriter"
	"github.com/cartesi/rollups-node/pkg/emulator"
)

type ServerLogLevel string

const (
	ServerLogLevelTrace ServerLogLevel = "trace"
	ServerLogLevelDebug ServerLogLevel = "debug"
	ServerLogLevelInfo  ServerLogLevel = "info"
	ServerLogLevelWarn  ServerLogLevel = "warn"
	ServerLogLevelError ServerLogLevel = "error"
	ServerLogLevelFatal ServerLogLevel = "fatal"
)

// StartServer starts the JSON RPC remote cartesi machine server.
//
// It configures the server's logging verbosity and initializes its address to localhost:port.
// If verbosity is an invalid LogLevel, a default value will be used instead.
// If port is 0, a random valid port will be used instead.
//
// StartServer also redirects the server's stdout and stderr to the provided io.Writers.
//
// It returns the server's address.
func StartServer(
	verbosity ServerLogLevel,
	port uint32,
	stdout io.Writer,
	stderr io.Writer,
) (string, error) {
	// Configures the command's arguments.
	args := []string{}
	if verbosity.valid() {
		args = append(args, "--log-level="+string(verbosity))
	}
	if port != 0 {
		args = append(args, fmt.Sprintf("--server-address=localhost:%d", port))
	}

	// Creates the command.
	cmd := exec.Command("jsonrpc-remote-cartesi-machine", args...)
	slog.Info(cmd.String())

	// Redirects stdout and stderr.
	intercepter := portIntercepter{
		inner: stderr,
		port:  make(chan uint32),
		found: new(bool),
	}
	cmd.Stdout = stdout
	cmd.Stderr = linewriter.New(intercepter)

	// Starts the server.
	if err := cmd.Start(); err != nil {
		return "", err
	}

	// Waits for the intercepter to write the port to the channel.
	if actualPort := <-intercepter.port; port == 0 {
		port = actualPort
	} else if port != actualPort {
		panic(fmt.Sprintf("mismatching ports (%d != %d)", port, actualPort))
	}

	return fmt.Sprintf("localhost:%d", port), nil
}

// StopServer shuts down the JSON RPC remote cartesi machine server hosted in address.
//
// Most users of the machine library should not call this function.
// We recommend using machine.Destroy() instead.
func StopServer(address string) error {
	remote, err := emulator.NewRemoteMachineManager(address)
	if err != nil {
		return err
	}
	defer remote.Delete()
	return remote.Shutdown()
}

// ------------------------------------------------------------------------------------------------

func (logLevel ServerLogLevel) valid() bool {
	return logLevel == ServerLogLevelTrace ||
		logLevel == ServerLogLevelDebug ||
		logLevel == ServerLogLevelInfo ||
		logLevel == ServerLogLevelWarn ||
		logLevel == ServerLogLevelError ||
		logLevel == ServerLogLevelFatal
}

// portIntercepter sends the server's port through the port channel as soon as it reads it.
// It then closes the channel and keeps on writing to the inner writer.
//
// It expects to be wrapped by a linewriter.LineWriter.
type portIntercepter struct {
	inner io.Writer
	port  chan uint32
	found *bool
}

var regex = regexp.MustCompile("initial server bound to port ([0-9]+)")

func (writer portIntercepter) Write(p []byte) (n int, err error) {
	if *writer.found {
		return writer.inner.Write(p)
	} else {
		matches := regex.FindStringSubmatch(string(p))
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

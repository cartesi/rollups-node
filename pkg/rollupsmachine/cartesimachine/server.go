// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cartesimachine

import (
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/cartesi/rollups-node/internal/services/linewriter"
	"github.com/cartesi/rollups-node/pkg/emulator"
)

type ServerVerbosity string

const (
	ServerVerbosityTrace ServerVerbosity = "trace"
	ServerVerbosityDebug ServerVerbosity = "debug"
	ServerVerbosityInfo  ServerVerbosity = "info"
	ServerVerbosityWarn  ServerVerbosity = "warn"
	ServerVerbosityError ServerVerbosity = "error"
	ServerVerbosityFatal ServerVerbosity = "fatal"
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
func StartServer(verbosity ServerVerbosity, port uint32, stdout, stderr io.Writer) (string, error) {
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
	slog.Info("running", "command", cmd.String())
	if err := cmd.Start(); err != nil {
		return "", err
	}

	// Waits for the interceptor to write the port to the channel.
	if actualPort := <-interceptor.port; port == 0 {
		port = actualPort
	} else if port != actualPort {
		panic(fmt.Sprintf("mismatching ports (%d != %d)", port, actualPort))
	}

	return fmt.Sprintf("127.0.0.1:%d", port), nil
}

// StopServer shuts down the JSON RPC remote cartesi machine server hosted at address.
func StopServer(address string) error {
	slog.Info("Stopping server at", "address", address)
	remote, err := emulator.NewRemoteMachineManager(address)
	if err != nil {
		return err
	}
	defer remote.Delete()
	return remote.Shutdown()
}

// ------------------------------------------------------------------------------------------------

func (verbosity ServerVerbosity) valid() bool {
	return verbosity == ServerVerbosityTrace ||
		verbosity == ServerVerbosityDebug ||
		verbosity == ServerVerbosityInfo ||
		verbosity == ServerVerbosityWarn ||
		verbosity == ServerVerbosityError ||
		verbosity == ServerVerbosityFatal
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

var portRegex = regexp.MustCompile("remote machine bound to [^:]+:([0-9]+)")

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

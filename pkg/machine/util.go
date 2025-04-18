// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

// StartServer starts the JSON RPC remote cartesi machine server hosted at address.
func StartServer(logger *slog.Logger, deadline time.Duration) (Backend, string, uint32, error) {
	if logger == nil {
		return nil, "", 0, errors.New("logger must not be nil")
	}

	logger.Info("Starting server")
	server, address, pid, err := emulator.SpawnServer("127.0.0.1:0", deadline)
	if err != nil {
		return nil, "", 0, fmt.Errorf("spawn server failed: %w", err)
	}

	return &LibCartesiBackend{inner: server}, address, pid, nil
}

// StopServer shuts down the JSON RPC remote cartesi machine server hosted at address.
func StopServer(address string, logger *slog.Logger, deadline time.Duration) error {
	if logger == nil {
		return errors.New("logger must not be nil")
	}

	logger.Info("Stopping server at", "address", address)
	remote, err := emulator.ConnectServer(address, deadline)
	if err != nil {
		return err
	}
	return remote.ShutdownServer()
}

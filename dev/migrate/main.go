// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/schema"
)

func main() {
	var s *schema.Schema
	var err error

	postgresEndpoint := config.GetPostgresEndpoint()

	for i := 0; i < 5; i++ {
		s, err = schema.New(postgresEndpoint)
		if err == nil {
			break
		}
		slog.Warn("Connection to database failed. Trying again.", "PostgresEndpoint", postgresEndpoint)
		if i == 4 {
			slog.Error("Failed to connect to database.", "error", err)
			os.Exit(1)
		}
		time.Sleep(5 * time.Second) // wait before retrying
	}
	defer s.Close()

	err = s.Upgrade()
	if err != nil {
		slog.Error("Error while upgrading database schema", "error", err)
		os.Exit(1)
	}

	version, err := s.ValidateVersion()
	if err != nil {
		slog.Error("Error while validating database schema version", "error", err)
		os.Exit(1)
	}

	slog.Info("Database Schema successfully Updated.", "version", version)
}

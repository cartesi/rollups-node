// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"embed"
	_ "embed"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*
var content embed.FS

func RunMigrations(postgres_endpoint string) {
	driver, err := iofs.New(content, "migrations")
	if err != nil {
		slog.Error("Unable to use embed files: ", err)
	}

	migrate, err := migrate.NewWithSourceInstance("iofs", driver, postgres_endpoint)
	if err != nil {
		slog.Error("Unable to setup migrations: ", err)
	}
	if err := migrate.Up(); err != nil {
		slog.Error("Unable to run migrations: ", err)
	}
}

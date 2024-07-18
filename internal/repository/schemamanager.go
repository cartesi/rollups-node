// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	mig "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*
var content embed.FS

const (
	EXPECTED_VERSION = 2
)

type (
	Migrate = mig.Migrate
)

type SchemaManager struct {
	migrate *Migrate
}

func NewSchemaManager(postgresEndpoint string) (*SchemaManager, error) {

	driver, err := iofs.New(content, "migrations")
	if err != nil {
		return nil, err
	}

	if !strings.Contains(postgresEndpoint, "sslmode=disable") {
		postgresEndpoint = fmt.Sprintf("%v?sslmode=disable", postgresEndpoint)
	}
	migrate, err := mig.NewWithSourceInstance(
		"iofs",
		driver,
		postgresEndpoint,
	)
	if err != nil {
		return nil, err
	}
	return &SchemaManager{
		migrate: migrate,
	}, nil

}

func (s *SchemaManager) GetVersion() (uint, error) {

	version, _, err := s.migrate.Version()

	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return version, fmt.Errorf("No valid database schema found")
		}
	}

	return version, err

}

func (s *SchemaManager) Upgrade() error {
	if err := s.migrate.Up(); err != nil {
		if !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
	}
	return nil
}

func (s *SchemaManager) Close() {
	source, db := s.migrate.Close()
	if source != nil {
		slog.Error("Error releasing migration sources", "error", source)
	}
	if db != nil {
		slog.Error("Error closing db connection", "error", db)
	}
}

func (s *SchemaManager) ValidateSchemaVersion() error {
	version, err := s.GetVersion()
	if err != nil {
		return err
	}

	if version != EXPECTED_VERSION {
		return fmt.Errorf(
			"Database schema version mismatch. Expected %d but it is %d",
			EXPECTED_VERSION,
			version,
		)
	}
	return nil
}

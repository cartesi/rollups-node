// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	mig "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*
var content embed.FS

type (
	Migrate = mig.Migrate
)

type SchemaManager struct {
	migrate      *Migrate
	sourceDriver source.Driver
}

func NewSchemaManager(postgresEndpoint string) (*SchemaManager, error) {

	sourceDriver, err := iofs.New(content, "migrations")
	if err != nil {
		return nil, err
	}

	if !strings.Contains(postgresEndpoint, "sslmode=disable") {
		postgresEndpoint = fmt.Sprintf("%v?sslmode=disable", postgresEndpoint)
	}
	migrate, err := mig.NewWithSourceInstance(
		"iofs",
		sourceDriver,
		postgresEndpoint,
	)
	if err != nil {
		return nil, err
	}
	return &SchemaManager{
		migrate:      migrate,
		sourceDriver: sourceDriver,
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

func (s *SchemaManager) GetVersionFromSources() (uint, error) {
	version, err := s.sourceDriver.First()

	if err != nil {
		return 0, err
	}

	for {
		version, err = s.sourceDriver.Next(version)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			return 0, err
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

// Validates the current Database Schema version against sources version
// Returns the current
func (s *SchemaManager) ValidateSchemaVersion() (uint, error) {
	version, err := s.GetVersion()
	if err != nil {
		return 0, err
	}

	expectedVersion, err := s.GetVersionFromSources()
	if err != nil {
		return 0, err
	}

	if version != expectedVersion {
		return 0, fmt.Errorf(
			"Database schema version mismatch. Expected %d but it is %d",
			expectedVersion,
			version,
		)
	}
	return expectedVersion, nil
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package schema

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*
var content embed.FS

const ExpectedVersion uint = 1

type Schema struct {
	migrate *migrate.Migrate
}

var (
	ErrMigrationNotCompleted = errors.New("Schema version is dirty")
	ErrNoValidDatabaseSchema = errors.New("No valid database schema found")
)

// VersionMismatchError contains only the schema versions needed for diagnosis.
type VersionMismatchError struct {
	Expected uint
	Actual   uint
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("database schema version mismatch. Expected %d but it is %d", e.Expected, e.Actual)
}

func New(postgresEndpoint string) (*Schema, error) {
	driver, err := iofs.New(content, "migrations")
	if err != nil {
		return nil, err
	}

	migrate, err := migrate.NewWithSourceInstance("iofs", driver, postgresEndpoint)
	if err != nil {
		return nil, err
	}

	return &Schema{migrate: migrate}, nil
}

func NewWithPool(pool *pgxpool.Pool) (*Schema, error) {
	source, err := iofs.New(content, "migrations")
	if err != nil {
		return nil, err
	}

	db := stdlib.OpenDBFromPool(pool)
	driver, err := pgx.WithInstance(db, &pgx.Config{})
	if err != nil {
		return nil, fmt.Errorf("could not instantiate pgx migrate driver: %v", err)
	}

	migrate, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return nil, err
	}

	return &Schema{migrate: migrate}, nil
}

func (s *Schema) Version() (uint, bool, error) {
	version, dirty, err := s.migrate.Version()
	if err != nil && errors.Is(err, migrate.ErrNilVersion) {
		return version, dirty, ErrNoValidDatabaseSchema
	}
	return version, dirty, err
}

func (s *Schema) Upgrade() error {
	if err := s.migrate.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func (s *Schema) Downgrade() error {
	if err := s.migrate.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func (s *Schema) Close() {
	source, db := s.migrate.Close()
	if source != nil {
		slog.Error("Error releasing migration sources", "error", source)
	}
	if db != nil {
		slog.Error("Error closing db connection", "error", db)
	}
}

func (s *Schema) ValidateVersion() (uint, error) {
	version, dirty, err := s.Version()
	if err != nil {
		return 0, err
	}

	if dirty {
		return 0, ErrMigrationNotCompleted
	}

	if version != ExpectedVersion {
		return 0, &VersionMismatchError{Expected: ExpectedVersion, Actual: version}
	}
	return version, nil
}

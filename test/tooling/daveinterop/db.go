// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
)

var databaseNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// interopDatabasePrefix starts the name of every database that the tool
// creates.
const interopDatabasePrefix = "interop_"

// createDatabase creates a new, empty database next to the one in adminURL,
// applies the node schema, and returns the new connection URL. It refuses an
// existing database: each replay attempt needs single-use state.
func createDatabase(ctx context.Context, adminURL, name string) (string, error) {
	if !databaseNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid database name %q (use lowercase letters, digits, and _)", name)
	}
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return "", withExitCode(exitPrerequisites, fmt.Errorf("connecting to Postgres (%s; is it running? make start-postgres): %w",
			redactURL(adminURL), err))
	}
	defer conn.Close(ctx)
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return "", fmt.Errorf("checking database %s: %w", name, err)
	}
	if exists {
		return "", fmt.Errorf("database %s already exists; use a new name for each replay attempt", name)
	}
	// The name is validated above; identifiers cannot be bound as parameters.
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return "", fmt.Errorf("creating database %s: %w", name, err)
	}

	parsed, err := url.Parse(adminURL)
	if err != nil {
		return "", fmt.Errorf("parsing database URL: %w", err)
	}
	parsed.Path = "/" + name
	newURL := parsed.String()

	s, err := schema.New(newURL)
	if err != nil {
		return "", fmt.Errorf("opening new database: %w", err)
	}
	defer s.Close()
	if err := s.Upgrade(); err != nil {
		return "", fmt.Errorf("migrating %s: %w", name, err)
	}
	if _, err := s.ValidateVersion(); err != nil {
		return "", fmt.Errorf("validating %s schema: %w", name, err)
	}
	return newURL, nil
}

// dropDatabase removes a database created by createDatabase.
func dropDatabase(ctx context.Context, adminURL, name string) error {
	if !databaseNamePattern.MatchString(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connecting to Postgres: %w", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("dropping database %s: %w", name, err)
	}
	return nil
}

func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<invalid URL>"
	}
	return parsed.Redacted()
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
)

type cleanOptions struct {
	casesDir string
	liveDir  string
	dbAdmin  string
	cases    bool
	dryRun   bool
}

type cleanResult struct {
	DryRun    bool     `json:"dry_run"`
	Databases []string `json:"databases"`
	Paths     []string `json:"paths"`
	Warnings  []string `json:"warnings,omitempty"`
}

func newCleanCommand() *cobra.Command {
	opts := &cleanOptions{}
	cmd := &cobra.Command{
		Use:   "clean [--cases] [--dry-run]",
		Short: "Remove run directories and drop the databases that this tool created",
		Long: `clean removes the replay run directories (<cases-dir>/*/runs), the failed
captures that the suite set aside (<cases-dir>/*.failed-*), the live run
directories (<live-dir>), and drops every database whose name starts with
"interop_" (the tool creates one per replay or live run). A database with open
connections is kept, with a warning. Other databases are never touched.

Captured cases are kept, because a capture takes minutes; --cases removes them
too. Stop any run that is still in progress (for example, one started with
--keep) before cleaning.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := runClean(cmd.Context(), opts)
			if printErr := emit(cmd.OutOrStdout(), result); printErr != nil {
				return printErr
			}
			return err
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.casesDir, "cases-dir", filepath.Join("_interop", "cases"), "directory of the case directories")
	flags.StringVar(&opts.liveDir, "live-dir", filepath.Join("_interop", "live"), "directory of the live run directories")
	flags.StringVar(&opts.dbAdmin, "db-admin", defaultDatabaseURL(), "Postgres URL used to drop the run databases")
	flags.BoolVar(&opts.cases, "cases", false, "also remove the captured cases")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "list what would be removed; remove nothing")
	return cmd
}

func runClean(ctx context.Context, opts *cleanOptions) (*cleanResult, error) {
	result := &cleanResult{DryRun: opts.dryRun}

	var paths []string
	runs, err := filepath.Glob(filepath.Join(opts.casesDir, "*", "runs"))
	if err != nil {
		return result, err
	}
	paths = append(paths, runs...)
	failed, err := filepath.Glob(filepath.Join(opts.casesDir, "*"+failedCaseMarker+"*"))
	if err != nil {
		return result, err
	}
	paths = append(paths, failed...)
	if fileExists(opts.liveDir) {
		paths = append(paths, opts.liveDir)
	}
	if opts.cases {
		entries, err := os.ReadDir(opts.casesDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		for _, entry := range entries {
			if entry.IsDir() && !strings.Contains(entry.Name(), failedCaseMarker) {
				paths = append(paths, filepath.Join(opts.casesDir, entry.Name()))
			}
		}
	}
	sort.Strings(paths)
	var failures []error
	for _, path := range paths {
		result.Paths = append(result.Paths, path)
		if opts.dryRun {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			failures = append(failures, err)
		}
	}

	if opts.dbAdmin == "" {
		result.Warnings = append(result.Warnings, "no Postgres URL (CARTESI_DATABASE_CONNECTION or --db-admin); databases not checked")
	} else {
		databases, err := listInteropDatabases(ctx, opts.dbAdmin)
		if err != nil {
			failures = append(failures, err)
		}
		for _, db := range databases {
			if db.Connections > 0 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("kept database %s: %d open connections "+
					"(a run in progress? stop it first)", db.Name, db.Connections))
				continue
			}
			result.Databases = append(result.Databases, db.Name)
			if opts.dryRun {
				continue
			}
			if err := dropDatabase(ctx, opts.dbAdmin, db.Name); err != nil {
				failures = append(failures, err)
			}
		}
	}
	if len(failures) > 0 {
		return result, withExitCode(exitFailure, errors.Join(failures...))
	}
	return result, nil
}

type interopDatabase struct {
	Name        string
	Connections int
}

// listInteropDatabases returns the databases created by replay and live, with
// their open connections.
func listInteropDatabases(ctx context.Context, adminURL string) ([]interopDatabase, error) {
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to Postgres (%s): %w", redactURL(adminURL), err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `SELECT d.datname,
		(SELECT count(*) FROM pg_stat_activity a WHERE a.datname = d.datname)
		FROM pg_database d WHERE starts_with(d.datname, $1) ORDER BY d.datname`, interopDatabasePrefix)
	if err != nil {
		return nil, fmt.Errorf("listing databases: %w", err)
	}
	defer rows.Close()
	var databases []interopDatabase
	for rows.Next() {
		var db interopDatabase
		if err := rows.Scan(&db.Name, &db.Connections); err != nil {
			return nil, err
		}
		if databaseNamePattern.MatchString(db.Name) {
			databases = append(databases, db)
		}
	}
	return databases, rows.Err()
}

func (r *cleanResult) writeText(w io.Writer) {
	verb := "removed"
	if r.DryRun {
		verb = "would remove"
	}
	fmt.Fprintf(w, "clean: %s %d directories and %d databases\n", verb, len(r.Paths), len(r.Databases))
	for _, path := range r.Paths {
		fmt.Fprintf(w, "  dir  %s\n", path)
	}
	for _, name := range r.Databases {
		fmt.Fprintf(w, "  db   %s\n", name)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "  warn %s\n", warning)
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package factory

import (
	"context"
	"fmt"
	"strings"
	"time"

	//    "github.com/cartesi/rollups-node/internal/repository/memory"
	. "github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres"
	//    "github.com/cartesi/rollups-node/internal/repository/sqlite"
	//"github.com/cartesi/rollups-node/internal/repository/migration"
)

// NewRepositoryFromConnectionString chooses the backend based on the connection string.
// For instance:
//   - "postgres://user:pass@localhost/dbname" => Postgres
//   - "sqlite://some/path.db" => SQLite
//
// Then it initializes the repo, runs migrations, and returns it.
func NewRepositoryFromConnectionString(ctx context.Context, conn string) (Repository, error) {
	lowerConn := strings.ToLower(conn)
	switch {
	case strings.HasPrefix(lowerConn, "postgres://"):
		return newPostgresRepository(ctx, conn)
	// case strings.HasPrefix(lowerConn, "sqlite://"):
	// 	return newSQLiteRepository(ctx, conn)
	default:
		return nil, fmt.Errorf("unrecognized connection string format: %s", conn)
	}
}

func newPostgresRepository(ctx context.Context, conn string) (Repository, error) {
	pgRepo, err := postgres.NewPostgresRepository(ctx, conn, 5, 3*time.Second) // FIXME: get from config
	if err != nil {
		return nil, err
	}

	return pgRepo, nil
}

// func newSQLiteRepository(ctx context.Context, conn string) (Repository, error) {
// 	// Typically parse out the file from the "sqlite://somefile.db" connection string,
// 	// open database, etc.
// 	sqliteRepo, err := sqlite.NewSQLiteRepository(ctx, conn)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	// run migrations for SQLite, if applicable
// 	if err := migration.EnsureMigrationsSQLite(ctx, conn); err != nil {
// 		sqliteRepo.Close()
// 		return nil, err
// 	}
//
// 	return sqliteRepo, nil
// }

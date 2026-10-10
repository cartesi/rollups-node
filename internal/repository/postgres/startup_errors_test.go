// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestSchemaStartupErrorsKeepOnlySafeDetails(t *testing.T) {
	const marker = "SCHEMA_STARTUP_SECRET"
	mismatch := &schema.VersionMismatchError{Expected: 1, Actual: 2}
	for _, test := range []struct {
		name string
		err  error
		safe error
	}{
		{"server message", &pgconn.PgError{Severity: "ERROR", Code: "08006", Message: marker}, nil},
		{"dirty schema", fmt.Errorf("%s: %w", marker, schema.ErrMigrationNotCompleted), schema.ErrMigrationNotCompleted},
		{"missing schema", fmt.Errorf("%s: %w", marker, schema.ErrNoValidDatabaseSchema), schema.ErrNoValidDatabaseSchema},
		{"cancellation", fmt.Errorf("%s: %w", marker, context.Canceled), context.Canceled},
		{"deadline", fmt.Errorf("%s: %w", marker, context.DeadlineExceeded), context.DeadlineExceeded},
		{"version mismatch", fmt.Errorf("%s: %w", marker, mismatch), mismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := safeSchemaValidationError(test.err)
			require.Contains(t, err.Error(), "failed to validate Postgres schema version")
			for _, verb := range []string{"%v", "%+v", "%#v"} {
				require.NotContains(t, fmt.Sprintf(verb, err), marker)
			}
			if test.safe != nil {
				require.ErrorIs(t, err, test.safe)
				require.Contains(t, err.Error(), test.safe.Error())
			} else {
				require.EqualError(t, err, "failed to validate Postgres schema version")
			}
			var driver *pgconn.PgError
			require.False(t, errors.As(err, &driver), "raw server diagnostics must not remain reachable")
		})
	}
}

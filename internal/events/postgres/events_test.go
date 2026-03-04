package postgres

import (
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/events"
	contract_tests "github.com/cartesi/rollups-node/internal/events/tests"
	"github.com/cartesi/rollups-node/test/tooling/db"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	pingAttempts = 5
	pingInterval = 3 * time.Second
)

func setupNewDBPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	endpoint, err := db.GetTestDatabaseEndpoint()
	require.NoError(t, err)

	err = db.SetupTestPostgres(endpoint)
	require.NoError(t, err)

	config, err := pgxpool.ParseConfig(endpoint)
	require.NoError(t, err)

	ctx := t.Context()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)

	// Wait for database to be available
	for range pingAttempts {
		if err = pool.Ping(ctx); err == nil {
			break
		}
		select {
		case <-time.After(pingInterval):
		case <-ctx.Done():
			pool.Close()
			panic(ctx.Err())
		}
	}

	return pool
}

func TestPostgresEvents(t *testing.T) {
	suite.Run(t, contract_tests.NewPublisherTestSuite(func() events.Service {
		return NewPostgresEventsService(setupNewDBPool(t))
	}))
}

// TODO: Subscriber reconnection after connection loss

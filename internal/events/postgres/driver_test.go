package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/events"
	contract_tests "github.com/cartesi/rollups-node/internal/events/tests"
	"github.com/cartesi/rollups-node/test/tooling/db"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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

	return pool
}

func TestPostgresEventsContract(t *testing.T) {
	dbPool := setupNewDBPool(t)
	suite.Run(t, contract_tests.NewPublisherTestSuite(func() events.Service {
		return events.NewEventsService(NewDriver(dbPool))
	}))
	dbPool.Close()
}

func TestPostgresEventsService(t *testing.T) {

	ctx := t.Context()

	t.Run("StartErrorsOnImmediateProblem", func(t *testing.T) {
		t.Parallel()

		dbPool := setupNewDBPool(t)
		service := events.NewEventsService(NewDriver(dbPool))

		t.Log("Closing database pool")
		dbPool.Close()

		require.EqualError(t, service.Start(ctx), "closed pool")
	})

	t.Run("ReconnectOnClosedConnection", func(t *testing.T) {
		t.Parallel()

		dbPool := setupNewDBPool(t)
		driver := &pgDriver{dbPool: dbPool}
		service := events.NewEventsService(driver)

		require.NoError(t, service.Start(ctx))

		sub, err := service.Subscribe(ctx, events.SubscriptionFilter{})
		require.NoError(t, err)

		expected := events.Event{
			Type:      events.EventAppRegistered,
			AppID:     "echo-dapp",
			Payload:   json.RawMessage(`"Hello, World!"`),
			Timestamp: time.Now().Truncate(time.Second),
		}

		for i := range 2 {
			err = service.Publish(ctx, expected)
			require.NoError(t, err)

			actual := contract_tests.WaitEvent(t, sub.Channel())
			require.Equal(t, actual, expected)

			if i == 0 {
				t.Log("Closing database pool")
				driver.conn.Close(ctx)

				time.Sleep(1 * time.Second)
			}
		}

	})

}

// TODO: Subscriber reconnection after connection loss


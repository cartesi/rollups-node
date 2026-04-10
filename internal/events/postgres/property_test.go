// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/cartesi/rollups-node/internal/events/eventstest"
)

// TestPropertySuitePostgres runs the transport-dependent property tests
// (P1, P5, P6) against the PostgreSQL LISTEN/NOTIFY backend. These same
// tests also run against memory.Bus in internal/events/property_test.go.
//
// Requires CARTESI_TEST_DATABASE_CONNECTION to be set.
func TestPropertySuitePostgres(t *testing.T) {
	connStr := getTestConnString(t)

	suite.Run(t, &eventstest.PropertySuite{
		Factory: func(t *testing.T) (events.Publisher, events.Subscriber, <-chan struct{}) {
			ctx := context.Background()
			pool, err := pgxpool.New(ctx, connStr)
			require.NoError(t, err)
			t.Cleanup(pool.Close)

			ready := make(chan struct{}, 1)
			pub := NewPublisher(pool, slog.Default())
			t.Cleanup(func() { _ = pub.Close() })
			sub := NewSubscriber(connStr, slog.Default(), &SubscriberConfig{ReadySignal: ready})
			t.Cleanup(func() { _ = sub.Close() })

			return pub, sub, ready
		},
		SettleTime: 200 * time.Millisecond,
	})
}

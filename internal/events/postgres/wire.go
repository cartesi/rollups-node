// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cartesi/rollups-node/internal/events"
)

// WireResult holds the components created by Wire.
type WireResult struct {
	Publisher  events.Publisher
	Subscriber *Subscriber
	Signal     <-chan struct{}

	pub *Publisher
}

// StartListener starts the subscriber's Listen loop in a background goroutine.
// Call this after the service is created (so the context is available).
func (w *WireResult) StartListener(ctx context.Context) {
	go func() { _ = w.Subscriber.Listen(ctx) }()
}

func (w *WireResult) Close() error {
	var err error
	if w.pub != nil {
		err = w.pub.Close()
	}
	if w.Subscriber != nil {
		err = errors.Join(err, w.Subscriber.Close())
	}
	return err
}

// Wire creates a Publisher and Subscriber for PostgreSQL LISTEN/NOTIFY,
// subscribes to the given channels, and returns a coalesced signal channel.
//
// If eventsConnStr is empty, mainConnStr is used for the subscriber's LISTEN
// connection. This allows PgBouncer deployments to use a separate connection
// string for LISTEN (which requires session-mode pooling).
//
// The caller must:
//   - defer w.Close()
//   - call w.StartListener(ctx) after the service is created
func Wire(
	pool *pgxpool.Pool,
	mainConnStr string,
	eventsConnStr string,
	logger *slog.Logger,
	channels ...events.Channel,
) *WireResult {
	pub := NewPublisher(pool, logger)
	connStr := eventsConnStr
	if connStr == "" {
		connStr = mainConnStr
	}
	sub := NewSubscriber(connStr, logger, nil)
	notifCh := sub.Subscribe(channels...)
	signal := events.Coalesce(notifCh)
	return &WireResult{
		Publisher:  pub,
		Subscriber: sub,
		Signal:     signal,
		pub:        pub,
	}
}

// PoolFromRepository extracts the *pgxpool.Pool from a repository via the
// Pool() method. This avoids unsafe type assertions against concrete types.
//
// Returns an error if the repository does not expose a Pool() method.
// In practice, PostgreSQL is the only supported backend, so this always
// succeeds for production code.
func PoolFromRepository(repo any) (*pgxpool.Pool, error) {
	type poolProvider interface {
		Pool() *pgxpool.Pool
	}
	pp, ok := repo.(poolProvider)
	if !ok {
		return nil, fmt.Errorf("events: repository type %T does not provide a connection pool", repo)
	}
	return pp.Pool(), nil
}

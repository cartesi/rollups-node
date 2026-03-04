package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cartesi/rollups-node/internal/events"

	"github.com/jackc/pgx/v5/pgxpool"
)

const notifyChannel = "events"

type pgEventsService struct {
	pool *pgxpool.Pool
}

// NewPostgresPublisher returns a Publisher backed by a pgxpool.Pool.
func NewPostgresEventsService(pool *pgxpool.Pool) events.Service {
	return &pgEventsService{pool: pool}
}

func (s *pgEventsService) Publish(ctx context.Context, e events.Event) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	// ensure payload is valid JSON (could be nil)
	if e.Payload == nil {
		e.Payload = json.RawMessage([]byte("null"))
	}
	// notify listeners with the full event JSON payload
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	// send NOTIFY; Postgres limits payload size (~8000), keep that in mind
	_, err = s.pool.Exec(ctx, "SELECT pg_notify($1::text, $2::text)", notifyChannel, string(b))
	return err
}

func (s *pgEventsService) Subscribe(ctx context.Context, filter events.SubscriptionFilter) (<-chan events.Event, error) {
	acq, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	// start LISTEN on the dedicated connection
	conn := acq.Conn()
	if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		acq.Release()
		return nil, err
	}

	eventsCh := make(chan events.Event)

	go func() {
		// ensure release when context done or on error/return
		// we will release in the goroutine
		defer acq.Release()
		defer close(eventsCh)

		for {
			// Wait for a notification (blocks)
			notify, err := acq.Conn().WaitForNotification(ctx)
			if err != nil {
				// ctx cancelled -> normal shutdown
				if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled") {
					return
				}
				// return on other errors
				return
			}
			var ev events.Event
			if err := json.Unmarshal([]byte(notify.Payload), &ev); err != nil {
				// skip malformed payloads
				continue
			}
			if filter.Matches(ev) {
				select {
				case eventsCh <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return eventsCh, nil
}

func (s *pgEventsService) Close() {
	s.pool.Close()
}

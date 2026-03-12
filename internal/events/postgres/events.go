package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/cartesi/rollups-node/internal/events/notifier"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgEventsService struct {
	dbPool   *pgxpool.Pool
	notifier *notifier.Notifier
}

// NewPostgresPublisher returns a Publisher backed by a pgxpool.Pool.
func NewPostgresEventsService(pool *pgxpool.Pool) events.Service {
	listener := notifier.NewListener(pool)
	return &pgEventsService{
		dbPool:   pool,
		notifier: notifier.NewNotifier(listener),
	}
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
	_, err = s.dbPool.Exec(ctx, "SELECT pg_notify($1::text, $2::text)", string(e.Type), string(b))
	return err
}

func (s *pgEventsService) Subscribe(ctx context.Context, filter events.SubscriptionFilter) (<-chan events.Event, error) {
	err := s.notifier.Start(ctx)
	if err != nil {
		return nil, err
	}

	<-s.notifier.Started()

	notifyCh := make(chan events.Event)
	_, err = s.notifier.Listen(ctx, filter, func(event events.Event) { notifyCh <- event })
	if err != nil {
		close(notifyCh)
	}

	return notifyCh, err
}

func (s *pgEventsService) Close() {
	s.notifier.Stop()
	s.dbPool.Close()
}

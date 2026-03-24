package postgres

import (
	"context"
	"sync"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPublisher(dbPool *pgxpool.Pool) events.PublisherDriver {
	return &pgPublisher{dbPool: dbPool}
}

type pgPublisher struct {
	conn   *pgx.Conn
	dbPool *pgxpool.Pool
	mu     sync.Mutex
}

func (d *pgPublisher) Notify(ctx context.Context, n *events.Notification) error {
	// send NOTIFY; Postgres limits payload size (~8000), keep that in mind
	_, err := d.dbPool.Exec(ctx, "SELECT pg_notify($1::text, $2::text)", n.Topic, n.Payload)
	return err
}

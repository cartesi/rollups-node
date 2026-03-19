package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDriver(dbPool *pgxpool.Pool) events.Driver {
	return &pgDriver{dbPool: dbPool}
}

type pgDriver struct {
	conn   *pgx.Conn
	dbPool *pgxpool.Pool
	mu     sync.Mutex
}

func (d *pgDriver) Notify(ctx context.Context, n *events.Notification) error {
	// send NOTIFY; Postgres limits payload size (~8000), keep that in mind
	_, err := d.dbPool.Exec(ctx, "SELECT pg_notify($1::text, $2::text)", n.Topic, n.Payload)
	return err
}

func (d *pgDriver) Close(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn == nil {
		return nil
	}

	// Release below would take care of cleanup and potentially put the
	// connection back into rotation, but in case a Listen was invoked without a
	// subsequent Unlisten on the same topic, close the connection explicitly to
	// guarantee no other caller will receive a partially tainted connection.
	err := d.conn.Close(ctx)

	// Even in the event of an error, make sure conn is set back to nil so that
	// the listener can be reused.
	d.conn = nil

	return err
}

func (d *pgDriver) Connect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return errors.New("connection already established")
	}

	poolConn, err := d.dbPool.Acquire(ctx)
	if err != nil {
		return err
	}

	// Assume full ownership of the conn so that it doesn't get released back to
	// the pool or auto-closed by the pool.
	d.conn = poolConn.Hijack()

	return nil
}

func (d *pgDriver) Listen(ctx context.Context, topics []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(ctx, makeMultipleTopicSql("LISTEN", topics))
	return err
}

func (d *pgDriver) Ping(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.conn.Ping(ctx)
}

func (d *pgDriver) Unlisten(ctx context.Context, topics []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.conn.Exec(ctx, makeMultipleTopicSql("UNLISTEN", topics))
	return err
}

func (d *pgDriver) WaitForNotification(ctx context.Context) (*events.Notification, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	notification, err := d.conn.WaitForNotification(ctx)
	if err != nil {
		return nil, err
	}

	return &events.Notification{
		Topic:   notification.Channel,
		Payload: notification.Payload,
	}, nil
}

const sqlPiecesCount = 4

func makeMultipleTopicSql(cmd string, topics []string) string {
	var sb strings.Builder
	for _, topic := range topics {
		sb.WriteString(cmd)
		sb.WriteString(" ")
		sb.WriteString(pgx.Identifier{topic}.Sanitize())
		sb.WriteString(";\n")
	}
	return sb.String()
}

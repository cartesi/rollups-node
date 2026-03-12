package notifier

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewListener(dbPool *pgxpool.Pool) events.Listener {
	return &Listener{dbPool: dbPool}
}

type Listener struct {
	conn   *pgx.Conn
	dbPool *pgxpool.Pool
	mu     sync.Mutex
}

func (l *Listener) Close(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		return nil
	}

	// Release below would take care of cleanup and potentially put the
	// connection back into rotation, but in case a Listen was invoked without a
	// subsequent Unlisten on the same topic, close the connection explicitly to
	// guarantee no other caller will receive a partially tainted connection.
	err := l.conn.Close(ctx)

	// Even in the event of an error, make sure conn is set back to nil so that
	// the listener can be reused.
	l.conn = nil

	return err
}

func (l *Listener) Connect(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn != nil {
		return errors.New("connection already established")
	}

	poolConn, err := l.dbPool.Acquire(ctx)
	if err != nil {
		return err
	}

	// Assume full ownership of the conn so that it doesn't get released back to
	// the pool or auto-closed by the pool.
	l.conn = poolConn.Hijack()

	return nil
}

func (l *Listener) Listen(ctx context.Context, topics []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	_, err := l.conn.Exec(ctx, makeMultipleTopicSql("LISTEN", topics))
	return err
}

func (l *Listener) Ping(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.conn.Ping(ctx)
}

func (l *Listener) Unlisten(ctx context.Context, topics []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	_, err := l.conn.Exec(ctx, makeMultipleTopicSql("UNLISTEN", topics))
	return err
}

func (l *Listener) WaitForNotification(ctx context.Context) (*events.Notification, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	notification, err := l.conn.WaitForNotification(ctx)
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
	strList := make([]string, sqlPiecesCount*len(topics))
	for i, topic := range topics {
		baseIdx := i * sqlPiecesCount
		strList[baseIdx] = cmd
		strList[baseIdx+1] = " \""
		strList[baseIdx+2] = topic
		strList[baseIdx+3] = "\";\n"
	}
	return strings.Join(strList, "")
}

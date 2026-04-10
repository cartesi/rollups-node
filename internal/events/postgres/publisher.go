// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cartesi/rollups-node/internal/events"
)

const (
	// publishTimeout bounds how long the background worker can spend on one
	// pg_notify before dropping it and moving on.
	publishTimeout = 100 * time.Millisecond

	// publishQueueSize bounds memory usage and ensures Publish remains
	// non-blocking. Notifications are dropped when the queue is full.
	publishQueueSize = 64
)

type notifyFunc func(ctx context.Context, channel, payload string) error

type publishRequest struct {
	channel string
	payload string
	appID   int64
	epochID uint64
}

// Publisher sends advisory notifications via PostgreSQL pg_notify().
// Publish is fire-and-forget: notifications are enqueued onto a bounded
// in-memory queue and a background worker performs the blocking database call.
type Publisher struct {
	logger *slog.Logger
	queue  chan publishRequest
	stop   chan struct{}
	done   chan struct{}
	notify notifyFunc

	closeOnce sync.Once
}

func NewPublisher(pool *pgxpool.Pool, logger *slog.Logger) *Publisher {
	return newPublisher(logger, publishQueueSize, func(ctx context.Context, channel, payload string) error {
		_, err := pool.Exec(ctx, "SELECT pg_notify($1, $2)", channel, payload)
		return err
	})
}

func newPublisher(logger *slog.Logger, queueSize int, notify notifyFunc) *Publisher {
	p := &Publisher{
		logger: logger,
		queue:  make(chan publishRequest, queueSize),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		notify: notify,
	}
	go p.run()
	return p
}

func (p *Publisher) Publish(_ context.Context, n events.Notification) {
	payload, err := json.Marshal(n)
	if err != nil {
		p.logger.Warn("Failed to marshal notification",
			"channel", n.Channel, "error", err)
		return
	}

	req := publishRequest{
		channel: string(n.Channel),
		payload: string(payload),
		appID:   n.ApplicationID,
		epochID: n.EpochIndex,
	}

	select {
	case <-p.stop:
		p.logger.Warn("Dropped notification after publisher closed",
			"channel", n.Channel,
			"app_id", n.ApplicationID,
		)
	case p.queue <- req:
	default:
		p.logger.Warn("Dropped notification because publisher queue is full",
			"channel", n.Channel,
			"app_id", n.ApplicationID,
		)
	}
}

func (p *Publisher) Close() error {
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done
	})
	return nil
}

func (p *Publisher) run() {
	defer close(p.done)
	for {
		select {
		case <-p.stop:
			return
		default:
		}

		select {
		case <-p.stop:
			return
		case req := <-p.queue:
			p.publish(req)
		}
	}
}

func (p *Publisher) publish(req publishRequest) {
	execCtx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()

	err := p.notify(execCtx, req.channel, req.payload)
	if err != nil {
		p.logger.Warn("Failed to publish notification",
			"channel", req.channel,
			"app_id", req.appID,
			"error", err,
		)
		return
	}

	p.logger.Debug("Published notification",
		"channel", req.channel,
		"app_id", req.appID,
		"epoch_idx", req.epochID,
	)
}

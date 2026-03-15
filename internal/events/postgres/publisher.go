// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cartesi/rollups-node/internal/events"
)

const publishTimeout = 5 * time.Second

// Publisher sends advisory notifications via PostgreSQL pg_notify().
// It uses the shared pgxpool.Pool for auto-committed NOTIFY statements.
type Publisher struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewPublisher(pool *pgxpool.Pool, logger *slog.Logger) *Publisher {
	return &Publisher{pool: pool, logger: logger}
}

func (p *Publisher) Publish(ctx context.Context, n events.Notification) {
	payload, err := json.Marshal(n)
	if err != nil {
		p.logger.Warn("Failed to marshal notification",
			"channel", n.Channel, "error", err)
		return
	}

	execCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	_, err = p.pool.Exec(execCtx,
		"SELECT pg_notify($1, $2)",
		string(n.Channel), string(payload))
	if err != nil {
		p.logger.Warn("Failed to publish notification",
			"channel", n.Channel,
			"app_id", n.ApplicationID,
			"error", err,
		)
	}
}

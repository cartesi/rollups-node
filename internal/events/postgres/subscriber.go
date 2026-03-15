// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/events"
)

const (
	defaultBufferSize   = 64
	reconnectBaseDelay  = 500 * time.Millisecond
	reconnectMaxDelay   = 30 * time.Second
	reconnectMultiplier = 2
	heartbeatTimeout    = 60 * time.Second
	heartbeatQuery      = "SELECT 1"
	jitterLow           = 0.75
	jitterRange         = 0.5
)

// Subscriber receives advisory notifications from PostgreSQL LISTEN.
// It manages a dedicated pgx.Conn (not from the pool) with automatic
// reconnection, heartbeat, and non-blocking delivery.
type Subscriber struct {
	connString string
	logger     *slog.Logger
	bufferSize int

	mu       sync.Mutex
	channels []events.Channel
	ch       chan events.Notification
	closed   bool
}

func NewSubscriber(connString string, logger *slog.Logger) *Subscriber {
	return &Subscriber{
		connString: connString,
		logger:     logger,
		bufferSize: defaultBufferSize,
	}
}

func (s *Subscriber) Subscribe(channels ...events.Channel) <-chan events.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range channels {
		if err := events.ValidateChannel(ch); err != nil {
			panic(fmt.Sprintf("events: invalid channel in Subscribe: %s", ch))
		}
	}
	s.channels = append(s.channels, channels...)
	if s.ch == nil {
		s.ch = make(chan events.Notification, s.bufferSize)
	}
	return s.ch
}

func (s *Subscriber) Listen(ctx context.Context) error {
	s.mu.Lock()
	channels := make([]events.Channel, len(s.channels))
	copy(channels, s.channels)
	ch := s.ch
	s.mu.Unlock()

	if ch == nil {
		return fmt.Errorf("events: Listen called without Subscribe")
	}
	if len(channels) == 0 {
		return fmt.Errorf("events: no channels subscribed")
	}

	defer s.closeChan()

	delay := reconnectBaseDelay
	for {
		err := s.listenLoop(ctx, channels, ch)
		if ctx.Err() != nil {
			return nil
		}
		s.logger.Warn("Event listener disconnected, reconnecting",
			"error", err, "delay", delay)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(jitter(delay)):
		}
		delay = min(delay*reconnectMultiplier, reconnectMaxDelay)
	}
}

func (s *Subscriber) listenLoop(
	ctx context.Context,
	channels []events.Channel,
	ch chan events.Notification,
) error {
	connConfig, err := pgx.ParseConfig(s.connString)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	connConfig.RuntimeParams["application_name"] = "rollups-node-events"
	// TCP keepalives are enabled by default in Go's net.Dialer.
	// Stale connection detection relies on the application-level heartbeat
	// (WaitForNotification timeout + SELECT 1) defined in heartbeatTimeout.

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.Background())

	for _, channel := range channels {
		_, err := conn.Exec(ctx,
			"LISTEN "+pgx.Identifier{string(channel)}.Sanitize())
		if err != nil {
			return fmt.Errorf("listen %s: %w", channel, err)
		}
	}

	s.logger.Info("Event listener connected", "channels", channels)

	for {
		waitCtx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
		notification, err := conn.WaitForNotification(waitCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return nil // parent context canceled: clean shutdown
			}
			if errors.Is(err, context.DeadlineExceeded) {
				if _, pingErr := conn.Exec(ctx, heartbeatQuery); pingErr != nil {
					return fmt.Errorf("heartbeat failed: %w", pingErr)
				}
				continue
			}
			return fmt.Errorf("wait: %w", err)
		}

		var n events.Notification
		if err := json.Unmarshal([]byte(notification.Payload), &n); err != nil {
			s.logger.Warn("Failed to unmarshal notification",
				"channel", notification.Channel,
				"payload", notification.Payload,
				"error", err,
			)
			continue
		}

		select {
		case ch <- n:
		default:
			s.logger.Debug("Notification buffer full, dropping",
				"channel", n.Channel,
				"app_id", n.ApplicationID,
			)
		}
	}
}

func (s *Subscriber) closeChan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && s.ch != nil {
		close(s.ch)
		s.closed = true
	}
}

func (s *Subscriber) Close() error {
	s.closeChan()
	return nil
}

func jitter(d time.Duration) time.Duration {
	// +/- 25% jitter
	factor := jitterLow + rand.Float64()*jitterRange //nolint:gosec
	return time.Duration(float64(d) * factor)
}

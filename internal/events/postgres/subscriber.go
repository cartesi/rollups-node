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
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/events"
)

const (
	defaultBufferSize       = 64
	reconnectBaseDelay      = 500 * time.Millisecond
	reconnectMaxDelay       = 30 * time.Second
	reconnectMultiplier     = 2
	maxAttemptsBeforeReset  = 7
	defaultHeartbeatTimeout = 60 * time.Second
	heartbeatQuery          = "SELECT 1"
	jitterLow               = 0.75
	jitterRange             = 0.5
)

// SubscriberConfig allows tuning the subscriber for different deployments.
type SubscriberConfig struct {
	// HeartbeatTimeout is the maximum time to wait for a notification before
	// sending a health-check query on the LISTEN connection. A shorter timeout
	// detects dead connections faster but generates more health-check traffic.
	// Default: 60s. Set lower for local development, higher for production.
	HeartbeatTimeout time.Duration

	// BufferSize is the capacity of the notification delivery channel.
	// When the buffer is full, new notifications are dropped (fire-and-forget).
	// Default: 64. Increase for systems with many applications where a single
	// block could generate notifications for all of them.
	BufferSize int

	// ReadySignal, if non-nil, receives a value each time the subscriber
	// successfully connects and issues LISTEN commands. This enables tests
	// to wait for readiness without time.Sleep.
	ReadySignal chan<- struct{}
}

// subscription holds the delivery channel, subscribed channels, and optional
// filter for one Subscribe or SubscribeWithFilter call.
type subscription struct {
	ch       chan events.Notification
	channels map[events.Channel]struct{} // channels this subscription cares about
	filter   *events.SubscriptionFilter  // nil means no app filter (deliver all)
}

// Subscriber receives advisory notifications from PostgreSQL LISTEN.
// It manages a dedicated pgx.Conn (not from the pool) with automatic
// reconnection, heartbeat, and non-blocking delivery.
type Subscriber struct {
	connString       string
	logger           *slog.Logger
	bufferSize       int
	heartbeatTimeout time.Duration
	readySignal      chan<- struct{}

	mu            sync.Mutex
	channels      []events.Channel
	subscriptions []subscription
	closed        bool
	subsClosed    bool

	connMu   sync.Mutex
	conn     *pgx.Conn
	closeCh  chan struct{}
	listenWg sync.WaitGroup
}

func NewSubscriber(connString string, logger *slog.Logger, cfg *SubscriberConfig) *Subscriber {
	heartbeat := defaultHeartbeatTimeout
	bufSize := defaultBufferSize
	var readySig chan<- struct{}
	if cfg != nil {
		if cfg.HeartbeatTimeout > 0 {
			heartbeat = cfg.HeartbeatTimeout
		}
		if cfg.BufferSize > 0 {
			bufSize = cfg.BufferSize
		}
		readySig = cfg.ReadySignal
	}
	return &Subscriber{
		connString:       connString,
		logger:           logger,
		bufferSize:       bufSize,
		heartbeatTimeout: heartbeat,
		readySignal:      readySig,
		closeCh:          make(chan struct{}),
	}
}

func (s *Subscriber) Subscribe(channels ...events.Channel) <-chan events.Notification {
	return s.SubscribeWithFilter(events.SubscriptionFilter{}, channels...)
}

func (s *Subscriber) SubscribeWithFilter(
	filter events.SubscriptionFilter,
	channels ...events.Channel,
) <-chan events.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range channels {
		if err := events.ValidateChannel(c); err != nil {
			panic(fmt.Sprintf("events: invalid channel in Subscribe: %s", c))
		}
		if !slices.Contains(s.channels, c) {
			s.channels = append(s.channels, c)
		}
	}
	ch := make(chan events.Notification, s.bufferSize)
	f := filter // copy
	chSet := make(map[events.Channel]struct{}, len(channels))
	for _, c := range channels {
		chSet[c] = struct{}{}
	}
	s.subscriptions = append(s.subscriptions, subscription{ch: ch, channels: chSet, filter: &f})
	return ch
}

func (s *Subscriber) Listen(ctx context.Context) error {
	if s.isClosed() {
		return nil
	}

	s.mu.Lock()
	channels := make([]events.Channel, len(s.channels))
	copy(channels, s.channels)
	subs := make([]subscription, len(s.subscriptions))
	copy(subs, s.subscriptions)
	s.mu.Unlock()

	if len(subs) == 0 {
		return fmt.Errorf("events: Listen called without Subscribe")
	}
	if len(channels) == 0 {
		return fmt.Errorf("events: no channels subscribed")
	}

	s.listenWg.Add(1)
	defer s.listenWg.Done()

	delay := reconnectBaseDelay
	attempt := 0
	for {
		err := s.listenLoop(ctx, channels, subs)
		if ctx.Err() != nil || s.isClosed() {
			return nil
		}
		attempt++
		if attempt >= maxAttemptsBeforeReset {
			attempt = 0
			delay = reconnectBaseDelay
		}
		s.logger.Warn("Event listener disconnected, reconnecting",
			"error", err, "delay", delay, "attempt", attempt)
		select {
		case <-ctx.Done():
			return nil
		case <-s.closeCh:
			return nil
		case <-time.After(jitter(delay)):
		}
		delay = min(delay*reconnectMultiplier, reconnectMaxDelay)
	}
}

func (s *Subscriber) listenLoop(
	ctx context.Context,
	channels []events.Channel,
	subs []subscription,
) error {
	connConfig, err := pgx.ParseConfig(s.connString)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	connConfig.RuntimeParams["application_name"] = "rollups-node-events"

	// TCP keepalives provide OS-level stale connection detection.
	// A LISTEN connection sits idle for long periods, making it vulnerable to
	// half-open TCP states and silent firewall drops. These probes detect dead
	// connections within 90s (60s idle + 3 probes x 10s), complementing the
	// application-level heartbeat (WaitForNotification timeout + SELECT 1).
	//
	// Note: keepalive params are libpq connection-time parameters, not PostgreSQL
	// GUC variables. They must be configured via DialFunc, not RuntimeParams
	// (which are sent as SET commands and rejected by the server).
	connConfig.DialFunc = (&net.Dialer{
		KeepAlive: 60 * time.Second,
	}).DialContext

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	s.setConn(conn)
	defer s.clearConn(conn)
	defer conn.Close(context.Background())

	// Issue all LISTEN commands in a single Exec call to reduce
	// network round-trips (1 round-trip instead of N).
	if err := listenAll(ctx, conn, channels); err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.logger.Info("Event listener connected", "channels", channels)
	if s.readySignal != nil {
		select {
		case s.readySignal <- struct{}{}:
		default:
		}
	}

	for {
		waitCtx, cancel := context.WithTimeout(ctx, s.heartbeatTimeout)
		notification, err := conn.WaitForNotification(waitCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil || s.isClosed() {
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

		s.logger.Debug("Received notification",
			"channel", n.Channel,
			"app_id", n.ApplicationID,
			"epoch_idx", n.EpochIndex,
		)

		// Deliver to all matching subscriptions.
		for i := range subs {
			if _, ok := subs[i].channels[n.Channel]; !ok {
				continue
			}
			if subs[i].filter != nil && !subs[i].filter.Matches(n) {
				continue
			}
			select {
			case subs[i].ch <- n:
			default:
				s.logger.Debug("Notification buffer full, dropping",
					"channel", n.Channel,
					"app_id", n.ApplicationID,
				)
			}
		}
	}
}

func (s *Subscriber) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subsClosed {
		return
	}
	s.subsClosed = true
	for i := range s.subscriptions {
		close(s.subscriptions[i].ch)
	}
}

func (s *Subscriber) Close() error {
	s.markClosed()
	s.closeCurrentConn()
	s.listenWg.Wait()
	s.closeAll()
	return nil
}

func (s *Subscriber) setConn(conn *pgx.Conn) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.closed {
		_ = conn.Close(context.Background())
		return
	}
	s.conn = conn
}

func (s *Subscriber) clearConn(conn *pgx.Conn) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.conn == conn {
		s.conn = nil
	}
}

func (s *Subscriber) closeCurrentConn() {
	s.connMu.Lock()
	conn := s.conn
	s.connMu.Unlock()
	if conn != nil {
		_ = conn.Close(context.Background())
	}
}

func (s *Subscriber) markClosed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.closeCh)
}

func (s *Subscriber) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func jitter(d time.Duration) time.Duration {
	// +/- 25% jitter
	factor := jitterLow + rand.Float64()*jitterRange //nolint:gosec
	return time.Duration(float64(d) * factor)
}

// listenAll issues all LISTEN commands in a single Exec call,
// reducing N round-trips to 1. Each channel name is sanitized via
// pgx.Identifier to prevent SQL injection.
func listenAll(ctx context.Context, conn *pgx.Conn, channels []events.Channel) error {
	var sb strings.Builder
	for _, ch := range channels {
		sb.WriteString("LISTEN ")
		sb.WriteString(pgx.Identifier{string(ch)}.Sanitize())
		sb.WriteString(";\n")
	}
	_, err := conn.Exec(ctx, sb.String())
	return err
}

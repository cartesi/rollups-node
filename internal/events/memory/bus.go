// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package memory

import (
	"context"
	"log/slog"
	"sync"

	"github.com/cartesi/rollups-node/internal/events"
)

const defaultBufferSize = 64

// subscription holds a delivery channel and optional filter.
type subscription struct {
	ch     chan events.Notification
	filter *events.SubscriptionFilter
}

// Bus is an in-memory event bus implementing both Publisher and Subscriber.
// It provides the same fire-and-forget semantics as the PostgreSQL backend.
// Used in standalone mode (single process) and tests.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[events.Channel][]subscription
	bufferSize  int
	closed      bool
	logger      *slog.Logger
}

func NewBus(bufferSize int) *Bus {
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}
	return &Bus{
		subscribers: make(map[events.Channel][]subscription),
		bufferSize:  bufferSize,
	}
}

// SetLogger enables debug logging for publish and delivery events.
// When nil (the default), the bus is silent.
func (b *Bus) SetLogger(logger *slog.Logger) {
	b.logger = logger
}

func (b *Bus) Publish(_ context.Context, n events.Notification) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, sub := range b.subscribers[n.Channel] {
		if sub.filter != nil && !sub.filter.Matches(n) {
			continue
		}
		select {
		case sub.ch <- n:
		default:
			// Drop: same semantics as PostgreSQL backend.
			if b.logger != nil {
				b.logger.Debug("Notification buffer full, dropping",
					"channel", n.Channel,
					"app_id", n.ApplicationID,
				)
			}
		}
	}
	if b.logger != nil {
		b.logger.Debug("Published notification",
			"channel", n.Channel,
			"app_id", n.ApplicationID,
			"epoch_idx", n.EpochIndex,
		)
	}
}

func (b *Bus) Subscribe(channels ...events.Channel) <-chan events.Notification {
	return b.SubscribeWithFilter(events.SubscriptionFilter{}, channels...)
}

func (b *Bus) SubscribeWithFilter(
	filter events.SubscriptionFilter,
	channels ...events.Channel,
) <-chan events.Notification {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan events.Notification, b.bufferSize)
	f := filter // copy
	sub := subscription{ch: ch, filter: &f}
	for _, channel := range channels {
		b.subscribers[channel] = append(b.subscribers[channel], sub)
	}
	return ch
}

func (b *Bus) Listen(ctx context.Context) error {
	<-ctx.Done()
	_ = b.Close()
	return nil
}

func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	// A single Go channel may be registered under multiple map keys
	// (when Subscribe is called with multiple channels). Deduplicate
	// to avoid closing the same Go channel twice.
	closed := make(map[chan events.Notification]struct{})
	for channel, subs := range b.subscribers {
		for _, sub := range subs {
			if _, ok := closed[sub.ch]; !ok {
				close(sub.ch)
				closed[sub.ch] = struct{}{}
			}
		}
		delete(b.subscribers, channel)
	}
	return nil
}

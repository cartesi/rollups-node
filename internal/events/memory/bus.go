// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package memory

import (
	"context"
	"sync"

	"github.com/cartesi/rollups-node/internal/events"
)

const defaultBufferSize = 64

// Bus is an in-memory event bus implementing both Publisher and Subscriber.
// It provides the same fire-and-forget semantics as the PostgreSQL backend.
// Used in standalone mode (single process) and tests.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[events.Channel][]chan events.Notification
	bufferSize  int
	closed      bool
}

func NewBus(bufferSize int) *Bus {
	if bufferSize <= 0 {
		bufferSize = defaultBufferSize
	}
	return &Bus{
		subscribers: make(map[events.Channel][]chan events.Notification),
		bufferSize:  bufferSize,
	}
}

func (b *Bus) Publish(_ context.Context, n events.Notification) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, ch := range b.subscribers[n.Channel] {
		select {
		case ch <- n:
		default:
			// Drop: same semantics as PostgreSQL backend.
		}
	}
}

func (b *Bus) Subscribe(channels ...events.Channel) <-chan events.Notification {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan events.Notification, b.bufferSize)
	for _, channel := range channels {
		b.subscribers[channel] = append(b.subscribers[channel], ch)
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
	for channel, chans := range b.subscribers {
		for _, ch := range chans {
			if _, ok := closed[ch]; !ok {
				close(ch)
				closed[ch] = struct{}{}
			}
		}
		delete(b.subscribers, channel)
	}
	return nil
}

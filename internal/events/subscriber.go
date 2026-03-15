// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package events

import "context"

// Subscriber receives advisory notifications for subscribed channels.
type Subscriber interface {
	// Subscribe registers interest in the given channels and returns
	// a channel that delivers notifications. The channel is closed when
	// the context is canceled or Close is called.
	//
	// The returned channel has a bounded buffer (default: 64).
	// When the buffer is full, new notifications are dropped (not blocked).
	//
	// Subscribe MUST be called before Listen. Calling Subscribe after
	// Listen has started is a programming error.
	Subscribe(channels ...Channel) <-chan Notification

	// Listen begins receiving notifications. It blocks until ctx is
	// canceled or a fatal error occurs. On transient errors (connection
	// loss), Listen reconnects automatically and re-issues LISTEN commands.
	//
	// The caller must have called Subscribe first.
	Listen(ctx context.Context) error

	// Close releases resources. After Close, all channels returned
	// by Subscribe are closed.
	Close() error
}

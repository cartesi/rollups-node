// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package events

import "context"

// Publisher sends advisory notifications. Implementations must be safe
// for concurrent use. Publish is fire-and-forget: errors are logged
// internally, not returned, because a failed notification only means
// the consumer will discover work on its next poll cycle.
type Publisher interface {
	// Publish sends an advisory notification on the given channel.
	// It MUST NOT block. If the underlying transport is unavailable,
	// the notification is silently dropped and logged.
	Publish(ctx context.Context, n Notification)
}

// NopPublisher discards all events. Use as the default when events
// are not configured.
type NopPublisher struct{}

func (NopPublisher) Publish(context.Context, Notification) {}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package eventstest

import (
	"context"
	"sync"

	"github.com/cartesi/rollups-node/internal/events"
)

// Recorder is a Publisher that captures events for test assertions.
// It is safe for concurrent use.
type Recorder struct {
	mu         sync.Mutex
	recordings []events.Notification
}

func (r *Recorder) Publish(_ context.Context, n events.Notification) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordings = append(r.recordings, n)
}

// Events returns a copy of all captured notifications.
func (r *Recorder) Events() []events.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]events.Notification, len(r.recordings))
	copy(result, r.recordings)
	return result
}

// EventsForChannel returns captured notifications filtered by channel.
func (r *Recorder) EventsForChannel(ch events.Channel) []events.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []events.Notification
	for _, e := range r.recordings {
		if e.Channel == ch {
			result = append(result, e)
		}
	}
	return result
}

// Reset clears all captured events.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordings = r.recordings[:0]
}

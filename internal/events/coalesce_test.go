// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoalesceDeliversSignal(t *testing.T) {
	in := make(chan Notification, 1)
	out := Coalesce(in)

	in <- Notification{Channel: ChannelInputReceived, ApplicationID: 1}

	select {
	case _, ok := <-out:
		require.True(t, ok, "signal channel should be open")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func TestCoalesceCollapsesMultipleNotifications(t *testing.T) {
	in := make(chan Notification, 64)
	out := Coalesce(in)

	// Send 50 rapid notifications.
	for i := range 50 {
		in <- Notification{Channel: ChannelInputReceived, ApplicationID: int64(i)}
	}

	// Give the coalesce goroutine time to drain.
	time.Sleep(50 * time.Millisecond)

	// Drain whatever is in the output channel.
	count := 0
	for {
		select {
		case <-out:
			count++
		default:
			goto done
		}
	}
done:
	// At most 1 signal should be pending (buffer size is 1).
	assert.Equal(t, 1, count, "coalesce should collapse 50 notifications into 1 signal")
}

func TestCoalesceClosesOnInputClose(t *testing.T) {
	in := make(chan Notification)
	out := Coalesce(in)

	close(in)

	select {
	case _, ok := <-out:
		assert.False(t, ok, "output should be closed when input is closed")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for close propagation")
	}
}

func TestCoalesceCleanExitOnInputClose(_ *testing.T) {
	in := make(chan Notification, 1)
	out := Coalesce(in)

	in <- Notification{Channel: ChannelEpochClosed, ApplicationID: 42, EpochIndex: 7}
	close(in)

	// Drain all signals until closed. If the goroutine exits cleanly,
	// this loop terminates.
	for range out { //nolint:revive
	}
}

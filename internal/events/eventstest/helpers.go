// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package eventstest

import (
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/events"
)

// WaitForNotification reads a single notification from ch within the given
// timeout. It calls t.Fatal if the timeout expires.
func WaitForNotification(
	t *testing.T,
	ch <-chan events.Notification,
	timeout time.Duration,
) events.Notification {
	t.Helper()
	select {
	case n, ok := <-ch:
		if !ok {
			t.Fatal("notification channel closed unexpectedly")
		}
		return n
	case <-time.After(timeout):
		t.Fatal("timed out waiting for notification")
		return events.Notification{} // unreachable
	}
}

// DrainChannel reads all buffered notifications from ch without blocking.
func DrainChannel(ch <-chan events.Notification) []events.Notification {
	var result []events.Notification
	for {
		select {
		case n, ok := <-ch:
			if !ok {
				return result
			}
			result = append(result, n)
		default:
			return result
		}
	}
}

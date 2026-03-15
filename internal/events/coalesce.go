// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package events

// Coalesce wraps a Notification channel into a wake-up signal.
// Multiple rapid notifications collapse into a single signal.
// The returned channel has a buffer of 1; at most one wake-up
// is pending at any time.
//
// The returned channel is closed when the input channel is closed.
func Coalesce(notifications <-chan Notification) <-chan struct{} {
	signal := make(chan struct{}, 1)
	go func() {
		defer close(signal)
		for range notifications {
			select {
			case signal <- struct{}{}:
			default:
				// Already signaled; collapse.
			}
		}
	}()
	return signal
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package util

import "time"

// Implments a simple method call retry policy.
// This retry policy will retry a failed execution for up to 'maxRetries' times
// In between retry calls it will wait a "maxDelay"
func CallFunctionWithRetryPolicy[
	R any,
	A any,
](
	fn func(A) (R, error),
	args A,
	maxRetries uint,
	maxDelay time.Duration,
) (R, error) {

	var lastErr error
	var lastalue R
	for i := uint(0); i <= maxRetries; i++ {
		lastalue, lastErr = fn(args)
		if lastErr == nil {
			return lastalue, nil
		}

		time.Sleep(maxDelay)
	}
	return lastalue, lastErr

}

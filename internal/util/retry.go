// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package util

import (
	"context"
	"log/slog"
	"reflect"
	"runtime"
	"time"
)

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
	var funcName string
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		funcName = getFunctionName(fn)
	}
	for i := uint(0); i <= maxRetries; i++ {
		lastalue, lastErr = fn(args)
		if lastErr == nil {
			return lastalue, nil
		}
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			slog.Debug(lastErr.Error())
			slog.Debug("Retrying '" + funcName + "'")
		}
		time.Sleep(maxDelay)
	}
	return lastalue, lastErr

}

func getFunctionName(fn interface{}) string {
	function := runtime.FuncForPC(
		reflect.ValueOf(fn).Pointer(),
	)
	if function != nil {
		return function.Name()
	}
	return "N/A"
}

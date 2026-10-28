// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShutdownOnlyRequiresEveryCauseToBeCancellationOrWait(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	failure := errors.New("database connection failed")
	extraWait := errors.New("protocol window is stale")
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"pure cancellation", context.Canceled, true},
		{"ordinary wait", ErrInconclusive, true},
		{"named wait", extraWait, true},
		{"wrapped waits and cancellation", fmt.Errorf("pass: %w", errors.Join(ErrInconclusive, ErrLimit, context.Canceled)), true},
		{"nested cancellations", errors.Join(context.Canceled, fmt.Errorf("read: %w", context.Canceled)), true},
		{"cancelled POST", &PublicationError{Class: Uncertain, Err: context.Canceled}, true},
		{"cancelled POST with real failure", &PublicationError{Class: Uncertain,
			Err: errors.Join(context.Canceled, failure)}, false},
		{"real failure", failure, false},
		{"mixed failure", errors.Join(ErrInconclusive, context.Canceled, failure), false},
		{"deadline", context.DeadlineExceeded, false},
		{"mixed deadline", errors.Join(context.Canceled, context.DeadlineExceeded), false},
		{"finality contradiction", errors.Join(context.Canceled, ErrFinality), false},
		{"content contradiction", errors.Join(context.Canceled, ErrContent), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, ShutdownOnly(ctx, test.err, ErrLimit, extraWait))
			require.False(t, ShutdownOnly(t.Context(), test.err, ErrLimit, extraWait),
				"cancellation without service shutdown is an operational failure")
		})
	}
}

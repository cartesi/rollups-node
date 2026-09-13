// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package errutil

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOnlyCancellation(t *testing.T) {
	other := errors.New("database unavailable")
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled", context.Canceled, true},
		{"wrapped", fmt.Errorf("query: %w", context.Canceled), true},
		{"joined", errors.Join(context.Canceled, fmt.Errorf("write: %w", context.Canceled)), true},
		{"multiple wrapped", fmt.Errorf("queries: %w; %w", context.Canceled, context.Canceled), true},
		{"nested", fmt.Errorf("query: %w", errors.Join(context.Canceled, context.Canceled)), true},
		{"deadline", context.DeadlineExceeded, false},
		{"other", other, false},
		{"mixed", errors.Join(context.Canceled, other), false},
		{"mixed reversed", errors.Join(other, context.Canceled), false},
		{"mixed deadline", errors.Join(context.Canceled, context.DeadlineExceeded), false},
		{"nested mixed", fmt.Errorf("query: %w", errors.Join(context.Canceled, other)), false},
		{"empty causes", &cancellationTestError{}, false},
		{"nil cause", &cancellationTestError{causes: []error{nil}}, false},
		{"nil wrapped cause", fmt.Errorf("query: %w", nil), false},
		{"custom cancellation leaf", &cancellationTestError{canceled: true}, true},
		{"custom cancellation with failure", &cancellationTestError{canceled: true, causes: []error{other}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, IsOnlyCancellation(test.err))
		})
	}
}

type cancellationTestError struct {
	causes   []error
	canceled bool
}

func (*cancellationTestError) Error() string     { return "test dependency error" }
func (e *cancellationTestError) Unwrap() []error { return e.causes }
func (e *cancellationTestError) Is(target error) bool {
	return e.canceled && target == context.Canceled
}

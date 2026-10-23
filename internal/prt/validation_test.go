// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/stretchr/testify/require"
	"log/slog"
	"strings"
	"testing"
)

func TestLogErrorUnlessShutdown(t *testing.T) {
	tests := []struct {
		name      string
		stopping  bool
		err       error
		wantError bool
	}{
		{name: "ShutdownCancellation", stopping: true, err: context.Canceled, wantError: false},
		{name: "ShutdownWaitAndCancellation", stopping: true,
			err: errors.Join(txsender.ErrInconclusive, context.Canceled), wantError: false},
		{name: "ShutdownWaitAndDatabaseFailure", stopping: true,
			err: errors.Join(txsender.ErrInconclusive, context.Canceled, errors.New("database unavailable")), wantError: true},
		{name: "ShutdownDeadline", stopping: true, err: context.DeadlineExceeded, wantError: true},
		{
			name:      "ShutdownCancellationWithDeadline",
			stopping:  true,
			err:       errors.Join(context.Canceled, context.DeadlineExceeded),
			wantError: true,
		},
		{name: "RuntimeCancellation", stopping: false, err: context.Canceled, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			s := &Service{
				TickServiceTemplate: service.TickServiceTemplate{
					BaseTemplate: service.BaseTemplate{
						Logger: slog.New(slog.NewTextHandler(&output, nil)),
					},
				},
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.stopping {
				cancel()
			}

			s.logErrorUnlessShutdown(ctx, "operation failed", test.err, "operation", "test")

			hasError := strings.Contains(output.String(), "level=ERROR")
			require.Equal(t, test.wantError, hasError, output.String())
		})
	}
}

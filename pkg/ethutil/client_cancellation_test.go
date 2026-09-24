// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPFailureLogLevel(t *testing.T) {
	transportErr := errors.New("connection reset")
	for _, test := range []struct {
		name  string
		err   error
		level slog.Level
	}{
		{name: "cancellation", err: context.Canceled, level: slog.LevelDebug},
		{name: "wrapped cancellation", err: fmt.Errorf("request: %w", context.Canceled), level: slog.LevelDebug},
		{name: "joined cancellations", err: errors.Join(context.Canceled, context.Canceled), level: slog.LevelDebug},
		{name: "deadline", err: context.DeadlineExceeded, level: slog.LevelError},
		{name: "transport failure", err: transportErr, level: slog.LevelError},
		{name: "cancellation text is not a cause", err: errors.New("context canceled"), level: slog.LevelError},
		{name: "mixed failure", err: errors.Join(context.Canceled, transportErr), level: slog.LevelError},
		{name: "mixed deadline", err: errors.Join(context.Canceled, context.DeadlineExceeded), level: slog.LevelError},
	} {
		t.Run(test.name, func(t *testing.T) {
			const endpoint = "https://rpc.example.test/secret-key"
			var logs bytes.Buffer
			logger := newRedactedLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), endpoint)
			logger.Error("request failed", "url", endpoint, "error", &url.Error{Op: "Post", URL: endpoint, Err: test.err})
			require.Contains(t, logs.String(), `"level":"`+test.level.String()+`"`)
			require.Equal(t, 1, strings.Count(logs.String(), `"msg":"request failed"`))
			require.Contains(t, logs.String(), "https://rpc.example.test")
			require.NotContains(t, logs.String(), "secret-key")
			if errors.Is(test.err, transportErr) {
				require.Contains(t, logs.String(), transportErr.Error())
			}
			if errors.Is(test.err, context.DeadlineExceeded) {
				require.Contains(t, logs.String(), context.DeadlineExceeded.Error())
			}
		})
	}
}

func TestNewEthClientCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		cancel()
		<-r.Context().Done()
	}))
	defer server.Close()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	// The request context, not the context used to construct the client, is canceled.
	client, err := NewEthClient(t.Context(), server.URL, logger, RetryConfig{RequestTimeout: time.Second})
	require.NoError(t, err)
	defer client.Close()
	_, err = client.ChainID(ctx)
	require.ErrorIs(t, err, context.Canceled, "cancellation must still reach the caller")
	require.NotContains(t, logs.String(), `"level":"ERROR"`)
	require.Contains(t, logs.String(), `"msg":"request failed"`)
	require.Contains(t, logs.String(), context.Canceled.Error())
}

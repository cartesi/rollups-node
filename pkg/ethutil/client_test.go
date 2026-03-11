// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactURLString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips path with API key",
			input:    "https://eth-mainnet.g.alchemy.com/v2/abcdef123456",
			expected: "https://eth-mainnet.g.alchemy.com",
		},
		{
			name:     "strips query params",
			input:    "https://mainnet.infura.io/v3/key123?project=secret",
			expected: "https://mainnet.infura.io",
		},
		{
			name:     "preserves scheme and host with port",
			input:    "http://localhost:8545/rpc",
			expected: "http://localhost:8545",
		},
		{
			name:     "handles plain host URL",
			input:    "http://localhost:8545",
			expected: "http://localhost:8545",
		},
		{
			name:     "redacts bare path without scheme",
			input:    "/v2/secret-key",
			expected: "[REDACTED]",
		},
		{
			name:     "redacts empty string",
			input:    "",
			expected: "[REDACTED]",
		},
	}

	// Verify URL roundtrip fidelity: redacting url.Parse(s).String() must
	// produce the same result as redacting s directly, otherwise string-based
	// redaction could silently miss normalized URLs.
	roundtripEndpoints := []string{
		"https://eth-mainnet.g.alchemy.com/v2/abcdef123456",
		"https://mainnet.infura.io/v3/key123?project=secret",
		"http://localhost:8545/rpc",
		"https://rpc.example.com/v2/key%2Bwith%2Bencoding",
		"wss://ws.alchemy.com/v2/secret-key-123",
	}
	for _, ep := range roundtripEndpoints {
		parsed, err := url.Parse(ep)
		if err != nil {
			continue
		}
		tests = append(tests, struct {
			name     string
			input    string
			expected string
		}{
			name:     "roundtrip fidelity: " + parsed.Host,
			input:    parsed.String(),
			expected: redactURLString(ep),
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactURLString(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestRedactedLeveledLogger(t *testing.T) {
	endpoint := "https://eth-mainnet.g.alchemy.com/v2/secret-key-123"

	newTestLogger := func() (*bytes.Buffer, *redactedLeveledLogger) {
		var buf bytes.Buffer
		handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		logger := slog.New(handler)
		return &buf, newRedactedLogger(logger, endpoint)
	}

	t.Run("redacts url key", func(t *testing.T) {
		buf, rl := newTestLogger()
		rl.Debug("performing request", "method", "POST", "url", endpoint)

		output := buf.String()
		require.Contains(t, output, "performing request")
		require.Contains(t, output, "https://eth-mainnet.g.alchemy.com")
		require.NotContains(t, output, "secret-key-123")
		require.NotContains(t, output, "/v2/")
	})

	t.Run("redacts request key containing method and URL", func(t *testing.T) {
		buf, rl := newTestLogger()
		rl.Debug("retrying request", "request", "POST "+endpoint, "timeout", "1s")

		output := buf.String()
		require.Contains(t, output, "retrying request")
		require.Contains(t, output, "POST https://eth-mainnet.g.alchemy.com")
		require.NotContains(t, output, "secret-key-123")
		require.NotContains(t, output, "/v2/")
	})

	t.Run("redacts error values containing URL", func(t *testing.T) {
		buf, rl := newTestLogger()
		httpErr := &url.Error{
			Op:  "Get",
			URL: endpoint,
			Err: errors.New("dial tcp: connection refused"),
		}
		rl.Error("request failed", "error", httpErr, "url", endpoint)

		output := buf.String()
		require.Contains(t, output, "request failed")
		require.Contains(t, output, "https://eth-mainnet.g.alchemy.com")
		require.NotContains(t, output, "secret-key-123")
		require.NotContains(t, output, "/v2/")
	})

	t.Run("all log levels redact", func(t *testing.T) {
		for _, level := range []string{"Error", "Warn", "Info", "Debug"} {
			t.Run(level, func(t *testing.T) {
				buf, rl := newTestLogger()
				switch level {
				case "Error":
					rl.Error("msg", "url", endpoint)
				case "Warn":
					rl.Warn("msg", "url", endpoint)
				case "Info":
					rl.Info("msg", "url", endpoint)
				case "Debug":
					rl.Debug("msg", "url", endpoint)
				}
				require.NotContains(t, buf.String(), "secret-key-123")
			})
		}
	})

	t.Run("no-op when endpoint has no path", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		logger := slog.New(handler)
		rl := newRedactedLogger(logger, "http://localhost:8545")

		rl.Debug("msg", "url", "http://localhost:8545")
		require.Contains(t, buf.String(), "http://localhost:8545")
	})

	t.Run("does not modify non-string values", func(t *testing.T) {
		buf, rl := newTestLogger()
		rl.Info("msg", "status", 500, "retries", 3)

		output := buf.String()
		require.Contains(t, output, "500")
		require.Contains(t, output, "3")
	})

	t.Run("handles stringer values", func(t *testing.T) {
		buf, rl := newTestLogger()
		u, _ := url.Parse(endpoint)
		rl.Debug("msg", "parsed", u)

		output := buf.String()
		require.NotContains(t, output, "secret-key-123")
	})

	t.Run("redacts msg parameter containing endpoint", func(t *testing.T) {
		buf, rl := newTestLogger()
		rl.Error("failed request to " + endpoint)

		output := buf.String()
		require.Contains(t, output, "failed request to https://eth-mainnet.g.alchemy.com")
		require.NotContains(t, output, "secret-key-123")
	})
}

func TestRedactEndpointFromError(t *testing.T) {
	t.Run("returns nil for nil error", func(t *testing.T) {
		require.NoError(t, RedactEndpointFromError(nil, "https://host/v2/key"))
	})

	t.Run("redacts endpoint from error message", func(t *testing.T) {
		endpoint := "https://alchemy.com/v2/secret-key"
		err := fmt.Errorf("Get %q: connection refused", endpoint)
		redacted := RedactEndpointFromError(err, endpoint)

		require.NotContains(t, redacted.Error(), "secret-key")
		require.Contains(t, redacted.Error(), "https://alchemy.com")
	})

	t.Run("returns original when endpoint not in error", func(t *testing.T) {
		original := errors.New("some other error")
		result := RedactEndpointFromError(original, "https://host/v2/key")
		require.Equal(t, original, result)
	})

	t.Run("returns original when nothing to redact", func(t *testing.T) {
		original := errors.New("connection refused")
		result := RedactEndpointFromError(original, "http://localhost:8545")
		require.Equal(t, original, result)
	})

	t.Run("redacts url.Error", func(t *testing.T) {
		endpoint := "https://alchemy.com/v2/secret-key"
		urlErr := &url.Error{Op: "Get", URL: endpoint, Err: errors.New("timeout")}
		redacted := RedactEndpointFromError(urlErr, endpoint)

		require.NotContains(t, redacted.Error(), "secret-key")
		require.Contains(t, redacted.Error(), "https://alchemy.com")
	})
}

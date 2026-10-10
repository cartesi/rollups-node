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
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestRetryLeveledLoggerPreservesSafeValues(t *testing.T) {
	var logs bytes.Buffer
	logger := &retryLeveledLogger{logger: slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	for _, log := range []func(string, ...any){logger.Error, logger.Warn, logger.Info, logger.Debug} {
		log("request", "url", "https://provider.example", "attempt", 2)
	}
	require.Equal(t, 4, strings.Count(logs.String(), `"msg":"request"`))
	require.Equal(t, 4, strings.Count(logs.String(), `"url":"https://provider.example"`))
	require.Contains(t, logs.String(), `"attempt":2`)
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

func TestParseRetryAfterHeader(t *testing.T) {
	maxDuration := time.Duration(math.MaxInt64)
	overflowSeconds := strconv.FormatInt(int64(maxDuration/time.Second)+1, 10)
	pastDate := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC1123)

	tests := []struct {
		name    string
		headers []string
		want    time.Duration
		wantOK  bool
	}{
		{
			name:   "missing header",
			wantOK: false,
		},
		{
			name:    "empty header",
			headers: []string{""},
			wantOK:  false,
		},
		{
			name:    "invalid header",
			headers: []string{"not-a-date-or-number"},
			wantOK:  false,
		},
		{
			name:    "negative seconds",
			headers: []string{"-1"},
			wantOK:  false,
		},
		{
			name:    "positive seconds",
			headers: []string{"3"},
			want:    3 * time.Second,
			wantOK:  true,
		},
		{
			name:    "seconds overflow saturates",
			headers: []string{overflowSeconds},
			want:    maxDuration,
			wantOK:  true,
		},
		{
			name:    "past date",
			headers: []string{pastDate},
			want:    0,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRetryAfterHeader(tt.headers)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRetryBackoff(t *testing.T) {
	response := func(status int, retryAfter string) *http.Response {
		resp := &http.Response{
			StatusCode: status,
			Header:     http.Header{},
		}
		if retryAfter != "" {
			resp.Header.Set("Retry-After", retryAfter)
		}
		return resp
	}

	maxDuration := time.Duration(math.MaxInt64)
	overflowSeconds := strconv.FormatInt(int64(maxDuration/time.Second)+1, 10)

	tests := []struct {
		name        string
		minDuration time.Duration
		maxDuration time.Duration
		attemptNum  int
		resp        *http.Response
		want        time.Duration
	}{
		{
			name:        "429 retry after is capped by max duration",
			minDuration: 100 * time.Millisecond,
			maxDuration: 3 * time.Second,
			attemptNum:  1,
			resp:        response(http.StatusTooManyRequests, "10"),
			want:        3 * time.Second,
		},
		{
			name:        "503 retry after below max is honored",
			minDuration: 100 * time.Millisecond,
			maxDuration: 3 * time.Second,
			attemptNum:  1,
			resp:        response(http.StatusServiceUnavailable, "2"),
			want:        2 * time.Second,
		},
		{
			name:        "overflowing retry after is capped by max duration",
			minDuration: 100 * time.Millisecond,
			maxDuration: 3 * time.Second,
			attemptNum:  1,
			resp:        response(http.StatusTooManyRequests, overflowSeconds),
			want:        3 * time.Second,
		},
		{
			name:        "invalid retry after falls back to exponential backoff",
			minDuration: 100 * time.Millisecond,
			maxDuration: 3 * time.Second,
			attemptNum:  1,
			resp:        response(http.StatusTooManyRequests, "-1"),
			want:        200 * time.Millisecond,
		},
		{
			name:        "retry after is ignored for non retry-after status",
			minDuration: 100 * time.Millisecond,
			maxDuration: 3 * time.Second,
			attemptNum:  1,
			resp:        response(http.StatusInternalServerError, "2"),
			want:        200 * time.Millisecond,
		},
		{
			name:        "exponential overflow is capped by max duration",
			minDuration: time.Second,
			maxDuration: 3 * time.Second,
			attemptNum:  100,
			want:        3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := retryBackoff(tt.minDuration, tt.maxDuration, tt.attemptNum, tt.resp)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNewEthClientRequestTimeout(t *testing.T) {
	const requestTimeout = 25 * time.Millisecond

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * requestTimeout)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client, err := NewEthClient(context.Background(), server.URL, logger, RetryConfig{
		RequestTimeout: requestTimeout,
	})
	require.NoError(t, err)

	start := time.Now()
	_, err = client.ChainID(context.Background())
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 5*requestTimeout)
}

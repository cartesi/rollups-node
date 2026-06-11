// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/hashicorp/go-retryablehttp"
)

// parseRetryAfterHeader parses the Retry-After header and returns the
// delay duration according to the spec: https://httpwg.org/specs/rfc7231.html#header.retry-after
// The bool returned will be true if the header was successfully parsed.
// Otherwise, the header was either not present, or was not parseable according to the spec.
func parseRetryAfterHeader(headers []string) (time.Duration, bool) {
	if len(headers) == 0 || headers[0] == "" {
		return 0, false
	}
	header := headers[0]
	// 'Retry-After' is provided in seconds.
	if sleep, err := strconv.ParseInt(header, 10, 64); err == nil {
		if sleep < 0 { // a negative sleep doesn't make sense
			return 0, false
		}
		if sleep > int64(time.Duration(math.MaxInt64)/time.Second) {
			return time.Duration(math.MaxInt64), true
		}
		return time.Second * time.Duration(sleep), true
	}

	// 'Retry-After' is provided as a date.
	retryTime, err := time.Parse(time.RFC1123, header)
	if err != nil {
		return 0, false
	}
	if duration := time.Until(retryTime); duration > 0 {
		return duration, true
	}
	return 0, true // past date
}

// RetryConfig holds configuration for the retryable HTTP client.
type RetryConfig struct {
	MaxRetries     uint64
	RetryMinWait   time.Duration
	RetryMaxWait   time.Duration
	RequestTimeout time.Duration
}

// NewEthClient creates an Ethereum JSON-RPC client with retryable HTTP transport.
// The logger output redacts the endpoint URL to prevent leaking API keys
// that may be embedded in endpoint paths or query parameters.
func NewEthClient(
	ctx context.Context,
	endpoint string,
	logger *slog.Logger,
	retryConfig RetryConfig,
	rpcOptions ...rpc.ClientOption,
) (*ethclient.Client, error) {
	rclient := retryablehttp.NewClient()
	rclient.Logger = newRedactedLogger(logger, endpoint)
	rclient.RetryMax = int(min(retryConfig.MaxRetries, uint64(math.MaxInt)))
	rclient.RetryWaitMin = retryConfig.RetryMinWait
	rclient.RetryWaitMax = retryConfig.RetryMaxWait
	rclient.HTTPClient.Timeout = retryConfig.RequestTimeout
	rclient.Backoff = retryBackoff

	opts := []rpc.ClientOption{
		rpc.WithHTTPClient(rclient.StandardClient()),
	}
	for _, opt := range rpcOptions {
		if opt != nil {
			opts = append(opts, opt)
		}
	}

	rpcClient, err := rpc.DialOptions(ctx, endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial eth client: %w",
			RedactEndpointFromError(err, endpoint))
	}

	return ethclient.NewClient(rpcClient), nil
}

// retryBackoff caps server-directed Retry-After delays while preserving the
// library's default exponential backoff for every other case.
func retryBackoff(minDuration, maxDuration time.Duration, attemptNum int, resp *http.Response) time.Duration {
	if resp != nil {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			if sleep, ok := parseRetryAfterHeader(resp.Header["Retry-After"]); ok {
				return min(maxDuration, sleep)
			}
		}
	}

	return retryablehttp.DefaultBackoff(minDuration, maxDuration, attemptNum, resp)
}

// Compile-time assertion that redactedLeveledLogger implements the LeveledLogger
// interface from hashicorp/go-retryablehttp. Without this, a change to the library's
// interface would only be caught at runtime when the logger is first used, because
// the assignment to rclient.Logger uses the untyped interface{} field.
var _ retryablehttp.LeveledLogger = (*redactedLeveledLogger)(nil)

// redactedLeveledLogger implements retryablehttp.LeveledLogger.
// It wraps slog.Logger and replaces all occurrences of the full endpoint URL
// in log values with a redacted version (scheme://host only). This prevents
// leaking API keys that providers like Alchemy and Infura embed in URL paths.
//
// The redaction is endpoint-aware rather than key-name-aware: it scrubs ALL
// string, error, and fmt.Stringer values, catching URLs regardless of which
// log key they appear under (e.g., "url", "request", "error").
type redactedLeveledLogger struct {
	logger       *slog.Logger
	endpoint     string // full endpoint URL to scrub
	normalized   string // url.Parse(endpoint).String() canonical form (may differ from endpoint)
	redactedHost string // scheme://host replacement
}

func newRedactedLogger(logger *slog.Logger, endpoint string) *redactedLeveledLogger {
	// Precompute the normalized form so we can scrub both the raw and
	// canonical representations without per-call parsing overhead.
	normalized := endpoint
	if u, err := url.Parse(endpoint); err == nil {
		if canon := u.String(); canon != endpoint {
			normalized = canon
		}
	}
	return &redactedLeveledLogger{
		logger:       logger,
		endpoint:     endpoint,
		normalized:   normalized,
		redactedHost: redactURLString(endpoint),
	}
}

func (l *redactedLeveledLogger) Error(msg string, keysAndValues ...any) {
	l.logger.Error(l.redactString(msg), l.redactValues(keysAndValues)...)
}

func (l *redactedLeveledLogger) Info(msg string, keysAndValues ...any) {
	l.logger.Info(l.redactString(msg), l.redactValues(keysAndValues)...)
}

func (l *redactedLeveledLogger) Debug(msg string, keysAndValues ...any) {
	l.logger.Debug(l.redactString(msg), l.redactValues(keysAndValues)...)
}

func (l *redactedLeveledLogger) Warn(msg string, keysAndValues ...any) {
	l.logger.Warn(l.redactString(msg), l.redactValues(keysAndValues)...)
}

func (l *redactedLeveledLogger) redactString(s string) string {
	if l.endpoint == l.redactedHost {
		return s
	}
	s = strings.ReplaceAll(s, l.endpoint, l.redactedHost)
	if l.normalized != l.endpoint {
		s = strings.ReplaceAll(s, l.normalized, l.redactedHost)
	}
	return s
}

// redactValues replaces all occurrences of the full endpoint URL in string,
// error, and fmt.Stringer values with the redacted host-only version.
func (l *redactedLeveledLogger) redactValues(keysAndValues []any) []any {
	if l.endpoint == l.redactedHost {
		return keysAndValues
	}
	result := make([]any, len(keysAndValues))
	copy(result, keysAndValues)
	for i := range result {
		result[i] = l.redactValue(result[i])
	}
	return result
}

func (l *redactedLeveledLogger) redactValue(v any) any {
	switch val := v.(type) {
	case string:
		return l.redactString(val)
	case error:
		s := val.Error()
		replaced := l.redactString(s)
		if replaced != s {
			return replaced
		}
		return v
	case fmt.Stringer:
		s := val.String()
		replaced := l.redactString(s)
		if replaced != s {
			return replaced
		}
		return v
	default:
		return v
	}
}

// RedactEndpointFromError returns a new error with all occurrences of the
// endpoint URL replaced by its redacted (scheme://host) form. Returns nil
// for nil errors. This should be used to wrap errors from dial functions
// (both HTTP and WebSocket) that may embed the full URL in error messages.
//
// When redaction occurs, the returned error intentionally does not wrap the
// original; errors.Is and errors.As will not match the original error chain.
// This prevents callers from recovering the sensitive URL via error unwrapping.
func RedactEndpointFromError(err error, endpoint string) error {
	if err == nil {
		return nil
	}
	redacted := redactURLString(endpoint)
	if endpoint == redacted {
		return err
	}
	original := err.Error()
	msg := strings.ReplaceAll(original, endpoint, redacted)
	// Also replace the normalized/canonical form which may differ from the
	// raw endpoint due to percent-encoding or other URL canonicalization.
	if u, parseErr := url.Parse(endpoint); parseErr == nil {
		if normalized := u.String(); normalized != endpoint {
			msg = strings.ReplaceAll(msg, normalized, redacted)
		}
	}
	if msg == original {
		return err
	}
	return fmt.Errorf("%s", msg)
}

// redactURLString strips the path, query, and fragment from a URL string,
// returning only scheme://host. Returns "[REDACTED]" if the URL cannot be
// parsed or has no scheme/host.
func redactURLString(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "[REDACTED]"
	}
	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
}

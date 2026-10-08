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

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/internal/httpclient"
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
// Only HTTP(S) endpoints with a hostname are supported. Endpoint userinfo,
// paths, and queries remain below retry and RPC error handling. The constructor
// owns its HTTP client: rpc.WithHTTPClient options are superseded by the protected
// retry client. Other RPC options, including headers and authentication, apply.
func NewEthClient(
	ctx context.Context,
	endpoint string,
	logger *slog.Logger,
	retryConfig RetryConfig,
	rpcOptions ...rpc.ClientOption,
) (*ethclient.Client, error) {
	rclient := retryablehttp.NewClient()
	safeBase, inner, err := httpclient.NewClient(endpoint, rclient.HTTPClient, httpclient.Options{})
	if err != nil {
		return nil, err
	}
	rclient.HTTPClient = inner
	rclient.Logger = &retryLeveledLogger{logger: logger}
	rclient.RetryMax = int(min(retryConfig.MaxRetries, uint64(math.MaxInt)))
	rclient.RetryWaitMin = retryConfig.RetryMinWait
	rclient.RetryWaitMax = retryConfig.RetryMaxWait
	rclient.HTTPClient.Timeout = retryConfig.RequestTimeout
	rclient.Backoff = retryBackoff
	rclient.CheckRetry = protectedRetryPolicy

	httpClient := rclient.StandardClient()
	httpClient.CheckRedirect = inner.CheckRedirect
	httpClient.Transport = nullErrorTransport{next: httpClient.Transport}
	opts := make([]rpc.ClientOption, 0, len(rpcOptions)+1)
	for _, opt := range rpcOptions {
		if opt != nil {
			opts = append(opts, opt)
		}
	}
	// Always keep endpoint restoration and diagnostic protection. ClientOption is
	// deliberately opaque; do not inspect it to try to extract an injected client.
	opts = append(opts, rpc.WithHTTPClient(httpClient))

	rpcClient, err := rpc.DialOptions(ctx, safeBase, opts...)
	if err != nil {
		return nil, httpclient.Diagnostic(ctx, err, "dial eth client", safeBase)
	}

	return ethclient.NewClient(rpcClient), nil
}

// DialEthClient connects to an Ethereum JSON-RPC endpoint like
// ethclient.DialContext, and accepts a null error member in the responses like
// NewEthClient. It accepts only HTTP(S) endpoints with a hostname and does not
// follow redirects.
func DialEthClient(ctx context.Context, endpoint string) (*ethclient.Client, error) {
	safeBase, client, err := httpclient.NewClient(endpoint, nil, httpclient.Options{})
	if err != nil {
		return nil, err
	}
	client.Transport = nullErrorTransport{next: client.Transport}
	rpcClient, err := rpc.DialOptions(ctx, safeBase, rpc.WithHTTPClient(client))
	if err != nil {
		return nil, httpclient.Diagnostic(ctx, err, "dial eth client", safeBase)
	}
	return ethclient.NewClient(rpcClient), nil
}

// Retry decisions use the original cause, because retryablehttp inspects both
// concrete certificate types and invalid-header text. Policy errors are discarded
// locally; the returned error can only be the request context's safe sentinel.
func protectedRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	retry, _ := retryablehttp.DefaultRetryPolicy(ctx, resp, httpclient.ClassificationError(err))
	// The policy uses the original cause only for its decision. Its error
	// output is restricted locally instead of trusting dependency behavior.
	return retry, ctx.Err()
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

// The transport supplies safe diagnostics before retryablehttp logs them.
// This adapter only lowers pure cancellation failures to Debug.
var _ retryablehttp.LeveledLogger = (*retryLeveledLogger)(nil)

type retryLeveledLogger struct{ logger *slog.Logger }

func (l *retryLeveledLogger) Error(msg string, values ...any) {
	for i := 0; i+1 < len(values); i += 2 {
		if values[i] == "error" {
			if err, ok := values[i+1].(error); ok && errutil.IsOnlyCancellation(err) {
				l.logger.Debug(msg, values...)
				return
			}
		}
	}
	l.logger.Error(msg, values...)
}

func (l *retryLeveledLogger) Info(msg string, values ...any)  { l.logger.Info(msg, values...) }
func (l *retryLeveledLogger) Debug(msg string, values ...any) { l.logger.Debug(msg, values...) }
func (l *retryLeveledLogger) Warn(msg string, values ...any)  { l.logger.Warn(msg, values...) }

// RedactEndpointFromError returns a new error with all occurrences of the
// endpoint URL replaced by its redacted (scheme://host) form. Returns nil
// for nil errors. This legacy API replaces only exact URL representations;
// masked or differently escaped URLs can escape replacement. HTTP callers
// should use NewEthClient or DialEthClient, which protect the error producer.
//
// When redaction occurs, the returned error intentionally does not wrap the
// original; errors.Is and errors.As will not match the original error chain.
// This prevents callers from recovering the sensitive URL via error unwrapping.
//
// Deprecated: use NewEthClient or DialEthClient to protect diagnostics at the
// HTTP transport boundary instead of matching endpoint strings after failure.
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

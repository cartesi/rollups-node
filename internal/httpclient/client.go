// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package httpclient keeps configured endpoint credentials below HTTP client
// error and redirect handling. Successful response bodies are never redacted.
package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/cartesi/rollups-node/internal/errutil"
)

// Validate accepts HTTP(S) endpoints with a DNS name or IP address. Its errors never
// include the input, including failures from net/url.
func Validate(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid HTTP endpoint")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, errors.New("HTTP endpoint requires http or https and a DNS name or IP address")
	}
	return u, nil
}

// Options configures endpoint composition and trusted diagnostic messages.
type Options struct {
	// AppendOperation appends the request path and query to the endpoint.
	AppendOperation bool
	// AllowedErrorMessages are fixed, nonsensitive messages that can pass through
	// in error responses. Values from configuration or responses are not trusted.
	AllowedErrorMessages []string
}

// New gives callers a host-only request URL and a transport which restores the
// actual endpoint only in a private request copy. Options control whether the
// operation path and query are appended and which fixed messages can be retained.
// An injected next transport must follow the net/http RoundTripper contract.
func New(raw string, next http.RoundTripper, options Options) (string, http.RoundTripper, error) {
	u, err := Validate(raw)
	if err != nil {
		return "", nil, err
	}
	if next == nil {
		next = http.DefaultTransport
	}
	safeBase := (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
	return safeBase, &transport{
		endpoint: *u, safeBase: safeBase, next: next, appendOperation: options.AppendOperation,
		allowedErrorMessages: slices.Clone(options.AllowedErrorMessages),
	}, nil
}

// NewClient copies client and protects its transport. It preserves timeout,
// cookie jar, and other client settings, but always disables redirects. A nil
// client uses the standard library's default client settings.
func NewClient(raw string, client *http.Client, options Options) (string, *http.Client, error) {
	if client == nil {
		client = http.DefaultClient
	}
	protected := *client
	safeBase, next, err := New(raw, client.Transport, options)
	if err != nil {
		return "", nil, err
	}
	protected.Transport = next
	if client.Jar != nil {
		protected.Jar = &endpointJar{next: client.Jar, endpoint: next.(*transport)}
	}
	protected.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return safeBase, &protected, nil
}

type transport struct {
	endpoint             url.URL
	safeBase             string
	next                 http.RoundTripper
	appendOperation      bool
	allowedErrorMessages []string
}

// Error bodies are diagnostic input, not consensus input. Bound the work done
// before retry classification; larger responses use a fixed fallback message.
const maxDiagnosticResponseBytes = 64 << 10

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("HTTP request requires a URL")
	}
	wire := req.Clone(req.Context())
	u := t.wireURL(req.URL)
	wire.URL = u
	if wire.Host == "" || wire.Host == req.URL.Host {
		wire.Host = u.Host
	}
	if wire.Header == nil {
		wire.Header = make(http.Header)
	}
	if u.User != nil && wire.Header.Get("Authorization") == "" {
		password, _ := u.User.Password()
		wire.SetBasicAuth(u.User.Username(), password)
	}
	resp, err := t.next.RoundTrip(wire)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, Diagnostic(req.Context(), err, "HTTP request", t.safeBase)
	}
	if resp == nil {
		return nil, errors.New("HTTP transport returned no response")
	}
	// net/http uses Response.Request.URL in redirect errors, and parses Location
	// before invoking CheckRedirect. Neither may see the wire endpoint.
	// Keep the response itself: net/http fills unannounced trailers on this
	// object when its body is consumed.
	resp.Header = resp.Header.Clone()
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	resp.Request = req
	resp.Header.Del("Location")
	resp.Status = strconv.Itoa(resp.StatusCode)
	if text := http.StatusText(resp.StatusCode); text != "" {
		resp.Status += " " + text
	}
	if resp.Body == nil {
		resp.Body = http.NoBody
	}
	resp.Body = &diagnosticBody{ReadCloser: resp.Body, safeBase: t.safeBase, ctx: req.Context()}
	if req.Method == http.MethodHead || resp.StatusCode < http.StatusOK ||
		resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		// A 101 response can carry an upgraded protocol stream. These HTTP API
		// clients do not support upgrades; close it instead of exposing its bytes.
		// Retain the status so existing HTTP error classification still applies.
		_ = resp.Body.Close()
		resp.Body = http.NoBody
		return resp, nil
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiagnosticResponseBytes+1))
		_ = resp.Body.Close()
		interrupted := req.Context().Err() != nil
		select {
		//nolint:staticcheck,nolintlint // Client.Timeout requires this deliberate use of legacy Cancel.
		case <-req.Cancel:
			interrupted = true
		default:
		}
		if err != nil && interrupted {
			// Keep the status visible to retry policy. The decoder above retry
			// handling must still receive the captured safe interruption cause.
			resp.Body = io.NopCloser(failedDiagnosticRead{err: err})
			return resp, nil
		}
		if err != nil || len(body) > maxDiagnosticResponseBytes {
			body = nil
		}
		body = diagnosticResponse(body, t.allowedErrorMessages)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.TransferEncoding = nil
		resp.Header.Del("Content-Length")
		resp.Header.Del("Transfer-Encoding")
		resp.Header.Del("Content-Encoding")
		contentType := "text/plain; charset=utf-8"
		if json.Valid(body) {
			contentType = "application/json"
		}
		resp.Header.Set("Content-Type", contentType)
	}
	return resp, nil
}

func (t *transport) wireURL(operation *url.URL) *url.URL {
	u := t.endpoint
	if t.appendOperation {
		// Trim only a literal separator, never an encoded trailing slash. RawPath
		// must continue to decode to Path or net/url silently discards it.
		if strings.HasSuffix(t.endpoint.EscapedPath(), "/") {
			u.Path = strings.TrimSuffix(u.Path, "/")
		}
		u.Path += operation.Path
		u.RawPath = strings.TrimSuffix(t.endpoint.EscapedPath(), "/") + operation.EscapedPath()
		if u.RawQuery == "" {
			u.RawQuery = operation.RawQuery
		} else if operation.RawQuery != "" {
			u.RawQuery += "&" + operation.RawQuery
		}
	}
	return &u
}

// Cookie paths must be selected and stored against the wire URL, rather than
// the host-only request seen by http.Client. The same URL composition is used
// for requests and both cookie-jar operations.
type endpointJar struct {
	next     http.CookieJar
	endpoint *transport
}

func (j *endpointJar) Cookies(u *url.URL) []*http.Cookie {
	return j.next.Cookies(j.endpoint.wireURL(u))
}

func (j *endpointJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.next.SetCookies(j.endpoint.wireURL(u), cookies)
}

func (t *transport) CloseIdleConnections() {
	if closer, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// A read failure must stay above retry status handling after its original body
// has been closed. The stored error already has a safe diagnostic message.
type failedDiagnosticRead struct{ err error }

func (b failedDiagnosticRead) Read([]byte) (int, error) { return 0, b.err }

// diagnosticResponse retains numeric RPC codes and the fixed nonce rejection
// phrase used by existing classifiers. An explicit client policy can also retain
// exact trusted messages. All other provider text may echo credentials.
func diagnosticResponse(body []byte, allowedErrorMessages []string) []byte {
	rpcMessage := "RPC request failed"
	if strings.Contains(strings.ToLower(string(body)), "nonce too low") {
		rpcMessage = "nonce too low"
	}
	var input struct {
		Error *struct {
			Code *int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &input) == nil && input.Error != nil && input.Error.Code != nil {
		response := struct {
			JSONRPC string `json:"jsonrpc"`
			Error   struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}{JSONRPC: "2.0"}
		response.Error.Code = *input.Error.Code
		response.Error.Message = rpcMessage
		encoded, _ := json.Marshal(response) // Integers and strings cannot fail to marshal.
		return encoded
	}
	if rpcMessage == "nonce too low" {
		return []byte(rpcMessage)
	}
	message := strings.TrimSuffix(string(body), "\n")
	if slices.Contains(allowedErrorMessages, message) {
		return []byte(message)
	}
	return []byte("HTTP request failed")
}

type diagnosticBody struct {
	io.ReadCloser
	safeBase string
	ctx      context.Context
}

func (b *diagnosticBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && err != io.EOF {
		err = Diagnostic(b.ctx, err, "HTTP response read", b.safeBase)
	}
	return n, err
}

func (b *diagnosticBody) Close() error {
	return Diagnostic(b.ctx, b.ReadCloser.Close(), "HTTP response close", b.safeBase)
}

// Diagnostic supplies input-independent output while preserving causes for
// errors.Is/errors.As and timeout classification. Unwrapped causes are for
// classification only: callers must render this error rather than the cause.
// Valid successful RPC messages and credential-bearing hostnames remain outside
// this guarantee. operation and safeBase must be trusted, nonsensitive text.
func Diagnostic(ctx context.Context, err error, operation, safeBase string) error {
	if err == nil {
		return nil
	}
	category := ""
	switch {
	case errutil.IsOnlyCancellation(err):
		category = "context canceled"
	case ctx != nil && ctx.Err() == context.DeadlineExceeded && errors.Is(err, ctx.Err()):
		category = "context deadline exceeded"
	default:
		var timeout net.Error
		var network *net.OpError
		var dns *net.DNSError
		var unknownCA x509.UnknownAuthorityError
		var hostname x509.HostnameError
		var certificate *tls.CertificateVerificationError
		var invalidCertificate x509.CertificateInvalidError
		var record tls.RecordHeaderError
		switch {
		case errors.As(err, &network) && network.Op == "proxyconnect":
			category = "proxy connection failed"
		case errors.As(err, &timeout) && timeout.Timeout():
			category = "timeout"
		case errors.As(err, &dns):
			category = "DNS lookup failed"
		case errors.Is(err, syscall.ECONNREFUSED):
			category = "connection refused"
		case errors.Is(err, syscall.ECONNRESET):
			category = "connection reset"
		case errors.As(err, &unknownCA):
			category = "unknown certificate authority"
		case errors.As(err, &hostname):
			category = "certificate hostname mismatch"
		case errors.As(err, &certificate), errors.As(err, &invalidCertificate):
			category = "certificate verification failed"
		case errors.As(err, &record):
			category = "TLS handshake failed"
		case errors.Is(err, io.ErrUnexpectedEOF):
			category = "unexpected EOF"
		case errors.Is(err, io.EOF):
			if operation == "decode RPC response" || operation == "decode JSON-RPC response" {
				category = "empty response"
			} else {
				category = "connection closed"
			}
		}
	}
	message := operation + " failed for " + safeBase
	if category != "" {
		message += ": " + category
	}
	return &diagnosticError{cause: err, message: message}
}

// ClassificationError restores the original transport cause for dependency
// classifiers which inspect concrete types or message text. Its result must
// never be logged, returned, or rendered. Use the original safe diagnostic for
// output. This only removes this package's adapters and copies url.Error.
func ClassificationError(err error) error {
	switch e := err.(type) {
	case *diagnosticError:
		return e.cause
	case *url.Error:
		original := *e
		original.Err = ClassificationError(e.Err)
		return &original
	default:
		return err
	}
}

type diagnosticError struct {
	cause   error
	message string
}

func (e *diagnosticError) Error() string { return e.message }
func (e *diagnosticError) Unwrap() error { return e.cause }
func (e *diagnosticError) Timeout() bool {
	var timeout net.Error
	return errors.As(e.cause, &timeout) && timeout.Timeout()
}
func (e *diagnosticError) Temporary() bool {
	var temporary interface{ Temporary() bool }
	return errors.As(e.cause, &temporary) && temporary.Temporary()
}
func (e *diagnosticError) GoString() string { return e.message }

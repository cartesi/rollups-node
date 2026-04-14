// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"

	"github.com/google/uuid"
)

const (
	requestIDHeader = "X-Request-ID"
)

// requestIDPattern defines the accepted charset and length for
// upstream-supplied request ids. Anything outside this set is treated as
// untrusted and a fresh UUID is generated instead.
//
// The charset is deliberately chosen to cover the ID formats emitted by
// common reverse proxies and tracing systems while remaining safe to log
// and echo in response headers:
//
//   - envoy uses `.` and `:`
//   - AWS ALB and X-Ray use `-` and `=`
//   - GCP Cloud Trace uses `/`
//   - `+` appears in some base64-style correlation IDs
//
// Explicitly excluded: `\r`, `\n`, space, `<`, `>`, `"`, `'`, backtick,
// and all other control characters — these would enable log-injection or
// header-splitting if echoed back verbatim. The `{1,128}` quantifier is
// the single source of truth for the length bound.
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:=/+-]{1,128}$`)

// RequestIDMiddleware reads the X-Request-ID request header and trusts it
// only when it matches ^[A-Za-z0-9._:=/+-]{1,128}$. Otherwise a fresh
// UUIDv4 is generated. The chosen id is placed on the request context
// under ctxKeyRequestID{} and echoed on the response as X-Request-ID.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !requestIDPattern.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// responseWriterTap wraps an http.ResponseWriter and records whether any
// header or body bytes have been sent. It implements Unwrap so that helpers
// such as http.MaxBytesReader and http.ResponseController can walk the
// wrapper chain to reach the underlying *response and access its internal
// interfaces (for example, to force a connection close after 413).
//
// Without Unwrap, oversized POST bodies still return 413 but the underlying
// connection is not force-closed and clients may see protocol confusion on a
// subsequent pipelined request. Do not remove Unwrap.
type responseWriterTap struct {
	http.ResponseWriter
	wroteHeader bool
}

func (t *responseWriterTap) WriteHeader(code int) {
	t.wroteHeader = true
	t.ResponseWriter.WriteHeader(code)
}

func (t *responseWriterTap) Write(b []byte) (int, error) {
	t.wroteHeader = true
	return t.ResponseWriter.Write(b)
}

func (t *responseWriterTap) Unwrap() http.ResponseWriter {
	return t.ResponseWriter
}

// Flush forwards to the wrapped writer's Flusher implementation if it
// supports streaming. Because responseWriterTap embeds the
// http.ResponseWriter interface (not a concrete type), Go's method
// promotion only surfaces Header/Write/WriteHeader — optional interfaces
// like http.Flusher are not promoted and a direct
// w.(http.Flusher).Flush() on the tap would fail the type assertion even
// when the underlying writer supports it. Forward explicitly so handlers
// can stream through RecoverMiddleware.
//
// http.NewResponseController(w).Flush() also works for the same purpose
// because it walks Unwrap() — use that path when you can't rely on a
// type assertion.
func (t *responseWriterTap) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards to the wrapped writer's Hijacker implementation if any.
// Required for any future websocket or connection-takeover use case;
// today no inspect or jsonrpc handler hijacks, but having the forwarding
// in place avoids a silent failure the moment one is added. Returns
// http.ErrNotSupported when the underlying writer does not implement
// http.Hijacker (for example, httptest.ResponseRecorder).
func (t *responseWriterTap) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := t.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// RecoverMiddleware catches panics from downstream handlers. If nothing has
// been written yet it emits a generic 500 via WriteInternalError. If bytes
// are already on the wire it re-panics with http.ErrAbortHandler to drop the
// connection silently — stitching a 500 onto a partial 200 would produce a
// corrupt response and a "superfluous WriteHeader" warning.
//
// Panics whose value is http.ErrAbortHandler are re-panicked unchanged so
// that the stdlib server preserves its special (non-logged) semantics.
//
// When RecoverMiddleware wraps RequestIDMiddleware (Recover outermost), the
// deferred recovery cannot read the request id from r.Context() because r is
// the pre-wrap request from this closure — downstream middleware creates a
// new *http.Request via WithContext and passes it to its next.ServeHTTP, but
// the outer r is untouched. The request id is instead read from the tap's
// response header, which RequestIDMiddleware populates on the shared
// ResponseWriter before calling next, so it is always set by the time a
// downstream panic reaches the defer.
func RecoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tap := &responseWriterTap{ResponseWriter: w}
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				reqID := tap.Header().Get(requestIDHeader)
				logger.Error("http handler panic",
					"panic", rec,
					"stack", string(debug.Stack()),
					"request_id", reqID,
				)
				if !tap.wroteHeader {
					ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, reqID)
					WriteInternalError(ctx, tap, logger, fmt.Errorf("panic in handler: %v", rec))
					return
				}
				panic(http.ErrAbortHandler)
			}()
			next.ServeHTTP(tap, r)
		})
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// runMiddleware wires a middleware in front of a handler and records the
// response via httptest.NewRecorder.
func runMiddleware(t *testing.T, mw func(http.Handler) http.Handler, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	mw(h).ServeHTTP(rr, req)
	return rr
}

// -----------------------------------------------------------------------------
// RequestIDMiddleware
// -----------------------------------------------------------------------------

func TestRequestIDMiddleware_GeneratesWhenMissing(t *testing.T) {
	var capturedID string
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := runMiddleware(t, RequestIDMiddleware, h, req)

	require.NotEmpty(t, capturedID)
	require.Equal(t, capturedID, rr.Header().Get("X-Request-ID"))

	_, err := uuid.Parse(capturedID)
	require.NoError(t, err, "generated id should parse as UUID")
}

func TestRequestIDMiddleware_AcceptsValid(t *testing.T) {
	// Pin the full accepted charset. Each entry represents a real-world
	// upstream format we must preserve end-to-end for correlation:
	//   - "abc_123-xyz"           — legacy underscore/hyphen
	//   - "abc.def.123"           — envoy-style dotted id
	//   - "a:b:c"                 — envoy host:port:id
	//   - "trace=1-2-3"           — AWS X-Ray style with '='
	//   - "projects/foo/traces/bar" — GCP Cloud Trace path
	//   - "trace+span"            — base64-ish '+'
	cases := []string{
		"abc_123-xyz",
		"abc.def.123",
		"a:b:c",
		"trace=1-2-3",
		"projects/foo/traces/bar",
		"trace+span",
	}
	for _, valid := range cases {
		t.Run(valid, func(t *testing.T) {
			var capturedID string
			h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				capturedID = RequestIDFromContext(r.Context())
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-Request-ID", valid)
			rr := runMiddleware(t, RequestIDMiddleware, h, req)

			require.Equal(t, valid, capturedID)
			require.Equal(t, valid, rr.Header().Get("X-Request-ID"))
		})
	}
}

func TestRequestIDMiddleware_RejectsTooLong(t *testing.T) {
	long := strings.Repeat("a", 129)
	var capturedID string
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", long)
	runMiddleware(t, RequestIDMiddleware, h, req)

	require.NotEqual(t, long, capturedID)
	_, err := uuid.Parse(capturedID)
	require.NoError(t, err, "regenerated id should parse as UUID")
}

func TestRequestIDMiddleware_RejectsBadChars(t *testing.T) {
	// Each case must be regenerated as a fresh UUID. Keep the charset
	// exclusion list tight: anything that could enable log-injection,
	// header-splitting, or HTML/JS smuggling when echoed back in logs or
	// on the X-Request-ID response header.
	cases := map[string]string{
		"semicolon":    "foo;bar",
		"space":        "id with space",
		"newline":      "id\nnewline",
		"carriage":     "id\rcr",
		"tab":          "foo\tbar",
		"angle":        "<script>",
		"double-quote": `id"quote`,
		"single-quote": "id'quote",
		"backtick":     "id`tick",
		"non-ascii":    "föö",
		"control-nul":  "id\x00nul",
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			var capturedID string
			h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				capturedID = RequestIDFromContext(r.Context())
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-Request-ID", bad)
			rr := runMiddleware(t, RequestIDMiddleware, h, req)

			require.NotEqual(t, bad, capturedID,
				"middleware must regenerate a fresh UUID for untrusted input")
			require.NotEqual(t, bad, rr.Header().Get("X-Request-ID"),
				"response header must not echo the rejected value")
			_, err := uuid.Parse(capturedID)
			require.NoError(t, err)
		})
	}
}

func TestRequestIDMiddleware_RejectsEmpty(t *testing.T) {
	var capturedID string
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "")
	runMiddleware(t, RequestIDMiddleware, h, req)

	require.NotEmpty(t, capturedID)
	_, err := uuid.Parse(capturedID)
	require.NoError(t, err)
}

func TestRequestIDMiddleware_AcceptsExactly128(t *testing.T) {
	exact := strings.Repeat("a", 128)
	var capturedID string
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", exact)
	runMiddleware(t, RequestIDMiddleware, h, req)

	require.Equal(t, exact, capturedID)
}

// -----------------------------------------------------------------------------
// responseWriterTap
// -----------------------------------------------------------------------------

func TestResponseWriterTap_Unwrap(t *testing.T) {
	inner := httptest.NewRecorder()
	tap := &responseWriterTap{ResponseWriter: inner}

	// Direct Unwrap returns the wrapped writer so http.MaxBytesReader and
	// http.ResponseController can walk to the real conn.
	require.Same(t, http.ResponseWriter(inner), tap.Unwrap())
}

func TestResponseWriterTap_TracksWrite(t *testing.T) {
	inner := httptest.NewRecorder()
	tap := &responseWriterTap{ResponseWriter: inner}
	require.False(t, tap.wroteHeader)

	_, _ = tap.Write([]byte("hello"))
	require.True(t, tap.wroteHeader)
}

func TestResponseWriterTap_TracksWriteHeader(t *testing.T) {
	inner := httptest.NewRecorder()
	tap := &responseWriterTap{ResponseWriter: inner}
	require.False(t, tap.wroteHeader)

	tap.WriteHeader(http.StatusTeapot)
	require.True(t, tap.wroteHeader)
	require.Equal(t, http.StatusTeapot, inner.Code)
}

// TestResponseWriterTap_FlushForwards pins the explicit http.Flusher
// forwarding. Because responseWriterTap embeds the http.ResponseWriter
// *interface* (not a concrete type), Go's method promotion does not
// surface optional interfaces like Flusher — without an explicit Flush
// method the type assertion below would fail even though the wrapped
// recorder supports flushing. This test would regress silently if the
// forwarding method were removed.
func TestResponseWriterTap_FlushForwards(t *testing.T) {
	inner := httptest.NewRecorder()
	tap := &responseWriterTap{ResponseWriter: inner}

	f, ok := any(tap).(http.Flusher)
	require.True(t, ok, "tap must satisfy http.Flusher via explicit forwarding")
	require.NotPanics(t, func() { f.Flush() })
	require.True(t, inner.Flushed, "underlying recorder should have observed the flush")
}

// TestResponseWriterTap_HijackReturnsNotSupported pins the explicit
// http.Hijacker forwarding. httptest.ResponseRecorder does not implement
// http.Hijacker, so the tap must return http.ErrNotSupported rather than
// panic or return a nil error. This also guards against accidentally
// making Hijack succeed with a nil conn.
func TestResponseWriterTap_HijackReturnsNotSupported(t *testing.T) {
	inner := httptest.NewRecorder()
	tap := &responseWriterTap{ResponseWriter: inner}

	hj, ok := any(tap).(http.Hijacker)
	require.True(t, ok, "tap must satisfy http.Hijacker via explicit forwarding")

	conn, brw, err := hj.Hijack()
	require.Nil(t, conn)
	require.Nil(t, brw)
	require.ErrorIs(t, err, http.ErrNotSupported)
}

// -----------------------------------------------------------------------------
// RecoverMiddleware
// -----------------------------------------------------------------------------

func TestRecoverMiddleware_NoPanic_PassesThrough(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := runMiddleware(t, RecoverMiddleware(captureLogger(&buf)), h, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "ok", rr.Body.String())
	require.Empty(t, buf.String(), "no log entries on the happy path")
}

func TestRecoverMiddleware_HandlerPanicBeforeWrite(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(errors.New("kaboom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := runMiddleware(t, RecoverMiddleware(captureLogger(&buf)), h, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, rr.Body.String(), "Internal server error (request_id=")

	logged := buf.String()
	require.Contains(t, logged, "http handler panic")
	require.Contains(t, logged, "kaboom")
	// Stack trace captured.
	require.Contains(t, logged, "goroutine")
}

func TestRecoverMiddleware_HandlerPanicsErrAbortHandler(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(http.ErrAbortHandler)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Our middleware must re-panic with ErrAbortHandler unchanged. In a
	// real server the stdlib catches it silently; here we assert the
	// re-panic reaches the caller and that we logged nothing.
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		rr := httptest.NewRecorder()
		RecoverMiddleware(captureLogger(&buf))(h).ServeHTTP(rr, req)
	})
	require.Empty(t, buf.String(), "no log output for ErrAbortHandler")
}

func TestRecoverMiddleware_HandlerPanicAfterWrite(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic(errors.New("kaboom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Once bytes are on the wire the middleware must re-panic with
	// ErrAbortHandler so the stdlib drops the connection. If this test
	// fails silently and returns a 500, we are producing a corrupt
	// response (200 header + body + synthetic 500).
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		rr := httptest.NewRecorder()
		RecoverMiddleware(captureLogger(&buf))(h).ServeHTTP(rr, req)
	})

	logged := buf.String()
	require.Contains(t, logged, "http handler panic")
	require.Contains(t, logged, "kaboom")
}

func TestRecoverMiddleware_LogsRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := captureLogger(&buf)

	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(errors.New("boom"))
	})

	// Stack: RecoverMiddleware -> RequestIDMiddleware -> handler
	// (production order — Recover outermost so it also catches panics from
	// RequestIDMiddleware itself). Because r in Recover's closure is the
	// pre-wrap request, Recover must read the request id from the response
	// header (set by RequestIDMiddleware before call-next), not from
	// r.Context(). This test pins that invariant.
	chain := RecoverMiddleware(logger)(RequestIDMiddleware(panicking))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "pin-me")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, rr.Body.String(), "pin-me")
	require.Contains(t, buf.String(), "pin-me")
}

// TestRecoverMiddleware_RealServer_PanicAfterFlushDropsConnection runs the
// post-flush path through an actual *http.Server so we can observe the
// end-to-end behaviour. The handler explicitly flushes before panicking, so
// bytes are on the wire; the stdlib then handles the re-panicked
// ErrAbortHandler silently. The client must never see a lying
// "Internal server error" body stitched onto a 200.
//
// Depending on timing, the client may observe either:
//   - an EOF at http.Get time (connection closed before the client finished
//     parsing the response), or
//   - a 200 + partial body + ErrUnexpectedEOF on body read.
//
// Both are correct outcomes — the invariant is simply that no "Internal
// server error" body is ever spliced onto a started 200 response.
func TestRecoverMiddleware_RealServer_PanicAfterFlushDropsConnection(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush() // force bytes onto the wire
		panic(errors.New("kaboom"))
	})

	srv := httptest.NewServer(RecoverMiddleware(captureLogger(&buf))(h))
	// defer acts as a safety net for any early require failure below; the
	// explicit Close before reading buf synchronises the test goroutine
	// with the server's panic-recovery goroutine (which writes to buf via
	// slog). httptest.Server.Close blocks until all in-flight handlers
	// return, and Close is idempotent so calling it twice is safe.
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err == nil {
		func() {
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				require.ErrorIs(t, readErr, io.ErrUnexpectedEOF)
			}
			require.NotContains(t, string(body), "Internal server error",
				"must not stitch a 500 body onto an already-started 200 response")
		}()
	}

	// Synchronisation barrier: block until the server handler goroutine
	// (which logs "http handler panic" from inside RecoverMiddleware's
	// defer) has fully returned before we read buf. Without this, reading
	// buf.String() races the slog.Write happening on the panic path.
	srv.Close()

	logged := buf.String()
	// Pin both halves of the post-flush log entry: the canonical
	// message and the panic value. Asserting the panic value ensures
	// this log line corresponds to *this* panic, not some stray entry,
	// and mirrors the recorder-based TestRecoverMiddleware_HandlerPanicAfterWrite.
	require.Contains(t, logged, "http handler panic")
	require.Contains(t, logged, "kaboom")
}

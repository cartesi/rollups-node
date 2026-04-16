// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ParseCORSConfig — constructor tests
// ---------------------------------------------------------------------------

func TestParseCORSConfig_EmptyString(t *testing.T) {
	cfg := ParseCORSConfig(discardLogger(), "", []string{"POST"}, []string{"Content-Type"})
	require.False(t, cfg.Enabled())
}

func TestParseCORSConfig_WhitespaceOnly(t *testing.T) {
	cfg := ParseCORSConfig(discardLogger(), "  ,  , ", []string{"POST"}, []string{"Content-Type"})
	require.False(t, cfg.Enabled())
}

func TestParseCORSConfig_NullRejected(t *testing.T) {
	var buf bytes.Buffer
	cfg := ParseCORSConfig(captureLogger(&buf), "null", []string{"POST"}, []string{"Content-Type"})
	require.False(t, cfg.Enabled())
	require.Contains(t, buf.String(), "null")
	require.Contains(t, buf.String(), "level=WARN")
}

func TestParseCORSConfig_NullAmongValid(t *testing.T) {
	var buf bytes.Buffer
	cfg := ParseCORSConfig(captureLogger(&buf), "http://ok.com,null", []string{"POST"}, []string{"Content-Type"})
	require.True(t, cfg.Enabled())
	_, ok := cfg.allowedOrigins["http://ok.com"]
	require.True(t, ok)
	_, ok = cfg.allowedOrigins["null"]
	require.False(t, ok)
}

func TestParseCORSConfig_TrailingSlashStripped(t *testing.T) {
	cfg := ParseCORSConfig(discardLogger(), "http://example.com/", []string{"POST"}, []string{"Content-Type"})
	require.True(t, cfg.Enabled())
	_, ok := cfg.allowedOrigins["http://example.com"]
	require.True(t, ok)
}

func TestParseCORSConfig_PathRejected(t *testing.T) {
	var buf bytes.Buffer
	cfg := ParseCORSConfig(captureLogger(&buf), "http://example.com/foo", []string{"POST"}, []string{"Content-Type"})
	require.False(t, cfg.Enabled())
	require.Contains(t, buf.String(), "level=WARN")
}

func TestParseCORSConfig_QueryRejected(t *testing.T) {
	var buf bytes.Buffer
	cfg := ParseCORSConfig(captureLogger(&buf), "http://example.com?x=1", []string{"POST"}, []string{"Content-Type"})
	require.False(t, cfg.Enabled())
	require.Contains(t, buf.String(), "level=WARN")
}

func TestParseCORSConfig_FragmentRejected(t *testing.T) {
	var buf bytes.Buffer
	cfg := ParseCORSConfig(captureLogger(&buf), "http://example.com#f", []string{"POST"}, []string{"Content-Type"})
	require.False(t, cfg.Enabled())
	require.Contains(t, buf.String(), "level=WARN")
}

func TestParseCORSConfig_Lowercased(t *testing.T) {
	cfg := ParseCORSConfig(discardLogger(), "HTTP://EXAMPLE.COM", []string{"POST"}, []string{"Content-Type"})
	require.True(t, cfg.Enabled())
	_, ok := cfg.allowedOrigins["http://example.com"]
	require.True(t, ok)
}

func TestParseCORSConfig_ValidOrigins(t *testing.T) {
	cfg := ParseCORSConfig(
		discardLogger(),
		"http://localhost:3000, https://app.example.com",
		[]string{"POST", "OPTIONS"},
		[]string{"Content-Type"},
	)
	require.True(t, cfg.Enabled())
	_, ok := cfg.allowedOrigins["http://localhost:3000"]
	require.True(t, ok)
	_, ok = cfg.allowedOrigins["https://app.example.com"]
	require.True(t, ok)
}

// ---------------------------------------------------------------------------
// CORSMiddleware — runtime tests
// ---------------------------------------------------------------------------

// echoHandler writes a 200 and records that it was called via the pointed bool.
func echoHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

func enabledCORSConfig() CORSConfig {
	return ParseCORSConfig(
		discardLogger(),
		"http://example.com, https://other.example.com",
		[]string{"POST", "OPTIONS"},
		[]string{"Content-Type"},
	)
}

func TestCORSMiddleware_Disabled(t *testing.T) {
	cfg := ParseCORSConfig(discardLogger(), "", []string{"POST"}, []string{"Content-Type"})
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.True(t, called)
	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, rr.Header().Get("Vary"))
}

func TestCORSMiddleware_NoOriginHeader(t *testing.T) {
	cfg := enabledCORSConfig()
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.True(t, called)
	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, rr.Header().Get("Vary"))
}

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	cfg := enabledCORSConfig()
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.True(t, called)
	require.Equal(t, "http://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rr.Header().Values("Vary"), "Origin")
	require.Equal(t, "POST, OPTIONS", rr.Header().Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Content-Type", rr.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	cfg := enabledCORSConfig()
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.True(t, called)
	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rr.Header().Values("Vary"), "Origin")
}

func TestCORSMiddleware_CaseInsensitive(t *testing.T) {
	cfg := enabledCORSConfig()
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "HTTP://EXAMPLE.COM")
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.True(t, called)
	require.Equal(t, "HTTP://EXAMPLE.COM", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_PreflightAllowed(t *testing.T) {
	cfg := enabledCORSConfig()
	var called bool
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.False(t, called)
	require.Equal(t, http.StatusNoContent, rr.Code)
	require.Equal(t, "http://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "POST, OPTIONS", rr.Header().Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Content-Type", rr.Header().Get("Access-Control-Allow-Headers"))
	require.Equal(t, "3600", rr.Header().Get("Access-Control-Max-Age"))
	vary := rr.Header().Values("Vary")
	require.Contains(t, vary, "Origin")
	require.Contains(t, vary, "Access-Control-Request-Method")
	require.Contains(t, vary, "Access-Control-Request-Headers")
}

func TestCORSMiddleware_PreflightDisallowed(t *testing.T) {
	cfg := enabledCORSConfig()
	var called bool
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.False(t, called, "disallowed preflight must not reach downstream handler")
	require.Equal(t, http.StatusNoContent, rr.Code)
	require.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rr.Header().Values("Vary"), "Origin")
	require.Empty(t, rr.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddleware_OptionsWithoutRequestMethod(t *testing.T) {
	cfg := enabledCORSConfig()
	var called bool
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.True(t, called)
	require.Equal(t, "http://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, rr.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddleware_CredentialsNeverSet(t *testing.T) {
	cfg := enabledCORSConfig()
	var called bool
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), echoHandler(&called), req)

	require.True(t, called)
	require.Empty(t, rr.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSMiddleware_UpstreamHeadersStripped(t *testing.T) {
	cfg := enabledCORSConfig()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "upstream")
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), upstream, req)

	require.Equal(t, "http://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_VaryUsesAdd(t *testing.T) {
	cfg := enabledCORSConfig()
	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Vary", "Accept-Encoding")
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), upstream, req)

	vary := rr.Header().Values("Vary")
	require.Contains(t, vary, "Origin")
	require.Contains(t, vary, "Accept-Encoding")
}

func TestCORSMiddleware_ErrorResponseGetsCORSHeaders(t *testing.T) {
	cfg := enabledCORSConfig()
	errorHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), errorHandler, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, "http://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rr.Header().Values("Vary"), "Origin")
}

func TestCorsWriter_Unwrap(t *testing.T) {
	inner := httptest.NewRecorder()
	cw := &corsWriter{ResponseWriter: inner, origin: "http://example.com", cfg: enabledCORSConfig()}

	require.Same(t, http.ResponseWriter(inner), cw.Unwrap())
}

func TestCORSMiddleware_MaxBytesReaderUnwrapChain(t *testing.T) {
	cfg := enabledCORSConfig()

	// Handler uses MaxBytesReader to enforce a 16-byte body limit.
	// MaxBytesReader walks the Unwrap() chain to reach the real
	// *http.response so it can force-close the connection after 413.
	// Without Unwrap on corsWriter, the walk would stop at the
	// embedded ResponseWriter interface and the close would silently
	// fail.
	maxBodyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16)
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	oversizedBody := strings.Repeat("X", 1024)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(oversizedBody))
	req.Header.Set("Origin", "http://example.com")
	rr := runMiddleware(t, CORSMiddleware(cfg), maxBodyHandler, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	require.Equal(t, "http://example.com", rr.Header().Get("Access-Control-Allow-Origin"),
		"CORS headers must be present on 413 responses when corsWriter wraps the response writer")
	require.Contains(t, rr.Header().Values("Vary"), "Origin")
}

// TestCORSMiddleware_Admission503GetsCORSHeaders verifies that when admission
// control rejects a request with 503, the response still carries CORS headers.
// The middleware chain is CORS -> Admission -> handler. The corsWriter wraps
// the response writer before admission runs, so WriteHeader(503) triggers CORS
// header injection. A middleware reorder would silently break this invariant.
func TestCORSMiddleware_Admission503GetsCORSHeaders(t *testing.T) {
	cfg := enabledCORSConfig()

	// Create an admission controller with limit 1 and immediately fill it
	// so every subsequent request is rejected with 503.
	admission := NewSemaphoreAdmission(1)
	acquired := admission.TryAcquire()
	require.True(t, acquired, "pre-fill must succeed on a fresh semaphore")

	// The inner handler must never be reached — admission rejects first.
	var handlerCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Build the middleware chain: CORS(Admission(handler)).
	chain := CORSMiddleware(cfg)(AdmissionMiddleware(admission)(handler))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	require.False(t, handlerCalled, "handler must not be called when admission rejects")
	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	require.Equal(t, "http://example.com", rr.Header().Get("Access-Control-Allow-Origin"),
		"CORS headers must be present on admission 503 responses")
	require.Contains(t, rr.Header().Values("Vary"), "Origin")
	require.Contains(t, rr.Body.String(), "service at capacity")

	retryAfter := rr.Header().Get("Retry-After")
	require.NotEmpty(t, retryAfter, "Retry-After header must be present on 503")
	retryVal, err := strconv.Atoi(retryAfter)
	require.NoError(t, err, "Retry-After must be a valid integer")
	require.GreaterOrEqual(t, retryVal, 1)
	require.LessOrEqual(t, retryVal, 3)
}

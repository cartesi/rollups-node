// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// captureLogger returns a logger whose output is written to buf. Level is set
// to debug so every call is recorded.
func captureLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestHTTPServerOptions_PresetValues(t *testing.T) {
	inspect := DefaultInspectOptions()
	require.Equal(t, 10*time.Second, inspect.ReadHeaderTimeout)
	require.Equal(t, 30*time.Second, inspect.ReadTimeout)
	require.Equal(t, 600*time.Second, inspect.WriteTimeout)
	require.Equal(t, 60*time.Second, inspect.IdleTimeout)
	require.Equal(t, 64*1024, inspect.MaxHeaderBytes)

	telemetry := DefaultTelemetryOptions()
	require.Equal(t, 5*time.Second, telemetry.ReadHeaderTimeout)
	require.Equal(t, 10*time.Second, telemetry.ReadTimeout)
	require.Equal(t, 10*time.Second, telemetry.WriteTimeout)
	require.Equal(t, 60*time.Second, telemetry.IdleTimeout)
	require.Equal(t, 16*1024, telemetry.MaxHeaderBytes)

	jsonrpc := DefaultJSONRPCOptions()
	require.Equal(t, 10*time.Second, jsonrpc.ReadHeaderTimeout)
	require.Equal(t, 30*time.Second, jsonrpc.ReadTimeout)
	require.Equal(t, 30*time.Second, jsonrpc.WriteTimeout)
	require.Equal(t, 60*time.Second, jsonrpc.IdleTimeout)
	require.Equal(t, 64*1024, jsonrpc.MaxHeaderBytes)
}

// TestDefaultInspectOptions_ReturnsFreshCopy pins that the preset constructor
// hands every caller an independent value. Mutating one returned struct must
// not affect the next call's result.
func TestDefaultInspectOptions_ReturnsFreshCopy(t *testing.T) {
	first := DefaultInspectOptions()
	first.WriteTimeout = 0
	require.Equal(t, time.Duration(0), first.WriteTimeout)
	second := DefaultInspectOptions()
	require.Equal(t, 600*time.Second, second.WriteTimeout)
}

// TestInspectWriteTimeoutExceedsMachineDeadline pins that the backstop
// WriteTimeout is well above any realistic machine deadline.
func TestInspectWriteTimeoutExceedsMachineDeadline(t *testing.T) {
	require.Greater(t, DefaultInspectOptions().WriteTimeout, 180*time.Second)
}

func TestNewHTTPServer_AppliesOptions(t *testing.T) {
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	opts := DefaultInspectOptions()
	srv := NewHTTPServer(":0", handler, opts, discardLogger())

	require.Equal(t, ":0", srv.Addr)
	require.NotNil(t, srv.Handler)
	require.Equal(t, opts.ReadHeaderTimeout, srv.ReadHeaderTimeout)
	require.Equal(t, opts.ReadTimeout, srv.ReadTimeout)
	require.Equal(t, opts.WriteTimeout, srv.WriteTimeout)
	require.Equal(t, opts.IdleTimeout, srv.IdleTimeout)
	require.Equal(t, opts.MaxHeaderBytes, srv.MaxHeaderBytes)
}

func TestNewHTTPServer_ErrorLogWiring(t *testing.T) {
	var buf bytes.Buffer
	srv := NewHTTPServer(":0", http.NotFoundHandler(), DefaultInspectOptions(), captureLogger(&buf))
	require.NotNil(t, srv.ErrorLog)
	// Write through the ErrorLog and check it lands in the captured output.
	srv.ErrorLog.Print("test-error-log-line")
	require.Contains(t, buf.String(), "test-error-log-line")
}

func TestStartupBindWarning_UnspecifiedV4(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "0.0.0.0:10012")
	require.Contains(t, buf.String(), "bound to all interfaces")
	require.Contains(t, buf.String(), "0.0.0.0:10012")
}

func TestStartupBindWarning_UnspecifiedV6(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "[::]:10012")
	require.Contains(t, buf.String(), "bound to all interfaces")
}

func TestStartupBindWarning_EmptyHost(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", ":10012")
	require.Contains(t, buf.String(), "bound to all interfaces")
}

func TestStartupBindWarning_Localhost(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "127.0.0.1:10012")
	require.Empty(t, buf.String())
}

func TestStartupBindWarning_LocalhostV6(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "[::1]:10012")
	require.Empty(t, buf.String())
}

func TestStartupBindWarning_Hostname(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "myhost:10012")
	require.Empty(t, buf.String())
}

func TestStartupBindWarning_PrivateV4_192(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "192.168.1.10:10012")
	require.Contains(t, buf.String(), "level=WARN")
	require.Contains(t, buf.String(), "private/link-local")
	require.Contains(t, buf.String(), "192.168.1.10:10012")
}

func TestStartupBindWarning_PrivateV4_10(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "10.0.0.5:10012")
	require.Contains(t, buf.String(), "level=WARN")
	require.Contains(t, buf.String(), "private/link-local")
	require.Contains(t, buf.String(), "10.0.0.5:10012")
}

func TestStartupBindWarning_PrivateV4_172(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "172.20.3.4:10012")
	require.Contains(t, buf.String(), "level=WARN")
	require.Contains(t, buf.String(), "private/link-local")
}

func TestStartupBindWarning_PrivateV6_ULA(t *testing.T) {
	var buf bytes.Buffer
	StartupBindWarning(captureLogger(&buf), "test", "[fc00::1]:10012")
	require.Contains(t, buf.String(), "level=WARN")
	require.Contains(t, buf.String(), "private/link-local")
}

func TestStartupBindWarning_LinkLocalV6(t *testing.T) {
	var buf bytes.Buffer
	// Using a bare link-local literal: net.SplitHostPort accepts zone ids
	// but net.ParseIP does not, so we use the zone-less form here.
	StartupBindWarning(captureLogger(&buf), "test", "[fe80::1]:10012")
	require.Contains(t, buf.String(), "level=WARN")
	require.Contains(t, buf.String(), "private/link-local")
}

func TestStartupBindWarning_MalformedAddr(t *testing.T) {
	var buf bytes.Buffer
	// SplitHostPort fails: we log at INFO so the operator sees the helper
	// bailed on their (likely mistyped) address rather than silently
	// making no statement.
	StartupBindWarning(captureLogger(&buf), "test", "not-a-valid-addr")
	out := buf.String()
	require.Contains(t, out, "level=INFO")
	require.Contains(t, out, "could not parse address")
	require.Contains(t, out, "not-a-valid-addr")
	require.NotContains(t, out, "level=WARN")
}

func TestWriteInternalError_BodyFormat(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteInternalError(context.Background(), rr, discardLogger(), errors.New("inner"))

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
	require.Equal(t, "Internal server error (request_id=)\n", rr.Body.String())
}

func TestWriteInternalError_BodyFormatWithID(t *testing.T) {
	rr := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ctxKeyRequestID{}, "abc-123")
	WriteInternalError(ctx, rr, discardLogger(), errors.New("inner"))

	require.Equal(t, "Internal server error (request_id=abc-123)\n", rr.Body.String())
}

func TestWriteInternalError_NeverLeaksErrText(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteInternalError(context.Background(), rr, discardLogger(), errors.New("secret-detail-12345"))

	require.NotContains(t, rr.Body.String(), "secret-detail-12345")
}

func TestWriteInternalError_LogsDetail(t *testing.T) {
	var buf bytes.Buffer
	rr := httptest.NewRecorder()
	ctx := context.WithValue(context.Background(), ctxKeyRequestID{}, "log-id-9")
	WriteInternalError(ctx, rr, captureLogger(&buf), errors.New("secret-detail-12345"))

	logged := buf.String()
	require.Contains(t, logged, "http internal error")
	require.Contains(t, logged, "secret-detail-12345")
	require.Contains(t, logged, "log-id-9")
}

func TestWriteInternalError_StatusAndContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteInternalError(context.Background(), rr, discardLogger(), errors.New("x"))

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
	// Defense-in-depth: match http.Error's behaviour so content-sniffing
	// clients cannot reinterpret the text/plain body as HTML.
	require.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	// Sanity: body starts with the canonical prefix.
	require.True(t, strings.HasPrefix(rr.Body.String(), "Internal server error (request_id="))
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	require.Equal(t, "", RequestIDFromContext(context.Background()))
}

func TestRequestIDFromContext_Set(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKeyRequestID{}, "xyz")
	require.Equal(t, "xyz", RequestIDFromContext(ctx))
}

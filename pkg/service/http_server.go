// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// HTTPServerOptions bundles the hardening knobs applied to an [http.Server].
//
// Every field is required. Callers typically start from one of the package-level
// preset constructors ([DefaultInspectOptions], [DefaultTelemetryOptions],
// [DefaultJSONRPCOptions]) and mutate fields on the returned value.
type HTTPServerOptions struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// DefaultInspectOptions returns a fresh [HTTPServerOptions] preset for the
// inspect HTTP surface. WriteTimeout is 600s (10 minutes) and serves as a
// process-health backstop only — it prevents leaked goroutines from holding
// a connection forever. The actual per-request deadline is set structurally
// in Inspector.ServeHTTP as InspectMaxDeadline + inspectResponseHeadroom.
//
// Each call returns an independent value; callers may mutate it freely
// without affecting other callers.
//
//nolint:mnd // These timers are the canonical source of these numbers.
func DefaultInspectOptions() HTTPServerOptions {
	return HTTPServerOptions{
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      600 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}
}

// DefaultTelemetryOptions returns a fresh [HTTPServerOptions] preset for the
// telemetry HTTP surface. Each call returns an independent value; callers
// may mutate it freely without affecting other callers.
//
//nolint:mnd // These timers are the canonical source of these numbers.
func DefaultTelemetryOptions() HTTPServerOptions {
	return HTTPServerOptions{
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
}

// DefaultJSONRPCOptions returns a fresh [HTTPServerOptions] preset for the
// JSON-RPC HTTP surface. Each call returns an independent value; callers
// may mutate it freely without affecting other callers.
//
//nolint:mnd // These timers are the canonical source of these numbers.
func DefaultJSONRPCOptions() HTTPServerOptions {
	return HTTPServerOptions{
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}
}

// NewHTTPServer builds an [http.Server] from opts with an ErrorLog routed
// through logger.
func NewHTTPServer(
	addr string,
	handler http.Handler,
	opts HTTPServerOptions,
	logger *slog.Logger,
) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: opts.ReadHeaderTimeout,
		ReadTimeout:       opts.ReadTimeout,
		WriteTimeout:      opts.WriteTimeout,
		IdleTimeout:       opts.IdleTimeout,
		MaxHeaderBytes:    opts.MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

// StartupBindWarning logs a warning when addr binds to an address that
// exposes the HTTP service beyond the local host. Specifically:
//
//   - Unspecified address (0.0.0.0, ::, or an empty host like ":10012") —
//     WARN: reachable on every interface.
//   - Private RFC1918/RFC4193 address (192.168/16, 10/8, 172.16/12, ULA
//     fc00::/7) or IPv6 link-local (fe80::/10) — WARN: reachable by other
//     hosts on the same LAN/VPC, which is the most common multi-tenant
//     foot-gun.
//
// Loopback (127.0.0.0/8, ::1) and public/hostname binds stay silent: the
// former is the recommended posture, the latter is assumed to be a
// deliberate choice the operator has already reasoned about.
//
// If [net.SplitHostPort] cannot parse addr, this logs at INFO so operator
// typos like "127.0.0.1.10012" (missing colon) are visible — the helper
// made no statement, and the subsequent ListenAndServe will fail loudly.
func StartupBindWarning(logger *slog.Logger, serviceName, addr string) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		logger.Info(
			"StartupBindWarning could not parse address; skipping bind-exposure check",
			"service", serviceName,
			"addr", addr,
			"err", err,
		)
		return
	}
	var ip net.IP
	if host != "" {
		ip = net.ParseIP(host)
	}
	switch {
	case host == "" || (ip != nil && ip.IsUnspecified()):
		logger.Warn(
			"HTTP service bound to all interfaces; restrict access via firewall or reverse proxy",
			"service", serviceName,
			"addr", addr,
		)
	case ip != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast()):
		logger.Warn(
			"HTTP service bound to a private/link-local address; restrict access via firewall or reverse proxy",
			"service", serviceName,
			"addr", addr,
		)
	}
	// Loopback, public literal, or non-IP hostname: stay silent.
}

// ctxKeyRequestID is the context key under which [RequestIDMiddleware] stores
// the validated or generated X-Request-ID value.
type ctxKeyRequestID struct{}

// RequestIDFromContext returns the request id stored by [RequestIDMiddleware],
// or the empty string if none is present.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxKeyRequestID{}).(string); ok {
		return v
	}
	return ""
}

// WriteInternalError writes a generic 500 response and logs the detailed
// error. The response body is exactly:
//
//	Internal server error (request_id=<id>)\n
//
// where <id> comes from [RequestIDFromContext] (empty if no middleware has
// populated it). The caller's err is never echoed onto the wire — it only
// appears in the structured log under the "err" key. The log message is a
// fixed string ("http internal error") so operators can grep reliably on
// the structured field rather than a per-call-site prefix.
func WriteInternalError(
	ctx context.Context,
	w http.ResponseWriter,
	logger *slog.Logger,
	err error,
) {
	reqID := RequestIDFromContext(ctx)
	logger.Error("http internal error",
		"err", err,
		"request_id", reqID,
	)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusInternalServerError)
	// gosec G705 is a false positive here: the response is text/plain with
	// nosniff, and reqID is validated upstream by RequestIDMiddleware (requestIDPattern),
	// so no taint can reach a browser as HTML.
	_, _ = fmt.Fprintf(w, "Internal server error (request_id=%s)\n", reqID) //nolint:gosec // G705
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bufio"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// CORSConfig holds the pre-validated, pre-joined CORS policy for a single
// HTTP surface (JSON-RPC or inspect). When the allowedOrigins map is empty
// the config is disabled and the middleware is a no-op pass-through.
type CORSConfig struct {
	allowedOrigins map[string]struct{} // lowercased at construction
	methods        string              // pre-joined for header value
	headers        string              // pre-joined for header value
	maxAge         string              // "3600"
}

// Enabled reports whether the config has at least one valid allowed origin.
func (c CORSConfig) Enabled() bool {
	return len(c.allowedOrigins) > 0
}

// ParseCORSConfig splits origins on comma, validates each entry, and returns
// a ready-to-use [CORSConfig]. Invalid entries are logged as warnings and
// skipped. If no valid origins remain, the returned config is disabled
// ([CORSConfig.Enabled] returns false).
//
// Validation rules per origin:
//   - Empty/whitespace-only entries are silently skipped.
//   - "null" is rejected (sandboxed iframe attack vector).
//   - Entries with paths, query strings, or fragments are rejected.
//   - A single trailing slash is stripped (bare origin normalization).
//   - The entire origin is lowercased per RFC 6454.
func ParseCORSConfig(logger *slog.Logger, origins string, methods []string, headers []string) CORSConfig {
	cfg := CORSConfig{
		allowedOrigins: make(map[string]struct{}),
		methods:        strings.Join(methods, ", "),
		headers:        strings.Join(headers, ", "),
		maxAge:         "3600",
	}

	for raw := range strings.SplitSeq(origins, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}

		if strings.EqualFold(entry, "null") {
			logger.Warn("CORS origin \"null\" rejected (sandboxed iframe attack vector)",
				"origin", entry,
			)
			continue
		}

		entry = strings.TrimSuffix(entry, "/")

		if scheme, rest, ok := strings.Cut(entry, "://"); ok {
			_ = scheme
			if strings.ContainsAny(rest, "/?#") {
				logger.Warn("CORS origin rejected: must not contain path, query, or fragment",
					"origin", raw,
				)
				continue
			}
		} else if strings.ContainsAny(entry, "/?#") {
			logger.Warn("CORS origin rejected: must not contain path, query, or fragment",
				"origin", raw,
			)
			continue
		}

		lower := strings.ToLower(entry)
		cfg.allowedOrigins[lower] = struct{}{}
	}

	if len(cfg.allowedOrigins) == 0 {
		cfg.allowedOrigins = nil
	}
	return cfg
}

// corsWriter wraps an [http.ResponseWriter] and injects CORS headers at
// WriteHeader time, after the handler has had a chance to set (or
// overwrite) response headers. This ensures CORS headers are authoritative
// regardless of what downstream handlers do — including setting their own
// Access-Control-Allow-Origin, which the wrapper strips and replaces.
type corsWriter struct {
	http.ResponseWriter
	origin  string
	cfg     CORSConfig
	written bool
}

func (cw *corsWriter) setCORSHeaders() {
	h := cw.Header()
	h.Add("Vary", "Origin")
	h.Del("Access-Control-Allow-Origin")
	h.Set("Access-Control-Allow-Origin", cw.origin)
	h.Set("Access-Control-Allow-Methods", cw.cfg.methods)
	h.Set("Access-Control-Allow-Headers", cw.cfg.headers)
}

func (cw *corsWriter) WriteHeader(code int) {
	if !cw.written {
		cw.written = true
		cw.setCORSHeaders()
	}
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *corsWriter) Write(b []byte) (int, error) {
	if !cw.written {
		cw.written = true
		cw.setCORSHeaders()
	}
	return cw.ResponseWriter.Write(b)
}

func (cw *corsWriter) Unwrap() http.ResponseWriter { return cw.ResponseWriter }

func (cw *corsWriter) Flush() {
	if !cw.written {
		cw.written = true
		cw.setCORSHeaders()
	}
	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (cw *corsWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := cw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// CORSMiddleware returns a middleware that enforces the given [CORSConfig].
// When the config is disabled ([CORSConfig.Enabled] returns false), the
// returned middleware is a no-op identity wrapper — the same pattern used
// by [AdmissionMiddleware] when its controller is nil.
//
// Preflight requests (OPTIONS with Access-Control-Request-Method) from
// allowed origins are short-circuited with a 204 before downstream
// handlers (and therefore before admission control) run.
func CORSMiddleware(cfg CORSConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled() {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Short-circuit ALL preflights before admission. Allowed
			// origins get CORS grant headers; disallowed origins get
			// Vary only. Either way, downstream handlers (and their
			// admission permits) are never touched.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Add("Vary", "Origin")
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				lower := strings.ToLower(origin)
				if _, ok := cfg.allowedOrigins[lower]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", cfg.methods)
					w.Header().Set("Access-Control-Allow-Headers", cfg.headers)
					w.Header().Set("Access-Control-Max-Age", cfg.maxAge)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			lower := strings.ToLower(origin)
			if _, ok := cfg.allowedOrigins[lower]; !ok {
				// Disallowed origin: set Vary for caching correctness
				// but emit no CORS grant headers.
				w.Header().Add("Vary", "Origin")
				next.ServeHTTP(w, r)
				return
			}

			// Non-preflight allowed origin: wrap the response writer so
			// CORS headers (including Vary: Origin) are injected at
			// WriteHeader time, after the handler has had a chance to
			// set (and potentially overwrite) headers.
			cw := &corsWriter{ResponseWriter: w, origin: origin, cfg: cfg}
			next.ServeHTTP(cw, r)

			// If the handler returned without calling WriteHeader or
			// Write (e.g., an empty 200), inject CORS headers now so
			// they are present when the stdlib flushes.
			if !cw.written {
				cw.setCORSHeaders()
			}
		})
	}
}

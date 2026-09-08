// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

type HTTPServiceTemplate struct {
	BaseTemplate
	// TODO: this should not be exported (but is used by tests in other packages)
	Server *http.Server
	// Listen opens the HTTP listener. It defaults to net.Listen and is
	// overridden in tests so Serve() can be exercised without real sockets.
	Listen func(network, address string) (net.Listener, error)
	// Admission is an optional HTTP-level concurrency gate. A nil value
	// disables Admission control; the middleware chain treats nil as a
	// pass-through so wiring is uniform regardless of configuration.
	Admission *SemaphoreAdmission
	// Maximum duration for the service to wait for in-flight requests to
	// complete and shutdown.
	shutdownTimeout time.Duration
}

type HTTPServiceConfigs struct {
	BaseConfigs
	HTTPServerOptions
	// HTTP address for JSON-RPC's telemetry service.
	Address string
	// Enforces request IDs are in the charset that cover the ID formats emitted
	// by common reverse proxies and tracing systems while remaining safe to log
	// and echo in response headers.
	SafeRequestID bool
	// Comma-separated list of allowed browser origins for the HTTP service.
	// If empty, CORS is disabled. Origins are lowercased and validated at
	// startup. Example: "http://localhost:3000,https://app.example.com".
	CorsAllowedOrigins string
	// Maximum number of concurrent in-flight JSON-RPC requests.
	// Requests beyond this limit receive HTTP 503 Service Unavailable
	// with Retry-After: 1. Set to 0 to disable HTTP-level admission
	// control.
	MaxInflight uint64
	// Maximum duration for the service to wait for in-flight requests to
	// complete and shutdown.
	ShutdownTimeout time.Duration
}

func InitHTTPServiceTemplate(
	tmpl *HTTPServiceTemplate,
	cfg *HTTPServiceConfigs,
	handler http.Handler,
) {
	if handler == nil {
		panic("http.Handler is nil")
	}

	InitServiceTemplate(&tmpl.BaseTemplate, &cfg.BaseConfigs)

	if cfg.MaxInflight > 0 {
		tmpl.Admission = NewSemaphoreAdmission(cfg.MaxInflight)
		handler = AdmissionMiddleware(tmpl.Admission)(handler)
	}
	if cfg.CorsAllowedOrigins != "" {
		corsCfg := ParseCORSConfig(
			tmpl.Logger,
			cfg.CorsAllowedOrigins,
			[]string{"POST", "OPTIONS"},
			[]string{"Content-Type"},
		)
		handler = CORSMiddleware(corsCfg)(handler)
	}
	if cfg.SafeRequestID {
		handler = RequestIDMiddleware(handler)
	}

	handler = RecoverMiddleware(tmpl.Logger)(handler)

	tmpl.shutdownTimeout = cfg.ShutdownTimeout
	tmpl.Listen = net.Listen
	tmpl.Server = NewHTTPServer(cfg.Address, handler, cfg.HTTPServerOptions, tmpl.Logger)
	StartupBindWarning(tmpl.Logger, cfg.Name, cfg.Address)
}

func (s *HTTPServiceTemplate) Serve(ctx context.Context) error {
	listener, err := s.Listen("tcp", s.Server.Addr)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		s.Logger.Info("Shutting down HTTP service", "addr", s.Server.Addr)
		ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := s.Server.Shutdown(ctx); err != nil {
			s.Logger.Error("Failure on HTTP service shut down", "err", err)
		}
	}()

	s.Logger.Info("Starting HTTP service", "addr", s.Server.Addr)
	if err := s.Server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return ctx.Err()
}

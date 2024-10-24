// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
)

// FIXME: Simple CORS middleware. Improve this
func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all origins
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight (OPTIONS) request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Proceed with the next handler if not preflight
		next.ServeHTTP(w, r)
	})
}

type HttpService struct {
	Name    string
	Address string
	Handler http.Handler
}

func (s *HttpService) String() string {
	return s.Name
}

func (s *HttpService) Start(ctx context.Context, ready chan<- struct{}) error {
	server := http.Server{
		Addr:     s.Address,
		Handler:  CorsMiddleware(s.Handler),
		ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
	}

	listener, err := net.Listen("tcp", s.Address)
	if err != nil {
		return err
	}

	slog.Info("HTTP server started listening", "service", s, "port", listener.Addr())
	ready <- struct{}{}

	done := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("Service exited with error", "service", s, "error", err)
		}
		done <- err
	}()

	select {
	case err = <-done:
		return err
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), DefaultServiceTimeout)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

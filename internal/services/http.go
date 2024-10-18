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
		Handler:  s.Handler,
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

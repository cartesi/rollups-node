// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/cartesi/rollups-node/internal/config"
)

type HttpService struct {
	Name    string
	Address string
	Handler http.Handler
}

func (s HttpService) String() string {
	return s.Name
}

func (s HttpService) Start(ctx context.Context, ready chan<- struct{}) error {
	server := http.Server{
		Addr:     s.Address,
		Handler:  s.Handler,
		ErrorLog: config.ErrorLogger,
	}

	listener, err := net.Listen("tcp", s.Address)
	if err != nil {
		return err
	}

	config.InfoLogger.Printf("%v: listening at %v\n", s, listener.Addr())
	ready <- struct{}{}

	done := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			config.WarningLogger.Printf("%v: %v", s, err)
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

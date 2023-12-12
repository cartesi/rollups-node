// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/cartesi/rollups-node/internal/logger"
)

// A service that provides a simple liveness check via HTTP.
// Implements service.Service and fmt.Stringer
type HealthService struct {
	Addr string
	Port int
}

func (s HealthService) Start(ctx context.Context) error {
	var server = &http.Server{Addr: fmt.Sprintf("%v:%v", s.Addr, s.Port)}

	var errChannel = make(chan error)
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "ok")
		})
		logger.Info.Printf("Starting %v at %v:%v", s, s.Addr, s.Port)
		errChannel <- server.ListenAndServe()
	}()

	select {
	case err := <-errChannel:
		return err
	case <-ctx.Done():
		if err := server.Close(); err != nil {
			logger.Warning.Printf("%v: error while closing: %v\n", s.String(), err)
		}
		return ctx.Err()
	}
}

func (s HealthService) Ready(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		conn, err := net.Dial("tcp", fmt.Sprintf("%v:%v", s.Addr, s.Port))
		if err == nil {
			logger.Debug.Printf("%s is ready\n", s)
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(DefaultDialInterval):
		}
	}
}

func (s HealthService) String() string {
	return "HealthService"
}

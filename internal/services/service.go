// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package services provides mechanisms to start multiple services in the
// background
package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/logger"
)

const (
	DefaultServiceTimeout = 15 * time.Second
	DefaultDialInterval   = 100 * time.Millisecond
)

type serviceLogger struct {
	Name string
}

func (l serviceLogger) Write(data []byte) (int, error) {
	logger.Info.Printf("%v: %v", l.Name, string(data))
	return len(data), nil
}

type Service interface {
	fmt.Stringer

	// Start will execute a binary and wait for its completion or until the context
	// is canceled
	Start(ctx context.Context) error

	// Ready blocks until the service is ready or the context is canceled.
	//
	// A service is considered ready when it is possible to establish a connection
	// to its healthcheck endpoint.
	Ready(ctx context.Context, timeout time.Duration) error
}

// The Run function serves as a very simple supervisor: it will start all the
// services provided to it and will run until the first of them finishes. Next
// it will try to stop the remaining services or timeout if they take too long
func Run(ctx context.Context, services []Service) {
	if len(services) == 0 {
		logger.Error.Panic("there are no services to run")
	}

	// start services
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for _, service := range services {
		service := service
		wg.Add(1)
		go func() {
			// cancel the context when one of the services finish
			defer cancel()
			defer wg.Done()
			if err := service.Start(ctx); err != nil {
				msg := "main: service '%v' exited with error: %v\n"
				logger.Error.Printf(msg, service, err)
			} else {
				msg := "main: service '%v' exited successfully\n"
				logger.Info.Printf(msg, service)
			}
		}()

		// wait for service to be ready or stop all services if it times out
		if err := service.Ready(ctx, DefaultServiceTimeout); err != nil {
			cancel()
			msg := "main: service '%v' failed to be ready with error: %v. Exiting\n"
			logger.Error.Printf(msg, service, err)
			break
		}
	}

	// wait until the context is canceled
	<-ctx.Done()

	// wait for the services to finish or timeout
	wait := make(chan struct{})
	go func() {
		wg.Wait()
		wait <- struct{}{}
	}()
	select {
	case <-wait:
		logger.Info.Println("main: all services were shutdown")
	case <-time.After(DefaultServiceTimeout):
		logger.Warning.Println("main: exited after a timeout")
	}
}

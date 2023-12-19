// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package services provides mechanisms to start multiple services in the
// background
package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
)

const (
	DefaultServiceTimeout = 15 * time.Second
)

type Service interface {
	fmt.Stringer

	// Starts a service and sends a message to the channel when ready
	Start(ctx context.Context, ready chan<- struct{}) error
}

// The Run function serves as a very simple supervisor: it will start all the
// services provided to it and will run until the first of them finishes. Next
// it will try to stop the remaining services or timeout if they take too long
func Run(ctx context.Context, services []Service) {
	if len(services) == 0 {
		config.ErrorLogger.Panic("there are no services to run")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	ready := make(chan struct{})

	// start services
	for _, service := range services {
		service := service
		wg.Add(1)
		go func() {
			// cancel the context when one of the services finish
			defer cancel()
			defer wg.Done()

			err := service.Start(ctx, ready)
			if err != nil && !errors.Is(err, context.Canceled) {
				msg := "main: service '%v' exited with error: %v\n"
				config.InfoLogger.Printf(msg, service, err)
			}
		}()

		select {
		case <-ready:
		case <-ctx.Done():
			break
		case <-time.After(DefaultServiceTimeout):
			cancel()
			config.ErrorLogger.Printf("main: service '%v' timed out\n", service)
			break
		}
	}

	// wait for context to be done
	if ctx.Err() == nil {
		config.InfoLogger.Printf("main: all services have started")
	}
	<-ctx.Done()

	// wait for the services to finish or timeout
	wait := make(chan struct{})
	go func() {
		wg.Wait()
		wait <- struct{}{}
	}()
	select {
	case <-wait:
		config.InfoLogger.Println("main: all services were shutdown")
	case <-time.After(DefaultServiceTimeout):
		config.WarningLogger.Println("main: exited after a timeout")
	}
}

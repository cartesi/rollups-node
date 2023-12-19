// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package services provides mechanisms to start multiple services in the
// background
package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sync"
	"syscall"
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

type Service struct {

	// Name that identifies the service.
	Name string

	// Port used to verify if the service is ready.
	HealthcheckPort int

	// Path to the service binary.
	Path string

	// Args to the service binary.
	Args []string

	// Environment variables.
	Env []string
}

// Start will execute a binary and wait for its completion or until the context
// is canceled
func (s Service) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, s.Path, s.Args...)
	cmd.Env = s.Env
	cmd.Stderr = serviceLogger{s.Name}
	cmd.Stdout = serviceLogger{s.Name}
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			msg := "failed to send SIGTERM to %v: %v\n"
			logger.Warning.Printf(msg, s.Name, err)
		}
		return err
	}

	err := cmd.Run()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Ready blocks until the service is ready or the context is canceled.
//
// A service is considered ready when it is possible to establish a connection
// to its healthcheck endpoint.
func (s Service) Ready(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		conn, err := net.Dial("tcp", fmt.Sprintf("0.0.0.0:%v", s.HealthcheckPort))
		if err == nil {
			logger.Debug.Printf("%s is ready\n", s.Name)
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

func (s Service) String() string {
	return s.Name
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
			if err := service.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				msg := "main: service '%v' exited with error: %v\n"
				logger.Error.Printf(msg, service.String(), err)
			} else {
				msg := "main: service '%v' exited successfully\n"
				logger.Info.Printf(msg, service.String())
			}
		}()

		// wait for service to be ready or stop all services if it times out
		if err := service.Ready(ctx, DefaultServiceTimeout); err != nil {
			cancel()
			msg := "main: service '%v' failed to be ready with error: %v. Exiting\n"
			logger.Error.Printf(msg, service.Name, err)
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

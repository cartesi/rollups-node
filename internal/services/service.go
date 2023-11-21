// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package services provides mechanisms to start multiple services in the
// background
package services

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/logger"
)

const DefaultServiceTimeout = 15 * time.Second

type Service struct {
	name       string
	binaryName string
}

// Start will execute a binary and wait for its completion or until the context
// is canceled
func (s Service) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, s.binaryName)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			msg := "failed to send SIGTERM to %v: %v\n"
			logger.Warning.Printf(msg, s.name, err)
		}
		return err
	}
	err := cmd.Run()
	if err != nil {
		exitCode := cmd.ProcessState.ExitCode()
		signal := cmd.ProcessState.Sys().(syscall.WaitStatus).Signal()
		if exitCode != 0 && signal != syscall.SIGTERM {
			// only return error if the service exits for reason other than shutdown
			return err
		}
	}
	return nil
}

func (s Service) String() string {
	return s.name
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
				logger.Error.Printf(msg, service.String(), err)
			} else {
				msg := "main: service '%v' exited successfully\n"
				logger.Info.Printf(msg, service.String())
			}
		}()
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

var (
	GraphQLServer Service = Service{
		name:       "graphql-server",
		binaryName: "cartesi-rollups-graphql-server",
	}
	Indexer Service = Service{
		name:       "indexer",
		binaryName: "cartesi-rollups-indexer",
	}
)

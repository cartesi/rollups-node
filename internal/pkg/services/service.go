// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Provide mechanisms to start multiple services in the background
package services

import (
	"context"
	"fmt"
	"time"
)

// A service that runs in the background endlessly until the context is canceled
type Service interface {
	fmt.Stringer

	// Start a service that will run until completion or until the context is
	// canceled
	Start(ctx context.Context) error
}

const DefaultServiceTimeout = 15 * time.Second

// The Run function serves as a very simple supervisor: it will start all the
// services provided to it and will run until the first of them finishes. Next
// it will try to stop the remaining services or timeout if they take too long
func Run(services []Service) {
	if len(services) == 0 {
		panic("there are no services to run")
	}

	// start services
	ctx, cancel := context.WithCancel(context.Background())
	exit := make(chan struct{})
	for _, service := range services {
		service := service
		go func() {
			if err := service.Start(ctx); err != nil {
				msg := "main: service '%v' exited with error: %v\n"
				fmt.Printf(msg, service.String(), err)
			} else {
				msg := "main: service '%v' exited successfully\n"
				fmt.Printf(msg, service.String())
			}
			exit <- struct{}{}
		}()
	}

	// wait for first service to exit
	<-exit

	// send stop message to all other services and wait for them to finish
	// or timeout
	wait := make(chan struct{})
	go func() {
		cancel()
		for i := 0; i < len(services)-1; i++ {
			<-exit
		}
		wait <- struct{}{}
	}()

	select {
	case <-wait:
		fmt.Println("main: all services exited")
	case <-time.After(DefaultServiceTimeout):
		fmt.Println("main: exited after timeout")
	}
}

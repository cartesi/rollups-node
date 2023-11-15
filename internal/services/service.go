// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package services provides mechanisms to start multiple services in the
// background
package services

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/logger"
)

const (
	DefaultServiceTimeout  = 15 * time.Second
	DefaultDialInterval    = 100 * time.Millisecond
	DefaultStateServerPort = "50051"
	DefaultHealthcheckPort = "8080"
)

type Service struct {
	name            string
	binaryName      string
	healthcheckPort string
}

// Start will execute a binary and wait for its completion or until the context
// is canceled
func (s Service) Start(ctx context.Context) error {
	cmd := exec.Command(s.binaryName)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		logger.Debug.Printf("%v: %v\n", s.String(), ctx.Err())
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			msg := "%v: failed to send SIGTERM to %v\n"
			logger.Error.Printf(msg, s.String(), s.name)
		}
	}()

	err := cmd.Wait()
	if err != nil && cmd.ProcessState.ExitCode() != int(syscall.SIGTERM) {
		return err
	}
	return nil
}

// Ready blocks until the service is ready or the context is canceled.
//
// A service is considered ready when it is possible to establish a connection
// to its healthcheck endpoint.
func (s Service) Ready(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			conn, err := net.Dial("tcp", fmt.Sprintf("0.0.0.0:%s", s.healthcheckPort))
			if err == nil {
				logger.Debug.Printf("%s is ready\n", s.name)
				conn.Close()
				return nil
			}
			time.Sleep(DefaultDialInterval)
		}
	}
}

func (s Service) String() string {
	return s.name
}

// The Run function serves as a very simple supervisor: it will start all the
// services provided to it and will run until the first of them finishes. Next
// it will try to stop the remaining services or timeout if they take too long
func Run(services []Service) {
	if len(services) == 0 {
		logger.Error.Panic("there are no services to run")
	}

	// start services
	ctx, cancel := context.WithCancel(context.Background())
	startedServicesCount := 0
	exit := make(chan struct{}, len(services))
	for _, service := range services {
		service := service
		go func() {
			if err := service.Start(ctx); err != nil {
				msg := "main: service '%v' exited with error: %v\n"
				logger.Error.Printf(msg, service.String(), err)
			} else {
				msg := "main: service '%v' exited successfully\n"
				logger.Info.Printf(msg, service.String())
			}
			exit <- struct{}{}
		}()

		// wait for service to be ready or stop all services if it times out
		readyCtx, readyCancel := context.WithTimeout(ctx, DefaultServiceTimeout)
		defer readyCancel()
		if err := service.Ready(readyCtx); err != nil {
			msg := "main: service '%v' failed to be ready with error: %v. Exiting\n"
			logger.Error.Printf(msg, service.name, err)
			exit <- struct{}{}
			break
		}
		startedServicesCount++
	}

	// wait for first service to exit
	<-exit

	// send stop message to all other services and wait for them to finish
	// or timeout
	wait := make(chan struct{})
	go func() {
		cancel()
		for i := 0; i < startedServicesCount; i++ {
			<-exit
		}
		wait <- struct{}{}
	}()

	select {
	case <-wait:
		logger.Info.Println("main: all services were shutdown")
	case <-time.After(DefaultServiceTimeout):
		logger.Warning.Println("main: exited after a timeout")
	}
}

func healthcheckPort(serviceName string) string {
	if serviceName == "state-server" {
		if address, ok := os.LookupEnv("SS_SERVER_ADDRESS"); ok {
			split := strings.Split(address, ":")
			if len(split) > 1 {
				return split[1]
			}
		}
		return DefaultStateServerPort
	}

	env := healthcheckEnv(serviceName)
	if port, ok := os.LookupEnv(env); ok {
		if serviceName == "state-server" {
			split := strings.Split(port, ":")
			return split[1]
		}
		return port
	} else {
		return DefaultHealthcheckPort
	}
}

func healthcheckEnv(serviceName string) string {
	suffix := "_HEALTHCHECK_PORT"
	if serviceName == "dispatcher" || serviceName == "authority-claimer" {
		suffix = "_HTTP_SERVER_PORT"
	}
	normalizedName := strings.Replace(serviceName, "-", "_", -1)
	return fmt.Sprintf("%s%s", strings.ToUpper(normalizedName), suffix)
}

var (
	AdvanceRunner = Service{
		name:            "advance-runner",
		binaryName:      "cartesi-rollups-advance-runner",
		healthcheckPort: healthcheckPort("advance-runner"),
	}
	AuthorityClaimer = Service{
		name:            "authority-claimer",
		binaryName:      "cartesi-rollups-authority-claimer",
		healthcheckPort: healthcheckPort("authority-claimer"),
	}
	Dispatcher = Service{
		name:            "dispatcher",
		binaryName:      "cartesi-rollups-dispatcher",
		healthcheckPort: healthcheckPort("dispatcher"),
	}
	GraphQLServer Service = Service{
		name:            "graphql-server",
		binaryName:      "cartesi-rollups-graphql-server",
		healthcheckPort: healthcheckPort("graphql-server"),
	}
	Indexer Service = Service{
		name:            "indexer",
		binaryName:      "cartesi-rollups-indexer",
		healthcheckPort: healthcheckPort("indexer"),
	}
	InspectServer = Service{
		name:            "inspect-server",
		binaryName:      "cartesi-rollups-inspect-server",
		healthcheckPort: healthcheckPort("inspect-server"),
	}
	StateServer = Service{
		name:            "state-server",
		binaryName:      "cartesi-rollups-state-server",
		healthcheckPort: healthcheckPort("state-server"),
	}
)

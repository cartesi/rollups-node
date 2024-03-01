// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package deps provides mechanisms to run Node dependencies using docker
package deps

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	DefaultPostgresDockerImage = "postgres:16-alpine"
	DefaultPostgresPort        = "5432"
	DefaultPostgresPassword    = "password"
	DefaultDevnetDockerImage   = "cartesi/rollups-node-devnet:devel"
	DefaultDevnetPort          = "8545"

	numPostgresCheckReadyAttempts = 2
	pollInterval                  = 5 * time.Second
)

// Struct to hold Node dependencies containers configurations
type DepsConfig struct {
	PostgresDockerImage string
	PostgresPort        string
	PostgresPassword    string
	DevnetDockerImage   string
	DevnetPort          string
}

// Builds a DepsConfig struct with default values
func NewDefaultDepsConfig() *DepsConfig {
	return &DepsConfig{
		DefaultPostgresDockerImage,
		DefaultPostgresPort,
		DefaultPostgresPassword,
		DefaultDevnetDockerImage,
		DefaultDevnetPort,
	}
}

// Struct to represent the Node dependencies containers
type DepsContainers struct {
	containers []testcontainers.Container
	//Literal copies lock value from waitGroup as sync.WaitGroup contains sync.noCopy
	waitGroup *sync.WaitGroup
}

// A dummy Logging to write all Test Containers logs with DEBUG priority
type debugLogging struct{}

func (debug debugLogging) Printf(format string, v ...interface{}) {
	config.DebugLogger.Printf(format, v...)
}

func createHook(containerName string,
	waitGroup *sync.WaitGroup) []testcontainers.ContainerLifecycleHooks {
	return []testcontainers.ContainerLifecycleHooks{
		{
			PostTerminates: []testcontainers.ContainerHook{
				func(ctx context.Context, container testcontainers.Container) error {
					waitGroup.Done()
					return nil
				},
			},
		},
	}

}

// Run starts the Node dependencies containers.
// The returned DepContainers struct can be used to gracefully
// terminate the containers using the Terminate method
func Run(ctx context.Context, depsConfig DepsConfig) (*DepsContainers, error) {
	nolog := debugLogging{}
	var waitGroup sync.WaitGroup

	// wait strategy copied from testcontainers docs
	postgresWaitStrategy := wait.ForLog("database system is ready to accept connections").
		WithOccurrence(numPostgresCheckReadyAttempts).
		WithPollInterval(pollInterval)

	postgresReq := testcontainers.ContainerRequest{
		Image: depsConfig.PostgresDockerImage,
		ExposedPorts: []string{strings.Join([]string{
			depsConfig.PostgresPort, ":5432/tcp"}, "")},
		WaitingFor: postgresWaitStrategy,
		Name:       "rollups-node-dep-postgres",
		Env: map[string]string{
			"POSTGRES_PASSWORD": depsConfig.PostgresPassword,
		},
		LifecycleHooks: createHook("rollups-node-dep-postgres", &waitGroup),
	}

	postgres, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
		Logger:           nolog,
	})

	if err != nil {
		return nil, err
	}
	waitGroup.Add(1)

	devNetReq := testcontainers.ContainerRequest{
		Image:        depsConfig.DevnetDockerImage,
		ExposedPorts: []string{strings.Join([]string{depsConfig.DevnetPort, ":8545/tcp"}, "")},
		WaitingFor:   wait.ForLog("Listening on 0.0.0.0:8545"),
		Name:         "rollups-node-dep-devnet",
		Env: map[string]string{
			"ANVIL_IP_ADDR": "0.0.0.0",
		},
		LifecycleHooks: createHook("rollups-node-dep-devnet", &waitGroup),
	}

	devnet, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: devNetReq,
		Started:          true,
		Logger:           nolog,
	})
	if err != nil {
		return nil, err
	}

	waitGroup.Add(1)

	containers := []testcontainers.Container{postgres, devnet}

	return &DepsContainers{containers, &waitGroup}, nil
}

// Terminate terminates all dependencies containers. This method waits for all the containers
// to terminate or gives an error if it fails to terminate one of the containers
func Terminate(ctx context.Context, depContainers *DepsContainers) error {

	for _, depContainer := range depContainers.containers {
		terr := depContainer.Terminate(ctx)
		if terr != nil {
			return terr
		}
	}
	depContainers.waitGroup.Wait()
	return nil
}

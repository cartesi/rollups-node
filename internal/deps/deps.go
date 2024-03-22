// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package deps provides mechanisms to run Node dependencies using docker
package deps

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	DefaultPostgresDockerImage = "postgres:16-alpine"
	DefaultPostgresPort        = "5432"
	DefaultPostgresPassword    = "password"
	DefaultDevnetDockerImage   = "cartesi/rollups-node-devnet:devel"
	DefaultDevnetPort          = "8545"
	DefaultBlockTime           = "1"

	numPostgresCheckReadyAttempts = 2
	pollInterval                  = 5 * time.Second
)

// Struct to hold Node dependencies containers configurations
type DepsConfig struct {
	Postgres *PostgresConfig
	Devnet   *DevnetConfig
}

type PostgresConfig struct {
	DockerImage string
	Port        string
	Password    string
}

type DevnetConfig struct {
	DockerImage string
	Port        string
	BlockTime   string
}

// Builds a DepsConfig struct with default values
func NewDefaultDepsConfig() *DepsConfig {
	return &DepsConfig{
		&PostgresConfig{
			DefaultPostgresDockerImage,
			DefaultPostgresPort,
			DefaultPostgresPassword,
		},
		&DevnetConfig{
			DefaultDevnetDockerImage,
			DefaultDevnetPort,
			DefaultBlockTime,
		},
	}
}

// Struct to represent the Node dependencies containers
type DepsContainers struct {
	containers []testcontainers.Container
	//Literal copies lock value from waitGroup as sync.WaitGroup contains sync.noCopy
	waitGroup *sync.WaitGroup
}

// debugLogging implements the testcontainers.Logging interface by printing the log to slog.Debug.
type debugLogging struct{}

func (d debugLogging) Printf(format string, v ...interface{}) {
	slog.Debug(fmt.Sprintf(format, v...))
}

func createHook(containerName string,
	finishedWaitGroup *sync.WaitGroup,
	startedWaitGroup *sync.WaitGroup) []testcontainers.ContainerLifecycleHooks {
	return []testcontainers.ContainerLifecycleHooks{
		{
			PostStarts: []testcontainers.ContainerHook{
				func(ctx context.Context, container testcontainers.Container) error {
					startedWaitGroup.Done()
					return nil
				},
			},
			PostTerminates: []testcontainers.ContainerHook{
				func(ctx context.Context, container testcontainers.Container) error {
					finishedWaitGroup.Done()
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

	debugLogger := debugLogging{}
	var finishedWaitGroup sync.WaitGroup
	var startedWaitGroup sync.WaitGroup

	containers := []testcontainers.Container{}
	if depsConfig.Postgres != nil {
		// wait strategy copied from testcontainers docs
		postgresWaitStrategy := wait.ForLog("database system is ready to accept connections").
			WithOccurrence(numPostgresCheckReadyAttempts).
			WithPollInterval(pollInterval)

		startedWaitGroup.Add(1)
		postgresReq := testcontainers.ContainerRequest{
			Image: depsConfig.Postgres.DockerImage,
			ExposedPorts: []string{strings.Join([]string{
				depsConfig.Postgres.Port, ":5432/tcp"}, "")},
			WaitingFor: postgresWaitStrategy,
			Name:       "rollups-node-dep-postgres",
			Env: map[string]string{
				"POSTGRES_PASSWORD": depsConfig.Postgres.Password,
			},
			LifecycleHooks: createHook("rollups-node-dep-postgres",
				&finishedWaitGroup, &startedWaitGroup),
		}
		postgres, err := testcontainers.GenericContainer(
			ctx,
			testcontainers.GenericContainerRequest{
				ContainerRequest: postgresReq,
				Started:          true,
				Logger:           debugLogger,
			},
		)
		if err != nil {
			return nil, err
		}
		finishedWaitGroup.Add(1)
		containers = append(containers, postgres)
	}

	if depsConfig.Devnet != nil {
		startedWaitGroup.Add(1)
		devNetReq := testcontainers.ContainerRequest{
			Image:        depsConfig.Devnet.DockerImage,
			ExposedPorts: []string{strings.Join([]string{depsConfig.Devnet.Port, ":8545/tcp"}, "")},
			WaitingFor:   wait.ForLog("Block Number: 21"),
			Name:         "rollups-node-dep-devnet",
			Cmd: []string{"anvil", "--block-time",
				depsConfig.Devnet.BlockTime, "--load-state", "/usr/share/devnet/anvil_state.json"},
			LifecycleHooks: createHook("rollups-node-dep-devnet",
				&finishedWaitGroup, &startedWaitGroup),
		}
		devnet, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: devNetReq,
			Started:          true,
			Logger:           debugLogger,
		})
		if err != nil {
			return nil, err
		}
		finishedWaitGroup.Add(1)
		containers = append(containers, devnet)
	}

	if len(containers) < 1 {
		return nil, fmt.Errorf("configuration is empty")
	}
	startedWaitGroup.Wait()

	return &DepsContainers{containers, &finishedWaitGroup}, nil
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

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package deps provides mechanisms to run Node dependencies using docker
package deps

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	DefaultPostgresDatabase              = "postgres"
	DefaultPostgresDockerImage           = "postgres:16-alpine"
	DefaultPostgresPort                  = "5432"
	DefaultPostgresUser                  = "postgres"
	DefaultPostgresPassword              = "password"
	DefaultDevnetDockerImage             = "cartesi/rollups-node-devnet:devel"
	DefaultDevnetPort                    = "8545"
	DefaultDevnetBlockTime               = "1"
	DefaultDevnetBlockToWaitForOnStartup = "21"
	DefaultDevnetNoMining                = false

	numPostgresCheckReadyAttempts = 2
	pollInterval                  = 5 * time.Second
)

const (
	postgresKey = iota
	devnetKey
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
	DockerImage             string
	Port                    string
	BlockTime               string
	BlockToWaitForOnStartup string
	NoMining                bool
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
			DefaultDevnetBlockTime,
			DefaultDevnetBlockToWaitForOnStartup,
			DefaultDevnetNoMining,
		},
	}
}

// Struct to represent the Node dependencies containers
type DepsContainers struct {
	containers map[int]testcontainers.Container
	//Literal copies lock value from waitGroup as sync.WaitGroup contains sync.noCopy
	waitGroup *sync.WaitGroup
}

func (depContainers *DepsContainers) DevnetLogs(ctx context.Context) (io.ReadCloser, error) {
	container, ok := depContainers.containers[devnetKey]
	if !ok {
		return nil, fmt.Errorf("Container Devnet is not present")
	}
	reader, err := container.Logs(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving logs from Devnet Container : %w", err)
	}
	return reader, nil
}

func (depContainers *DepsContainers) PostgresLogs(ctx context.Context) (io.ReadCloser, error) {
	container, ok := depContainers.containers[postgresKey]
	if !ok {
		return nil, fmt.Errorf("Container Postgres is not present")
	}
	reader, err := container.Logs(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving logs from Postgres Container : %w", err)
	}
	return reader, nil
}

func (depContainers *DepsContainers) DevnetEndpoint(
	ctx context.Context,
	protocol string,
) (string, error) {
	container, ok := depContainers.containers[devnetKey]
	if !ok {
		return "", fmt.Errorf("Container Devnet is not present")
	}
	endpoint, err := container.Endpoint(ctx, protocol)
	if err != nil {
		return "", fmt.Errorf("Error retrieving endpoint from Devnet Container : %w", err)
	}
	return endpoint, nil
}

func (depContainers *DepsContainers) PostgresEndpoint(
	ctx context.Context,
	protocol string,
) (string, error) {

	container, ok := depContainers.containers[postgresKey]
	if !ok {
		return "", fmt.Errorf("Container Postgres is not present")
	}
	endpoint, err := container.Endpoint(ctx, protocol)
	if err != nil {
		return "", fmt.Errorf("Error retrieving endpoint from Postgres Container : %w", err)
	}
	return endpoint, nil
}

// debugLogging implements the testcontainers.Logging interface by printing the log to slog.Debug.
type debugLogging struct{}

func (d debugLogging) Printf(format string, v ...interface{}) {
	slog.Debug(fmt.Sprintf(format, v...))
}

func createHook(finishedWaitGroup *sync.WaitGroup) []testcontainers.ContainerLifecycleHooks {
	return []testcontainers.ContainerLifecycleHooks{
		{

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
	containers := make(map[int]testcontainers.Container)
	if depsConfig.Postgres != nil {
		// wait strategy copied from testcontainers docs
		postgresWaitStrategy := wait.ForLog("database system is ready to accept connections").
			WithOccurrence(numPostgresCheckReadyAttempts).
			WithPollInterval(pollInterval)

		postgresExposedPorts := "5432/tcp"
		if depsConfig.Postgres.Port != "" {
			postgresExposedPorts = strings.Join([]string{
				depsConfig.Postgres.Port, ":", postgresExposedPorts}, "")
		}
		postgresReq := testcontainers.ContainerRequest{
			Image:        depsConfig.Postgres.DockerImage,
			ExposedPorts: []string{postgresExposedPorts},
			WaitingFor:   postgresWaitStrategy,
			Env: map[string]string{
				"POSTGRES_PASSWORD": depsConfig.Postgres.Password,
			},
			LifecycleHooks: createHook(&finishedWaitGroup),
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
		containers[postgresKey] = postgres
	}

	if depsConfig.Devnet != nil {

		devnetExposedPort := "8545/tcp"
		if depsConfig.Devnet.Port != "" {
			devnetExposedPort = strings.Join([]string{
				depsConfig.Devnet.Port, ":", devnetExposedPort}, "")
		}
		cmd := []string{
			"anvil",
			"--load-state",
			"/usr/share/devnet/anvil_state.json",
		}
		var waitStrategy *wait.LogStrategy
		if depsConfig.Devnet.NoMining {
			cmd = append(cmd, "--no-mining")
			waitStrategy = wait.ForLog("net_listening")
		} else {
			cmd = append(cmd, "--block-time",
				depsConfig.Devnet.BlockTime)
			waitStrategy = wait.ForLog("Block Number: " + depsConfig.Devnet.BlockToWaitForOnStartup)
		}
		devNetReq := testcontainers.ContainerRequest{
			Image:          depsConfig.Devnet.DockerImage,
			ExposedPorts:   []string{devnetExposedPort},
			WaitingFor:     waitStrategy,
			Cmd:            cmd,
			LifecycleHooks: createHook(&finishedWaitGroup),
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
		containers[devnetKey] = devnet
	}

	if len(containers) < 1 {
		return nil, fmt.Errorf("configuration is empty")
	}

	return &DepsContainers{containers: containers,
		waitGroup: &finishedWaitGroup,
	}, nil
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
	return nil
}

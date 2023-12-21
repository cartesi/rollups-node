// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package machine provides mechanisms to handle Cartesi Machine Snapshots
// to run Node locally for development and tests
package deps

import (
	"context"
	"strings"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	defaultPostgresDockerImage = "postgres:13-alpine"
	defaultPostgresPort        = "5432"
	defaultPostgresPassword    = "password"
	defaultDevnetDockerImage   = "sunodo/devnet:1.1.1"
	defaultDevnetPort          = "8545"

	numPostgresCheckReadyAttempts = 2
	fiveSeconds                   = 5 * time.Second
)

type DepsConfig struct {
	postgresDockerImage string
	postgresPort        string
	postgresPassword    string
	devnetDockerImage   string
	devnetPort          string
}

func NewDefaultDepsConfig() *DepsConfig {
	return &DepsConfig{
		defaultPostgresDockerImage,
		defaultPostgresPort,
		defaultPostgresPassword,
		defaultDevnetDockerImage,
		defaultDevnetPort,
	}
}

func NewDepsConfig() *DepsConfig {
	return &DepsConfig{}
}

func (config *DepsConfig) WithPostgresDockerImage(postgresDockerImage string) *DepsConfig {
	if postgresDockerImage != "" {
		config.postgresDockerImage = postgresDockerImage
	} else {
		config.postgresDockerImage = defaultPostgresDockerImage
	}
	return config
}

func (config *DepsConfig) WithPostgresPort(postgresPort string) *DepsConfig {
	if postgresPort != "" {
		config.postgresPort = postgresPort
	} else {
		config.postgresPort = defaultPostgresPort
	}

	return config
}

func (config *DepsConfig) WithPostgresPassword(postgresPassword string) *DepsConfig {
	if postgresPassword != "" {
		config.postgresPassword = postgresPassword
	} else {
		config.postgresPassword = defaultPostgresPassword
	}
	return config
}

func (config *DepsConfig) WithDevenetDockerImage(devnetDockerImage string) *DepsConfig {
	if devnetDockerImage != "" {
		config.devnetDockerImage = devnetDockerImage
	} else {
		config.devnetDockerImage = defaultDevnetDockerImage
	}

	return config
}

func (config *DepsConfig) WithDevenetPort(devnetPort string) *DepsConfig {
	if devnetPort != "" {
		config.devnetPort = devnetPort
	} else {
		config.devnetPort = defaultDevnetPort
	}

	return config
}

type DepsContainers struct {
	postgres testcontainers.Container
	devnet   testcontainers.Container
}

func (depsContainers *DepsContainers) toContainerArray() []testcontainers.Container {
	return []testcontainers.Container{depsContainers.postgres, depsContainers.devnet}
}

func Run(ctx context.Context, depsConfig DepsConfig) (*DepsContainers, error) {

	// wait strategy copied from testcontainers docs
	postgresWaitStrategy := wait.ForLog("database system is ready to accept connections").
		WithOccurrence(numPostgresCheckReadyAttempts).
		WithPollInterval(fiveSeconds)

	postgresReq := testcontainers.ContainerRequest{
		Image: depsConfig.postgresDockerImage,
		ExposedPorts: []string{strings.Join([]string{
			depsConfig.postgresPort, ":5432/tcp"}, "")},
		WaitingFor: postgresWaitStrategy,
		Name:       "rollups-node-dep-postgres",
		Env: map[string]string{
			"POSTGRES_PASSWORD": depsConfig.postgresPassword,
		},
	}

	postgres, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	devNetReq := testcontainers.ContainerRequest{
		Image:        depsConfig.devnetDockerImage,
		ExposedPorts: []string{strings.Join([]string{depsConfig.devnetPort, ":8545/tcp"}, "")},
		WaitingFor:   wait.ForExec([]string{"eth_isready"}),
		Name:         "rollups-node-dep-devnet",
		Env: map[string]string{
			"ANVIL_IP_ADDR": "0.0.0.0",
		},
	}

	devnet, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: devNetReq,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	return &DepsContainers{postgres, devnet}, nil
}

func Terminate(ctx context.Context, depsContainers *DepsContainers) []error {
	errors := []error{}
	for _, container := range depsContainers.toContainerArray() {
		err := container.Terminate(ctx)
		if err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return errors
	}
	return nil
}

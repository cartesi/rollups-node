// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package node defines the individual services internally used to implement
// Rollups Node's features
package node

import (
	"fmt"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/internal/services"
)

const (
	serverManagerPort    = "5001"
	serverManagerAddress = "0.0.0.0:" + serverManagerPort
)

var ValidatorServices = []services.Service{
	StateServer, // must start before Dispatcher
	AdvanceRunner,
	AuthorityClaimer,
	Dispatcher,
	GraphQLServer,
	Indexer,
	InspectServer,
	ServerManager,
}

var (
	AdvanceRunner = services.NewService(
		"advance-runner",
		healthcheckPort("advance-runner"),
		"cartesi-rollups-advance-runner",
	)
	AuthorityClaimer = services.NewService(
		"authority-claimer",
		healthcheckPort("authority-claimer"),
		"cartesi-rollups-authority-claimer",
	)
	Dispatcher = services.NewService(
		"dispatcher",
		healthcheckPort("dispatcher"),
		"cartesi-rollups-dispatcher",
	)
	GraphQLServer = services.NewService(
		"graphql-server",
		healthcheckPort("graphql"),
		"cartesi-rollups-graphql-server",
	)
	Indexer = services.NewService(
		"indexer",
		healthcheckPort("indexer"),
		"cartesi-rollups-indexer",
	)
	InspectServer = services.NewService(
		"inspect-server",
		healthcheckPort("inspect-server"),
		"cartesi-rollups-inspect-server",
	)
	StateServer = services.NewService(
		"state-server",
		stateServerHealthcheckPort(),
		"cartesi-rollups-state-server",
	)
	ServerManager = services.NewService(
		"server-manager",
		serverManagerPort,
		"server-manager",
		"--manager-address="+serverManagerAddress,
	)
)

func healthcheckPort(serviceName string) string {
	env := healthcheckEnv(serviceName)
	if port, ok := os.LookupEnv(env); ok {
		return port
	}
	panic(fmt.Sprintf("environment variable %s is empty", env))
}

func healthcheckEnv(serviceName string) string {
	suffix := "_HEALTHCHECK_PORT"
	if serviceName == "dispatcher" || serviceName == "authority-claimer" {
		suffix = "_HTTP_SERVER_PORT"
	}
	normalizedName := strings.Replace(serviceName, "-", "_", -1)
	return fmt.Sprintf("%s%s", strings.ToUpper(normalizedName), suffix)
}

func stateServerHealthcheckPort() string {
	env := "SS_SERVER_ADDRESS"
	if address, ok := os.LookupEnv(env); ok {
		split := strings.Split(address, ":")
		if len(split) > 1 {
			return split[1]
		}
	}
	panic(fmt.Sprintf("environment variable %s is empty", env))
}

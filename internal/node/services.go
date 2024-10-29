// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"fmt"
	"log/slog"
	"net/http"

	advancerservice "github.com/cartesi/rollups-node/internal/advancer/service"
	claimerservice "github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/config"
	espressoreaderservice "github.com/cartesi/rollups-node/internal/espressoreader/service"
	evmreaderservice "github.com/cartesi/rollups-node/internal/evmreader/service"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/services"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/cartesi/rollups-node/pkg/service"
)

// We use an enum to define the ports of each service and avoid conflicts.
type portOffset = int

const (
	portOffsetProxy = iota
)

// Get the port of the given service.
func getPort(c config.NodeConfig, offset portOffset) int {
	return c.HttpPort + int(offset)
}

func newSupervisorService(
	c config.NodeConfig,
	workDir string,
	database *repository.Database,
) *services.SupervisorService {
	var s []services.Service

	if c.FeatureClaimerEnabled {
		s = append(s, newClaimerService(c, database))
	}

	serveMux := http.NewServeMux()
	serveMux.Handle("/healthz", http.HandlerFunc(healthcheckHandler))

	if c.MainSequencer == "ethereum" {
		s = append(s, newEvmReaderService(c, database))
	} else {
		s = append(s, NewEspressoReaderService(c, database))
	}
	s = append(s, newAdvancerService(c, database, serveMux))
	s = append(s, newValidatorService(c, database))
	s = append(s, newHttpService(c, serveMux))

	supervisor := services.SupervisorService{
		Name:     "rollups-node",
		Services: s,
	}
	return &supervisor
}

func newHttpService(c config.NodeConfig, serveMux *http.ServeMux) services.Service {
	addr := fmt.Sprintf("%v:%v", c.HttpAddress, getPort(c, portOffsetProxy))
	return &services.HttpService{
		Name:    "http",
		Address: addr,
		Handler: serveMux,
	}
}

func newEvmReaderService(c config.NodeConfig, database *repository.Database) services.Service {
	return evmreaderservice.NewEvmReaderService(
		c.BlockchainHttpEndpoint.Value,
		c.BlockchainWsEndpoint.Value,
		database,
		c.EvmReaderRetryPolicyMaxRetries,
		c.EvmReaderRetryPolicyMaxDelay,
	)
}

func NewEspressoReaderService(
	c config.NodeConfig,
	database *repository.Database,
) services.Service {
	return espressoreaderservice.NewEspressoReaderService(
		c.BlockchainHttpEndpoint.Value,
		c.BlockchainHttpEndpoint.Value,
		database,
		c.EspressoBaseUrl,
		c.EspressoStartingBlock,
		c.EspressoNamespace,
		c.EvmReaderRetryPolicyMaxRetries,
		c.EvmReaderRetryPolicyMaxDelay,
	)
}

func newAdvancerService(c config.NodeConfig, database *repository.Database, serveMux *http.ServeMux) services.Service {
	return advancerservice.NewAdvancerService(
		database,
		serveMux,
		c.AdvancerPollingInterval,
		c.MachineServerVerbosity,
	)
}

func newValidatorService(c config.NodeConfig, database *repository.Database) services.Service {
	return validator.NewValidatorService(
		database,
		uint64(c.ContractsInputBoxDeploymentBlockNumber),
		c.ValidatorPollingInterval,
	)
}

func newClaimerService(c config.NodeConfig, database *repository.Database) services.Service {
	claimerService := claimerservice.Service{}
	createInfo := claimerservice.CreateInfo{
		Auth: c.Auth,
		DBConn: database,
		PostgresEndpoint: c.PostgresEndpoint,
		BlockchainHttpEndpoint: c.BlockchainHttpEndpoint,
		CreateInfo: service.CreateInfo{
			Name:                 "claimer",
			PollInterval:         c.ClaimerPollingInterval,
			Impl:                 &claimerService,
			ProcOwner: true, // TODO: Remove this after updating supervisor
			LogLevel: map[slog.Level]string{ // reverse it to string
				slog.LevelDebug: "debug",
				slog.LevelInfo: "info",
				slog.LevelWarn: "warn",
				slog.LevelError: "error",
			}[c.LogLevel],
		},
	}

	err := claimerservice.Create(createInfo, &claimerService)
	if err != nil {
		claimerService.Logger.Error("Fatal",
			"service", claimerService.Name,
			"error", err)
	}
	return &claimerService
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	advancerservice "github.com/cartesi/rollups-node/internal/advancer/service"
	"github.com/cartesi/rollups-node/internal/config"
	evmreaderservice "github.com/cartesi/rollups-node/internal/evmreader/service"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/services"
	"github.com/cartesi/rollups-node/internal/validator"
)

// We use an enum to define the ports of each service and avoid conflicts.
type portOffset = int

const (
	portOffsetProxy = iota
	portOffsetAuthorityClaimer
)

// Get the port of the given service.
func getPort(c config.NodeConfig, offset portOffset) int {
	return c.HttpPort + int(offset)
}

// Create the RUST_LOG variable using the config log level.
// If the log level is set to debug, set tracing log for the given rust module.
func getRustLog(c config.NodeConfig, rustModule string) string {
	switch c.LogLevel {
	case slog.LevelDebug:
		return fmt.Sprintf("RUST_LOG=info,%v=trace", rustModule)
	case slog.LevelInfo:
		return "RUST_LOG=info"
	case slog.LevelWarn:
		return "RUST_LOG=warn"
	case slog.LevelError:
		return "RUST_LOG=error"
	default:
		panic("impossible")
	}
}

func newAuthorityClaimer(c config.NodeConfig, workDir string) services.Service {
	var s services.CommandService
	s.Name = "authority-claimer"
	s.HealthcheckPort = getPort(c, portOffsetAuthorityClaimer)
	s.Path = "cartesi-rollups-authority-claimer"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(c, "authority_claimer"))
	s.Env = append(s.Env, fmt.Sprintf("TX_PROVIDER_HTTP_ENDPOINT=%v",
		c.BlockchainHttpEndpoint.Value))
	s.Env = append(s.Env, fmt.Sprintf("TX_CHAIN_ID=%v", c.BlockchainID))
	s.Env = append(s.Env, fmt.Sprintf("TX_CHAIN_IS_LEGACY=%v", c.LegacyBlockchainEnabled))
	s.Env = append(s.Env, fmt.Sprintf("TX_DEFAULT_CONFIRMATIONS=%v",
		c.BlockchainFinalityOffset))
	s.Env = append(s.Env, fmt.Sprintf("POSTGRES_ENDPOINT=%v",
		fmt.Sprintf("%v", c.PostgresEndpoint.Value)))
	s.Env = append(s.Env, fmt.Sprintf("INPUT_BOX_ADDRESS=%v", c.ContractsInputBoxAddress))
	s.Env = append(s.Env, fmt.Sprintf("GENESIS_BLOCK=%v",
		c.ContractsInputBoxDeploymentBlockNumber))
	s.Env = append(s.Env, fmt.Sprintf("HTTP_SERVER_PORT=%v",
		getPort(c, portOffsetAuthorityClaimer)))
	switch auth := c.Auth.(type) {
	case config.AuthPrivateKey:
		s.Env = append(s.Env, fmt.Sprintf("TX_SIGNING_PRIVATE_KEY=%v",
			auth.PrivateKey.Value))
	case config.AuthMnemonic:
		s.Env = append(s.Env, fmt.Sprintf("TX_SIGNING_MNEMONIC=%v", auth.Mnemonic.Value))
		s.Env = append(s.Env, fmt.Sprintf("TX_SIGNING_MNEMONIC_ACCOUNT_INDEX=%v",
			auth.AccountIndex.Value))
	case config.AuthAWS:
		s.Env = append(s.Env, fmt.Sprintf("TX_SIGNING_AWS_KMS_KEY_ID=%v", auth.KeyID.Value))
		s.Env = append(s.Env, fmt.Sprintf("TX_SIGNING_AWS_KMS_REGION=%v",
			auth.Region.Value))
	default:
		panic("invalid auth config")
	}
	s.Env = append(s.Env, os.Environ()...)
	s.WorkDir = workDir
	return &s
}

func newSupervisorService(
	c config.NodeConfig,
	workDir string,
	database *repository.Database,
) *services.SupervisorService {
	var s []services.Service

	if c.FeatureClaimerEnabled {
		s = append(s, newAuthorityClaimer(c, workDir))
	}

	serveMux := http.NewServeMux()
	serveMux.Handle("/healthz", http.HandlerFunc(healthcheckHandler))

	s = append(s, newEvmReaderService(c, database))
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

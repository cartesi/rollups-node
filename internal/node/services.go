// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/internal/services"
)

// We use an enum to define the ports of each service and avoid conflicts.
type portOffset = int

const (
	portOffsetProxy = iota
	portOffsetAuthorityClaimer
	portOffsetRedis
)

const localhost = "127.0.0.1"

// Get the port of the given service.
func getPort(c config.NodeConfig, offset portOffset) int {
	return c.HttpPort + int(offset)
}

// Get the redis endpoint based on whether the experimental sunodo validator mode is enabled.
func getRedisEndpoint(c config.NodeConfig) string {
	if c.ExperimentalSunodoValidatorEnabled {
		return c.ExperimentalSunodoValidatorRedisEndpoint
	} else {
		return fmt.Sprintf("redis://%v:%v", localhost, getPort(c, portOffsetRedis))
	}
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

func newAuthorityClaimer(c config.NodeConfig, workDir string) services.CommandService {
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
	s.Env = append(s.Env, fmt.Sprintf("TX_CHAIN_IS_LEGACY=%v", c.BlockchainIsLegacy))
	s.Env = append(s.Env, fmt.Sprintf("TX_DEFAULT_CONFIRMATIONS=%v",
		c.BlockchainFinalityOffset))
	s.Env = append(s.Env, fmt.Sprintf("REDIS_ENDPOINT=%v", getRedisEndpoint(c)))
	s.Env = append(s.Env, fmt.Sprintf("ICONSENSUS_ADDRESS=%v", c.ContractsIConsensusAddress))
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
	return s
}

func newRedis(c config.NodeConfig, workDir string) services.CommandService {
	var s services.CommandService
	s.Name = "redis"
	s.HealthcheckPort = getPort(c, portOffsetRedis)
	s.Path = "redis-server"
	s.Args = append(s.Args, "--port", fmt.Sprint(getPort(c, portOffsetRedis)))
	// Disable persistence with --save and --appendonly config
	s.Args = append(s.Args, "--save", "")
	s.Args = append(s.Args, "--appendonly", "no")
	s.Env = append(s.Env, os.Environ()...)
	s.WorkDir = workDir
	return s
}

func newSupervisorService(c config.NodeConfig, workDir string) services.SupervisorService {
	var s []services.Service

	if !c.ExperimentalSunodoValidatorEnabled {
		// add Redis first
		s = append(s, newRedis(c, workDir))
	}

	// enable claimer if reader mode and sunodo validator mode are disabled
	if !c.FeatureDisableClaimer && !c.ExperimentalSunodoValidatorEnabled {
		s = append(s, newAuthorityClaimer(c, workDir))
	}

	s = append(s, newHttpService(c))

	supervisor := services.SupervisorService{
		Name:     "rollups-node",
		Services: s,
	}
	return supervisor
}

func newHttpService(c config.NodeConfig) services.HttpService {
	addr := fmt.Sprintf("%v:%v", c.HttpAddress, getPort(c, portOffsetProxy))
	handler := newHttpServiceHandler(c)
	return services.HttpService{
		Name:    "http",
		Address: addr,
		Handler: handler,
	}
}

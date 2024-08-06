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
	portOffsetAdvanceRunner
	portOffsetAuthorityClaimer
	portOffsetDispatcher
	portOffsetGraphQLServer
	portOffsetGraphQLHealthcheck
	portOffsetHostRunnerHealthcheck
	portOffsetHostRunnerRollups
	portOffsetIndexer
	portOffsetInspectServer
	portOffsetInspectHealthcheck
	portOffsetRedis
	portOffsetServerManager
	portOffsetStateServer
)

const (
	localhost              = "127.0.0.1"
	serverManagerSessionId = "default_session_id"
)

// Get the port of the given service.
func getPort(c config.NodeConfig, offset portOffset) int {
	return c.HttpPort + int(offset)
}

// Get the redis endpoint based on whether the experimental sunodo validator mode is enabled.
func getRedisEndpoint(c config.NodeConfig) string {
	if c.ExperimentalSunodoValidatorEnabled {
		return c.ExperimentalSunodoValidatorRedisEndpoint.Value
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

func newAdvanceRunner(c config.NodeConfig, workDir string) services.CommandService {
	var s services.CommandService
	s.Name = "advance-runner"
	s.HealthcheckPort = getPort(c, portOffsetAdvanceRunner)
	s.Path = "cartesi-rollups-advance-runner"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(c, "advance_runner"))
	s.Env = append(s.Env, fmt.Sprintf("SERVER_MANAGER_ENDPOINT=http://%v:%v",
		localhost, getPort(c, portOffsetServerManager)))
	s.Env = append(s.Env, fmt.Sprintf("SESSION_ID=%v", serverManagerSessionId))
	s.Env = append(s.Env, fmt.Sprintf("REDIS_ENDPOINT=%v", getRedisEndpoint(c)))
	s.Env = append(s.Env, fmt.Sprintf("CHAIN_ID=%v", c.BlockchainID))
	s.Env = append(s.Env, fmt.Sprintf("DAPP_CONTRACT_ADDRESS=%v",
		c.ContractsApplicationAddress))
	s.Env = append(s.Env, fmt.Sprintf("PROVIDER_HTTP_ENDPOINT=%v",
		c.BlockchainHttpEndpoint.Value))
	s.Env = append(s.Env, fmt.Sprintf("ADVANCE_RUNNER_HEALTHCHECK_PORT=%v",
		getPort(c, portOffsetAdvanceRunner)))
	s.Env = append(s.Env, fmt.Sprintf("READER_MODE=%v", c.FeatureDisableClaimer))
	if c.FeatureHostMode || c.FeatureDisableMachineHashCheck {
		s.Env = append(s.Env, "SNAPSHOT_VALIDATION_ENABLED=false")
	}
	if !c.FeatureHostMode {
		s.Env = append(s.Env, fmt.Sprintf("MACHINE_SNAPSHOT_PATH=%v", c.SnapshotDir))
	}
	s.Env = append(s.Env, os.Environ()...)
	s.WorkDir = workDir
	return s
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
	s.Env = append(s.Env, fmt.Sprintf("HISTORY_ADDRESS=%v", c.ContractsHistoryAddress))
	s.Env = append(s.Env, fmt.Sprintf("AUTHORITY_ADDRESS=%v", c.ContractsAuthorityAddress))
	s.Env = append(s.Env, fmt.Sprintf("INPUT_BOX_ADDRESS=%v", c.ContractsInputBoxAddress))
	s.Env = append(s.Env, fmt.Sprintf("GENESIS_BLOCK=%v",
		c.ContractsInputBoxDeploymentBlockNumber))
	s.Env = append(s.Env, fmt.Sprintf("AUTHORITY_CLAIMER_HTTP_SERVER_PORT=%v",
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

func newDispatcher(c config.NodeConfig, workDir string) services.CommandService {
	var s services.CommandService
	s.Name = "dispatcher"
	s.HealthcheckPort = getPort(c, portOffsetDispatcher)
	s.Path = "cartesi-rollups-dispatcher"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(c, "dispatcher"))
	s.Env = append(s.Env, fmt.Sprintf("SC_GRPC_ENDPOINT=http://%v:%v", localhost,
		getPort(c, portOffsetStateServer)))
	s.Env = append(s.Env, fmt.Sprintf("SC_DEFAULT_CONFIRMATIONS=%v",
		c.BlockchainFinalityOffset))
	s.Env = append(s.Env, fmt.Sprintf("REDIS_ENDPOINT=%v", getRedisEndpoint(c)))
	s.Env = append(s.Env, fmt.Sprintf("DAPP_ADDRESS=%v", c.ContractsApplicationAddress))
	s.Env = append(s.Env, fmt.Sprintf("INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER=%v",
		c.ContractsInputBoxDeploymentBlockNumber))
	s.Env = append(s.Env, fmt.Sprintf("HISTORY_ADDRESS=%v", c.ContractsHistoryAddress))
	s.Env = append(s.Env, fmt.Sprintf("AUTHORITY_ADDRESS=%v", c.ContractsAuthorityAddress))
	s.Env = append(s.Env, fmt.Sprintf("INPUT_BOX_ADDRESS=%v", c.ContractsInputBoxAddress))
	s.Env = append(s.Env, fmt.Sprintf("RD_EPOCH_LENGTH=%v", c.RollupsEpochLength))
	s.Env = append(s.Env, fmt.Sprintf("CHAIN_ID=%v", c.BlockchainID))
	s.Env = append(s.Env, fmt.Sprintf("DISPATCHER_HTTP_SERVER_PORT=%v",
		getPort(c, portOffsetDispatcher)))
	s.Env = append(s.Env, os.Environ()...)
	s.WorkDir = workDir
	return s
}

func newGraphQLServer(c config.NodeConfig, workDir string) services.CommandService {
	var s services.CommandService
	s.Name = "graphql-server"
	s.HealthcheckPort = getPort(c, portOffsetGraphQLHealthcheck)
	s.Path = "cartesi-rollups-graphql-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(c, "graphql_server"))
	s.Env = append(s.Env, fmt.Sprintf("POSTGRES_ENDPOINT=%v", c.PostgresEndpoint.Value))
	s.Env = append(s.Env, fmt.Sprintf("GRAPHQL_HOST=%v", localhost))
	s.Env = append(s.Env, fmt.Sprintf("GRAPHQL_PORT=%v", getPort(c, portOffsetGraphQLServer)))
	s.Env = append(s.Env, fmt.Sprintf("GRAPHQL_HEALTHCHECK_PORT=%v",
		getPort(c, portOffsetGraphQLHealthcheck)))
	s.Env = append(s.Env, os.Environ()...)
	s.WorkDir = workDir
	return s
}

func newHostRunner(c config.NodeConfig, workDir string) services.CommandService {
	var s services.CommandService
	s.Name = "host-runner"
	s.HealthcheckPort = getPort(c, portOffsetHostRunnerHealthcheck)
	s.Path = "cartesi-rollups-host-runner"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(c, "host_runner"))
	s.Env = append(s.Env, fmt.Sprintf("GRPC_SERVER_MANAGER_ADDRESS=%v", localhost))
	s.Env = append(s.Env, fmt.Sprintf("GRPC_SERVER_MANAGER_PORT=%v",
		getPort(c, portOffsetServerManager)))
	s.Env = append(s.Env, fmt.Sprintf("HTTP_ROLLUP_SERVER_ADDRESS=%v", localhost))
	s.Env = append(s.Env, fmt.Sprintf("HTTP_ROLLUP_SERVER_PORT=%v",
		getPort(c, portOffsetHostRunnerRollups)))
	s.Env = append(s.Env, fmt.Sprintf("HOST_RUNNER_HEALTHCHECK_PORT=%v",
		getPort(c, portOffsetHostRunnerHealthcheck)))
	s.Env = append(s.Env, os.Environ()...)
	s.WorkDir = workDir
	return s
}

func newIndexer(c config.NodeConfig, workdir string) services.CommandService {
	var s services.CommandService
	s.Name = "indexer"
	s.HealthcheckPort = getPort(c, portOffsetIndexer)
	s.Path = "cartesi-rollups-indexer"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(c, "indexer"))
	s.Env = append(s.Env, fmt.Sprintf("POSTGRES_ENDPOINT=%v", c.PostgresEndpoint.Value))
	s.Env = append(s.Env, fmt.Sprintf("CHAIN_ID=%v", c.BlockchainID))
	s.Env = append(s.Env, fmt.Sprintf("DAPP_CONTRACT_ADDRESS=%v",
		c.ContractsApplicationAddress))
	s.Env = append(s.Env, fmt.Sprintf("REDIS_ENDPOINT=%v", getRedisEndpoint(c)))
	s.Env = append(s.Env, fmt.Sprintf("INDEXER_HEALTHCHECK_PORT=%v",
		getPort(c, portOffsetIndexer)))
	s.Env = append(s.Env, os.Environ()...)
	s.WorkDir = workdir
	return s
}

func newInspectServer(c config.NodeConfig, workDir string) services.CommandService {
	var s services.CommandService
	s.Name = "inspect-server"
	s.HealthcheckPort = getPort(c, portOffsetInspectHealthcheck)
	s.Path = "cartesi-rollups-inspect-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(c, "inspect_server"))
	s.Env = append(s.Env, fmt.Sprintf("INSPECT_SERVER_ADDRESS=%v:%v", localhost,
		getPort(c, portOffsetInspectServer)))
	s.Env = append(s.Env, fmt.Sprintf("SERVER_MANAGER_ADDRESS=%v:%v", localhost,
		getPort(c, portOffsetServerManager)))
	s.Env = append(s.Env, fmt.Sprintf("SESSION_ID=%v", serverManagerSessionId))
	s.Env = append(s.Env, fmt.Sprintf("INSPECT_SERVER_HEALTHCHECK_PORT=%v",
		getPort(c, portOffsetInspectHealthcheck)))
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

func newServerManager(c config.NodeConfig, workDir string) services.ServerManager {
	var s services.ServerManager
	s.Name = "server-manager"
	s.HealthcheckPort = getPort(c, portOffsetServerManager)
	s.Path = "server-manager"
	s.Args = append(s.Args,
		fmt.Sprintf("--manager-address=%v:%v", localhost, getPort(c, portOffsetServerManager)))
	s.Env = append(s.Env, "REMOTE_CARTESI_MACHINE_LOG_LEVEL=info")
	if c.LogLevel == slog.LevelDebug {
		s.Env = append(s.Env, "SERVER_MANAGER_LOG_LEVEL=info")
	} else {
		s.Env = append(s.Env, "SERVER_MANAGER_LOG_LEVEL=warning")
	}
	s.Env = append(s.Env, os.Environ()...)
	s.BypassLog = c.ExperimentalServerManagerBypassLog
	s.WorkDir = workDir
	return s
}

func newStateServer(c config.NodeConfig, workDir string) services.CommandService {
	var s services.CommandService
	s.Name = "state-server"
	s.HealthcheckPort = getPort(c, portOffsetStateServer)
	s.Path = "cartesi-rollups-state-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(c, "state_server"))
	s.Env = append(s.Env, "SF_CONCURRENT_EVENTS_FETCH=1")
	s.Env = append(s.Env, fmt.Sprintf("SF_GENESIS_BLOCK=%v",
		c.ContractsInputBoxDeploymentBlockNumber))
	s.Env = append(s.Env, fmt.Sprintf("SF_SAFETY_MARGIN=%v", c.BlockchainFinalityOffset))
	s.Env = append(s.Env, fmt.Sprintf("BH_WS_ENDPOINT=%v", c.BlockchainWsEndpoint.Value))
	s.Env = append(s.Env, fmt.Sprintf("BH_HTTP_ENDPOINT=%v",
		c.BlockchainHttpEndpoint.Value))
	s.Env = append(s.Env, fmt.Sprintf("BLOCKCHAIN_BLOCK_TIMEOUT=%v", c.BlockchainBlockTimeout))
	s.Env = append(s.Env, fmt.Sprintf("SS_SERVER_ADDRESS=%v:%v", localhost,
		getPort(c, portOffsetStateServer)))
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

	// add services without dependencies
	s = append(s, newGraphQLServer(c, workDir))
	s = append(s, newIndexer(c, workDir))
	s = append(s, newStateServer(c, workDir))

	// start either the server manager or host runner
	if c.FeatureHostMode {
		s = append(s, newHostRunner(c, workDir))
	} else {
		s = append(s, newServerManager(c, workDir))
	}

	// enable claimer if reader mode and sunodo validator mode are disabled
	if !c.FeatureDisableClaimer && !c.ExperimentalSunodoValidatorEnabled {
		s = append(s, newAuthorityClaimer(c, workDir))
	}

	// add services with dependencies
	s = append(s, newAdvanceRunner(c, workDir)) // Depends on the server-manager/host-runner
	s = append(s, newDispatcher(c, workDir))    // Depends on the state server
	s = append(s, newInspectServer(c, workDir)) // Depends on the server-manager/host-runner

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

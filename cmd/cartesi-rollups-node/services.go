// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/config"
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
func getPort(nodeConfig config.NodeConfig, offset portOffset) int {
	return nodeConfig.CartesiHttpPort + int(offset)
}

// Get the redis endpoint based on whether the experimental sunodo validator mode is enabled.
func getRedisEndpoint(nodeConfig config.NodeConfig) string {
	if nodeConfig.CartesiExperimentalSunodoValidatorEnabled {
		return nodeConfig.CartesiExperimentalSunodoValidatorRedisEndpoint
	} else {
		return fmt.Sprintf("redis://%v:%v", localhost, getPort(nodeConfig, portOffsetRedis))
	}
}

// Create the RUST_LOG variable using the config log level.
// If the log level is set to debug, set tracing log for the given rust module.
func getRustLog(nodeConfig config.NodeConfig, rustModule string) string {
	switch nodeConfig.CartesiLogLevel {
	case config.LogLevelDebug:
		return fmt.Sprintf("RUST_LOG=info,%v=trace", rustModule)
	case config.LogLevelInfo:
		return "RUST_LOG=info"
	case config.LogLevelWarning:
		return "RUST_LOG=warn"
	case config.LogLevelError:
		return "RUST_LOG=error"
	default:
		panic("impossible")
	}
}

func newAdvanceRunner(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "advance-runner"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetAdvanceRunner)
	s.Path = "cartesi-rollups-advance-runner"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(nodeConfig, "advance_runner"))
	s.Env = append(s.Env,
		fmt.Sprintf("SERVER_MANAGER_ENDPOINT=http://%v:%v",
			localhost,
			getPort(nodeConfig, portOffsetServerManager)))
	s.Env = append(s.Env,
		fmt.Sprintf("SESSION_ID=%v", serverManagerSessionId))
	s.Env = append(s.Env,
		fmt.Sprintf("REDIS_ENDPOINT=%v", getRedisEndpoint(nodeConfig)))
	s.Env = append(s.Env,
		fmt.Sprintf("CHAIN_ID=%v", nodeConfig.CartesiBlockchainId))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_CONTRACT_ADDRESS=%v", nodeConfig.CartesiContractsApplicationAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("PROVIDER_HTTP_ENDPOINT=%v", nodeConfig.CartesiBlockchainHttpEndpoint))
	s.Env = append(s.Env,
		fmt.Sprintf("ADVANCE_RUNNER_HEALTHCHECK_PORT=%v",
			getPort(nodeConfig, portOffsetAdvanceRunner)))
	s.Env = append(s.Env,
		fmt.Sprintf("READER_MODE=%v", nodeConfig.CartesiFeatureDisableClaimer))
	if nodeConfig.CartesiFeatureHostMode || nodeConfig.CartesiFeatureDisableMachineHashCheck {
		s.Env = append(s.Env, "SNAPSHOT_VALIDATION_ENABLED=false")
	}
	if !nodeConfig.CartesiFeatureHostMode {
		s.Env = append(s.Env,
			fmt.Sprintf("MACHINE_SNAPSHOT_PATH=%v", nodeConfig.CartesiSnapshotDir))
	}
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newAuthorityClaimer(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "authority-claimer"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetAuthorityClaimer)
	s.Path = "cartesi-rollups-authority-claimer"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(nodeConfig, "authority_claimer"))
	s.Env = append(s.Env,
		fmt.Sprintf("TX_PROVIDER_HTTP_ENDPOINT=%v", nodeConfig.CartesiBlockchainHttpEndpoint))
	s.Env = append(s.Env,
		fmt.Sprintf("TX_CHAIN_ID=%v", nodeConfig.CartesiBlockchainId))
	s.Env = append(s.Env,
		fmt.Sprintf("TX_CHAIN_IS_LEGACY=%v", nodeConfig.CartesiBlockchainIsLegacy))
	s.Env = append(s.Env,
		fmt.Sprintf("TX_DEFAULT_CONFIRMATIONS=%v", nodeConfig.CartesiBlockchainFinalityOffset))
	s.Env = append(s.Env,
		fmt.Sprintf("REDIS_ENDPOINT=%v", getRedisEndpoint(nodeConfig)))
	s.Env = append(s.Env,
		fmt.Sprintf("HISTORY_ADDRESS=%v", nodeConfig.CartesiContractsHistoryAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("AUTHORITY_ADDRESS=%v", nodeConfig.CartesiContractsAuthorityAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("INPUT_BOX_ADDRESS=%v", nodeConfig.CartesiContractsInputBoxAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("GENESIS_BLOCK=%v", nodeConfig.CartesiContractsInputBoxDeploymentBlockNumber))
	s.Env = append(s.Env,
		fmt.Sprintf("AUTHORITY_CLAIMER_HTTP_SERVER_PORT=%v",
			getPort(nodeConfig, portOffsetAuthorityClaimer)))
	switch auth := config.GetAuth().(type) {
	case config.AuthMnemonic:
		s.Env = append(s.Env,
			fmt.Sprintf("TX_SIGNING_MNEMONIC=%v", auth.Mnemonic))
		s.Env = append(s.Env,
			fmt.Sprintf("TX_SIGNING_MNEMONIC_ACCOUNT_INDEX=%v", auth.AccountIndex))
	case config.AuthAWS:
		s.Env = append(s.Env,
			fmt.Sprintf("TX_SIGNING_AWS_KMS_KEY_ID=%v", auth.KeyID))
		s.Env = append(s.Env,
			fmt.Sprintf("TX_SIGNING_AWS_KMS_REGION=%v", auth.Region))
	default:
		panic("invalid auth config")
	}
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newDispatcher(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "dispatcher"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetDispatcher)
	s.Path = "cartesi-rollups-dispatcher"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(nodeConfig, "dispatcher"))
	s.Env = append(s.Env,
		fmt.Sprintf("SC_GRPC_ENDPOINT=http://%v:%v",
			localhost, getPort(nodeConfig, portOffsetStateServer)))
	s.Env = append(s.Env,
		fmt.Sprintf("SC_DEFAULT_CONFIRMATIONS=%v", nodeConfig.CartesiBlockchainFinalityOffset))
	s.Env = append(s.Env,
		fmt.Sprintf("REDIS_ENDPOINT=%v", getRedisEndpoint(nodeConfig)))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_ADDRESS=%v", nodeConfig.CartesiContractsApplicationAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_DEPLOYMENT_BLOCK_NUMBER=%v",
			nodeConfig.CartesiContractsApplicationDeploymentBlockNumber))
	s.Env = append(s.Env,
		fmt.Sprintf("HISTORY_ADDRESS=%v", nodeConfig.CartesiContractsHistoryAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("AUTHORITY_ADDRESS=%v", nodeConfig.CartesiContractsAuthorityAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("INPUT_BOX_ADDRESS=%v", nodeConfig.CartesiContractsInputBoxAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("RD_EPOCH_DURATION=%v", int(nodeConfig.CartesiEpochDuration.Seconds())))
	s.Env = append(s.Env,
		fmt.Sprintf("CHAIN_ID=%v", nodeConfig.CartesiBlockchainId))
	s.Env = append(s.Env,
		fmt.Sprintf("DISPATCHER_HTTP_SERVER_PORT=%v", getPort(nodeConfig, portOffsetDispatcher)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newGraphQLServer(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "graphql-server"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetGraphQLHealthcheck)
	s.Path = "cartesi-rollups-graphql-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(nodeConfig, "graphql_server"))
	s.Env = append(s.Env,
		fmt.Sprintf("POSTGRES_ENDPOINT=%v", nodeConfig.CartesiPostgresEndpoint))
	s.Env = append(s.Env, fmt.Sprintf("GRAPHQL_HOST=%v", localhost))
	s.Env = append(s.Env,
		fmt.Sprintf("GRAPHQL_PORT=%v", getPort(nodeConfig, portOffsetGraphQLServer)))
	s.Env = append(s.Env,
		fmt.Sprintf("GRAPHQL_HEALTHCHECK_PORT=%v",
			getPort(nodeConfig, portOffsetGraphQLHealthcheck)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newHostRunner(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "host-runner"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetHostRunnerHealthcheck)
	s.Path = "cartesi-rollups-host-runner"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(nodeConfig, "host_runner"))
	s.Env = append(s.Env, fmt.Sprintf("GRPC_SERVER_MANAGER_ADDRESS=%v", localhost))
	s.Env = append(s.Env,
		fmt.Sprintf("GRPC_SERVER_MANAGER_PORT=%v", getPort(nodeConfig, portOffsetServerManager)))
	s.Env = append(s.Env, fmt.Sprintf("HTTP_ROLLUP_SERVER_ADDRESS=%v", localhost))
	s.Env = append(s.Env,
		fmt.Sprintf("HTTP_ROLLUP_SERVER_PORT=%v", getPort(nodeConfig, portOffsetHostRunnerRollups)))
	s.Env = append(s.Env,
		fmt.Sprintf("HOST_RUNNER_HEALTHCHECK_PORT=%v",
			getPort(nodeConfig, portOffsetHostRunnerHealthcheck)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newIndexer(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "indexer"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetIndexer)
	s.Path = "cartesi-rollups-indexer"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(nodeConfig, "indexer"))
	s.Env = append(s.Env,
		fmt.Sprintf("POSTGRES_ENDPOINT=%v", nodeConfig.CartesiPostgresEndpoint))
	s.Env = append(s.Env,
		fmt.Sprintf("CHAIN_ID=%v", nodeConfig.CartesiBlockchainId))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_CONTRACT_ADDRESS=%v", nodeConfig.CartesiContractsApplicationAddress))
	s.Env = append(s.Env,
		fmt.Sprintf("REDIS_ENDPOINT=%v", getRedisEndpoint(nodeConfig)))
	s.Env = append(s.Env,
		fmt.Sprintf("INDEXER_HEALTHCHECK_PORT=%v", getPort(nodeConfig, portOffsetIndexer)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newInspectServer(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "inspect-server"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetInspectHealthcheck)
	s.Path = "cartesi-rollups-inspect-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(nodeConfig, "inspect_server"))
	s.Env = append(s.Env,
		fmt.Sprintf("INSPECT_SERVER_ADDRESS=%v:%v",
			localhost, getPort(nodeConfig, portOffsetInspectServer)))
	s.Env = append(s.Env,
		fmt.Sprintf("SERVER_MANAGER_ADDRESS=%v:%v",
			localhost, getPort(nodeConfig, portOffsetServerManager)))
	s.Env = append(s.Env,
		fmt.Sprintf("SESSION_ID=%v", serverManagerSessionId))
	s.Env = append(s.Env,
		fmt.Sprintf("INSPECT_SERVER_HEALTHCHECK_PORT=%v",
			getPort(nodeConfig, portOffsetInspectHealthcheck)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newRedis(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "redis"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetRedis)
	s.Path = "redis-server"
	s.Args = append(s.Args, "--port", fmt.Sprint(getPort(nodeConfig, portOffsetRedis)))
	// Disable persistence with --save and --appendonly config
	s.Args = append(s.Args, "--save", "")
	s.Args = append(s.Args, "--appendonly", "no")
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newServerManager(nodeConfig config.NodeConfig) services.ServerManager {
	var s services.ServerManager
	s.Name = "server-manager"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetServerManager)
	s.Path = "server-manager"
	s.Args = append(s.Args,
		fmt.Sprintf("--manager-address=%v:%v",
			localhost, getPort(nodeConfig, portOffsetServerManager)))
	s.Env = append(s.Env, "REMOTE_CARTESI_MACHINE_LOG_LEVEL=info")
	if nodeConfig.CartesiLogLevel == config.LogLevelDebug {
		s.Env = append(s.Env, "SERVER_MANAGER_LOG_LEVEL=info")
	} else {
		s.Env = append(s.Env, "SERVER_MANAGER_LOG_LEVEL=warning")
	}
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newStateServer(nodeConfig config.NodeConfig) services.CommandService {
	var s services.CommandService
	s.Name = "state-server"
	s.HealthcheckPort = getPort(nodeConfig, portOffsetStateServer)
	s.Path = "cartesi-rollups-state-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog(nodeConfig, "state_server"))
	s.Env = append(s.Env, "SF_CONCURRENT_EVENTS_FETCH=1")
	s.Env = append(s.Env,
		fmt.Sprintf("SF_GENESIS_BLOCK=%v",
			nodeConfig.CartesiContractsInputBoxDeploymentBlockNumber))
	s.Env = append(s.Env,
		fmt.Sprintf("SF_SAFETY_MARGIN=%v", nodeConfig.CartesiBlockchainFinalityOffset))
	s.Env = append(s.Env,
		fmt.Sprintf("BH_WS_ENDPOINT=%v", nodeConfig.CartesiBlockchainWsEndpoint))
	s.Env = append(s.Env,
		fmt.Sprintf("BH_HTTP_ENDPOINT=%v", nodeConfig.CartesiBlockchainHttpEndpoint))
	s.Env = append(s.Env,
		fmt.Sprintf("BLOCKCHAIN_BLOCK_TIMEOUT=%v", nodeConfig.CartesiBlockchainBlockTimeout))
	s.Env = append(s.Env,
		fmt.Sprintf("SS_SERVER_ADDRESS=%v:%v",
			localhost, getPort(nodeConfig, portOffsetStateServer)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newSupervisorService(s []services.Service) services.SupervisorService {
	return services.SupervisorService{
		Name:     "rollups-node",
		Services: s,
	}
}

func newHttpService(nodeConfig config.NodeConfig) services.HttpService {
	addr := fmt.Sprintf("%v:%v", nodeConfig.CartesiHttpAddress,
		getPort(nodeConfig, portOffsetProxy))
	handler := newHttpServiceHandler(nodeConfig)
	return services.HttpService{
		Name:    "http",
		Address: addr,
		Handler: handler,
	}
}

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

// Get the port of the given service.
func getPort(offset portOffset) int {
	return config.GetHttpPort() + int(offset)
}

const serverManagerSessionId = "default_session_id"

// Create the RUST_LOG variable using the config log level.
// If the log level is set to debug, set tracing log for the given rust module.
func getRustLog(rustModule string) string {
	switch config.GetLogLevel() {
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

func newAdvanceRunner() services.Service {
	var s services.Service
	s.Name = "advance-runner"
	s.HealthcheckPort = getPort(portOffsetAdvanceRunner)
	s.Path = "cartesi-rollups-advance-runner"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog("advance_runner"))
	s.Env = append(s.Env,
		fmt.Sprintf("SERVER_MANAGER_ENDPOINT=http://127.0.0.1:%v",
			getPort(portOffsetServerManager)))
	s.Env = append(s.Env,
		fmt.Sprintf("SESSION_ID=%v", serverManagerSessionId))
	s.Env = append(s.Env,
		fmt.Sprintf("REDIS_ENDPOINT=redis://127.0.0.1:%v", getPort(portOffsetRedis)))
	s.Env = append(s.Env,
		fmt.Sprintf("CHAIN_ID=%v", config.GetBlockchainId()))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_CONTRACT_ADDRESS=%v", config.GetContractsDappAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("PROVIDER_HTTP_ENDPOINT=%v", config.GetBlockchainHttpEndpoint()))
	s.Env = append(s.Env,
		fmt.Sprintf("ADVANCE_RUNNER_HEALTHCHECK_PORT=%v", getPort(portOffsetAdvanceRunner)))
	if config.GetFeatureHostMode() {
		s.Env = append(s.Env, "SNAPSHOT_ENABLED=false")
		s.Env = append(s.Env, "SNAPSHOT_VALIDATION_ENABLED=false")
	} else {
		s.Env = append(s.Env,
			fmt.Sprintf("SNAPSHOT_DIR=%v", config.GetSnapshotDir()))
		s.Env = append(s.Env,
			fmt.Sprintf("SNAPSHOT_LATEST=%v/latest", config.GetSnapshotDir()))
	}
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newAuthorityClaimer() services.Service {
	var s services.Service
	s.Name = "authority-claimer"
	s.HealthcheckPort = getPort(portOffsetAuthorityClaimer)
	s.Path = "cartesi-rollups-authority-claimer"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog("authority_claimer"))
	s.Env = append(s.Env,
		fmt.Sprintf("TX_PROVIDER_HTTP_ENDPOINT=%v", config.GetBlockchainHttpEndpoint()))
	s.Env = append(s.Env,
		fmt.Sprintf("TX_CHAIN_ID=%v", config.GetBlockchainId()))
	s.Env = append(s.Env,
		fmt.Sprintf("TX_CHAIN_IS_LEGACY=%v", config.GetBlockchainIsLegacy()))
	s.Env = append(s.Env,
		fmt.Sprintf("TX_DEFAULT_CONFIRMATIONS=%v", config.GetBlockchainReadDepth()))
	s.Env = append(s.Env,
		fmt.Sprintf("REDIS_ENDPOINT=redis://127.0.0.1:%v", getPort(portOffsetRedis)))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_ADDRESS=%v", config.GetContractsDappAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_DEPLOYMENT_BLOCK_NUMBER=%v",
			config.GetContractsDappDeploymentBlockNumber()))
	s.Env = append(s.Env,
		fmt.Sprintf("HISTORY_ADDRESS=%v", config.GetContractsHistoryAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("AUTHORITY_ADDRESS=%v", config.GetContractsAuthorityAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("INPUT_BOX_ADDRESS=%v", config.GetContractsInputBoxAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("GENESIS_BLOCK=%v", config.GetBlockchainGenesisBlock()))
	s.Env = append(s.Env,
		fmt.Sprintf("AUTHORITY_CLAIMER_HTTP_SERVER_PORT=%v", getPort(portOffsetAuthorityClaimer)))
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

func newDispatcher() services.Service {
	var s services.Service
	s.Name = "dispatcher"
	s.HealthcheckPort = getPort(portOffsetDispatcher)
	s.Path = "cartesi-rollups-dispatcher"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog("dispatcher"))
	s.Env = append(s.Env,
		fmt.Sprintf("SC_GRPC_ENDPOINT=http://127.0.0.1:%v", getPort(portOffsetStateServer)))
	s.Env = append(s.Env,
		fmt.Sprintf("SC_DEFAULT_CONFIRMATIONS=%v", config.GetBlockchainReadDepth()))
	s.Env = append(s.Env,
		fmt.Sprintf("REDIS_ENDPOINT=redis://127.0.0.1:%v", getPort(portOffsetRedis)))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_ADDRESS=%v", config.GetContractsDappAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_DEPLOYMENT_BLOCK_NUMBER=%v",
			config.GetContractsDappDeploymentBlockNumber()))
	s.Env = append(s.Env,
		fmt.Sprintf("HISTORY_ADDRESS=%v", config.GetContractsHistoryAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("AUTHORITY_ADDRESS=%v", config.GetContractsAuthorityAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("INPUT_BOX_ADDRESS=%v", config.GetContractsInputBoxAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("RD_EPOCH_DURATION=%v", int(config.GetEpochDuration().Seconds())))
	s.Env = append(s.Env,
		fmt.Sprintf("CHAIN_ID=%v", config.GetBlockchainId()))
	s.Env = append(s.Env,
		fmt.Sprintf("DISPATCHER_HTTP_SERVER_PORT=%v", getPort(portOffsetDispatcher)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newGraphQLServer() services.Service {
	var s services.Service
	s.Name = "graphql-server"
	s.HealthcheckPort = getPort(portOffsetGraphQLHealthcheck)
	s.Path = "cartesi-rollups-graphql-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog("graphql_server"))
	s.Env = append(s.Env,
		fmt.Sprintf("POSTGRES_ENDPOINT=%v", config.GetPostgresEndpoint()))
	s.Env = append(s.Env, "GRAPHQL_HOST=0.0.0.0")
	s.Env = append(s.Env,
		fmt.Sprintf("GRAPHQL_PORT=%v", getPort(portOffsetGraphQLServer)))
	s.Env = append(s.Env,
		fmt.Sprintf("GRAPHQL_HEALTHCHECK_PORT=%v", getPort(portOffsetGraphQLHealthcheck)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newHostRunner() services.Service {
	var s services.Service
	s.Name = "host-runner"
	s.HealthcheckPort = getPort(portOffsetHostRunnerHealthcheck)
	s.Path = "cartesi-rollups-host-runner"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog("host_runner"))
	s.Env = append(s.Env, "GRPC_SERVER_MANAGER_ADDRESS=127.0.0.1")
	s.Env = append(s.Env,
		fmt.Sprintf("GRPC_SERVER_MANAGER_PORT=%v", getPort(portOffsetServerManager)))
	s.Env = append(s.Env, "HTTP_ROLLUP_SERVER_ADDRESS=0.0.0.0")
	s.Env = append(s.Env,
		fmt.Sprintf("HTTP_ROLLUP_SERVER_PORT=%v", getPort(portOffsetHostRunnerRollups)))
	s.Env = append(s.Env,
		fmt.Sprintf("HOST_RUNNER_HEALTHCHECK_PORT=%v", getPort(portOffsetHostRunnerHealthcheck)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newIndexer() services.Service {
	var s services.Service
	s.Name = "indexer"
	s.HealthcheckPort = getPort(portOffsetIndexer)
	s.Path = "cartesi-rollups-indexer"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog("indexer"))
	s.Env = append(s.Env,
		fmt.Sprintf("POSTGRES_ENDPOINT=%v", config.GetPostgresEndpoint()))
	s.Env = append(s.Env,
		fmt.Sprintf("CHAIN_ID=%v", config.GetBlockchainId()))
	s.Env = append(s.Env,
		fmt.Sprintf("DAPP_CONTRACT_ADDRESS=%v", config.GetContractsDappAddress()))
	s.Env = append(s.Env,
		fmt.Sprintf("REDIS_ENDPOINT=redis://127.0.0.1:%v", getPort(portOffsetRedis)))
	s.Env = append(s.Env,
		fmt.Sprintf("INDEXER_HEALTHCHECK_PORT=%v", getPort(portOffsetIndexer)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newInspectServer() services.Service {
	var s services.Service
	s.Name = "inspect-server"
	s.HealthcheckPort = getPort(portOffsetInspectHealthcheck)
	s.Path = "cartesi-rollups-inspect-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog("inspect_server"))
	s.Env = append(s.Env,
		fmt.Sprintf("INSPECT_SERVER_ADDRESS=0.0.0.0:%v", getPort(portOffsetInspectServer)))
	s.Env = append(s.Env,
		fmt.Sprintf("SERVER_MANAGER_ADDRESS=127.0.0.1:%v", getPort(portOffsetServerManager)))
	s.Env = append(s.Env,
		fmt.Sprintf("SESSION_ID=%v", serverManagerSessionId))
	s.Env = append(s.Env,
		fmt.Sprintf("INSPECT_SERVER_HEALTHCHECK_PORT=%v", getPort(portOffsetInspectHealthcheck)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

func newRedis() services.Service {
	var s services.Service
	s.Name = "redis"
	s.HealthcheckPort = getPort(portOffsetRedis)
	s.Path = "redis-server"
	s.Args = append(s.Args, "--port", fmt.Sprint(getPort(portOffsetRedis)))
	// Disable persistence with --save and --appendonly config
	s.Args = append(s.Args, "--save", "")
	s.Args = append(s.Args, "--appendonly", "no")
	return s
}

func newServerManager() services.Service {
	var s services.Service
	s.Name = "server-manager"
	s.HealthcheckPort = getPort(portOffsetServerManager)
	s.Path = "server-manager"
	s.Args = append(s.Args,
		fmt.Sprintf("--manager-address=127.0.0.1:%v", getPort(portOffsetServerManager)))
	s.Env = append(s.Env, "REMOTE_CARTESI_MACHINE_LOG_LEVEL=info")
	if config.GetLogLevel() == config.LogLevelDebug {
		s.Env = append(s.Env, "SERVER_MANAGER_LOG_LEVEL=info")
	} else {
		s.Env = append(s.Env, "SERVER_MANAGER_LOG_LEVEL=warning")
	}
	return s
}

func newStateServer() services.Service {
	var s services.Service
	s.Name = "state-server"
	s.HealthcheckPort = getPort(portOffsetStateServer)
	s.Path = "cartesi-rollups-state-server"
	s.Env = append(s.Env, "LOG_ENABLE_TIMESTAMP=false")
	s.Env = append(s.Env, "LOG_ENABLE_COLOR=false")
	s.Env = append(s.Env, getRustLog("state_server"))
	s.Env = append(s.Env, "SF_CONCURRENT_EVENTS_FETCH=1")
	s.Env = append(s.Env,
		fmt.Sprintf("SF_GENESIS_BLOCK=%v", config.GetBlockchainGenesisBlock()))
	s.Env = append(s.Env,
		fmt.Sprintf("SF_SAFETY_MARGIN=%v", config.GetBlockchainReadDepth()))
	s.Env = append(s.Env,
		fmt.Sprintf("BH_WS_ENDPOINT=%v", config.GetBlockchainWsEndpoint()))
	s.Env = append(s.Env,
		fmt.Sprintf("BH_HTTP_ENDPOINT=%v", config.GetBlockchainHttpEndpoint()))
	s.Env = append(s.Env,
		fmt.Sprintf("BLOCKCHAIN_BLOCK_TIMEOUT=%v", config.GetBlockchainBlockTimeout()))
	s.Env = append(s.Env,
		fmt.Sprintf("SS_SERVER_ADDRESS=127.0.0.1:%v", getPort(portOffsetStateServer)))
	s.Env = append(s.Env, os.Environ()...)
	return s
}

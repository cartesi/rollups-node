package config

import (
	"log/slog"

	. "github.com/cartesi/rollups-node/internal/config"
)

type EVMReaderConfig struct {
	LogLevel               slog.Level
	LogPrettyEnabled       bool
	PostgresEndpoint       Redacted[string]
	BlockchainHttpEndpoint Redacted[string]
	BlockchainWsEndpoint   Redacted[string]
	DefaultBlock           DefaultBlock
	RetryPolicyMaxRetries  uint64
	RetryPolicyMaxDelay    Duration
	ContractsInputBoxAddress string
	ContractsInputBoxDeploymentBlockNumber int64
	BlockchainID          uint64
}

func GetEVMReaderConfig() EVMReaderConfig {
	return EVMReaderConfig{
		LogLevel:              GetLogLevel(),
		LogPrettyEnabled:      false,
		PostgresEndpoint:      Redacted[string]{GetPostgresEndpoint()},
		BlockchainHttpEndpoint:Redacted[string]{GetBlockchainHttpEndpoint()},
		BlockchainWsEndpoint:  Redacted[string]{GetBlockchainWsEndpoint()},
		DefaultBlock:          GetEvmReaderDefaultBlock(),
		RetryPolicyMaxRetries: GetEvmReaderRetryPolicyMaxRetries(),
		RetryPolicyMaxDelay:   GetEvmReaderRetryPolicyMaxDelay(),
		ContractsInputBoxAddress: GetContractsInputBoxAddress(),
		ContractsInputBoxDeploymentBlockNumber: GetContractsInputBoxDeploymentBlockNumber(),
		BlockchainID: GetBlockchainId(),
	}
}

func (c *EVMReaderConfig) ToNodeConfig() NodeConfig {
	return NodeConfig {
		LogLevel: c.LogLevel,
		LogPrettyEnabled: c.LogPrettyEnabled,
		PostgresEndpoint: c.PostgresEndpoint,
		BlockchainHttpEndpoint: c.BlockchainHttpEndpoint,
		BlockchainWsEndpoint: c.BlockchainWsEndpoint,
		EvmReaderDefaultBlock: c.DefaultBlock,
		EvmReaderRetryPolicyMaxRetries: c.RetryPolicyMaxRetries,
		EvmReaderRetryPolicyMaxDelay: c.RetryPolicyMaxDelay,
		ContractsInputBoxAddress: c.ContractsInputBoxAddress,
		ContractsInputBoxDeploymentBlockNumber: c.ContractsInputBoxDeploymentBlockNumber,
		BlockchainID: c.BlockchainID,
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package endtoendtests
package endtoendtests

import (
	"log/slog"

	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

const (
	LocalBlockchainID                  = 31337
	LocalInputBoxDeploymentBlockNumber = 16
	LocalHttpAddress                   = "0.0.0.0"
	LocalHttpPort                      = 10000
	LocalBlockTimeout                  = 120
	LocalFinalityOffset                = 1
	LocalEpochLength                   = 5
)

func NewLocalNodeConfig(localPostgresEndpoint string, localBlockchainHttpEndpoint string,
	localBlockchainWsEndpoint string, snapshotDir string) config.NodeConfig {

	var nodeConfig config.NodeConfig

	book := addresses.GetTestBook()

	//Log
	nodeConfig.LogLevel = slog.LevelInfo
	nodeConfig.LogPrettyEnabled = false

	//Postgres
	nodeConfig.PostgresEndpoint =
		config.Redacted[string]{Value: localPostgresEndpoint}

	//Epoch
	nodeConfig.RollupsEpochLength = LocalEpochLength

	//Blockchain
	nodeConfig.BlockchainID = LocalBlockchainID
	nodeConfig.BlockchainHttpEndpoint =
		config.Redacted[string]{Value: localBlockchainHttpEndpoint}
	nodeConfig.BlockchainWsEndpoint =
		config.Redacted[string]{Value: localBlockchainWsEndpoint}
	nodeConfig.LegacyBlockchainEnabled = false
	nodeConfig.BlockchainFinalityOffset = LocalFinalityOffset
	nodeConfig.BlockchainBlockTimeout = LocalBlockTimeout

	//Contracts
	nodeConfig.ContractsInputBoxAddress = book.InputBox.Hex()
	nodeConfig.ContractsInputBoxDeploymentBlockNumber = LocalInputBoxDeploymentBlockNumber

	//HTTP endpoint
	nodeConfig.HttpAddress = LocalHttpAddress
	nodeConfig.HttpPort = LocalHttpPort

	//Features
	nodeConfig.FeatureClaimerEnabled = true
	nodeConfig.FeatureMachineHashCheckEnabled = true

	//Experimental
	nodeConfig.ExperimentalSunodoValidatorEnabled = false

	//Auth
	nodeConfig.Auth = config.AuthMnemonic{
		Mnemonic:     config.Redacted[string]{Value: ethutil.FoundryMnemonic},
		AccountIndex: config.Redacted[int]{Value: 0},
	}

	nodeConfig.SnapshotDir = snapshotDir

	return nodeConfig
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package endtoendtests
package endtoendtests

import (
	"fmt"
	"log/slog"
	"time"

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
	LocalEpochDurationInSeconds        = 240
)

func NewLocalNodeConfig(localPostgresEnpoint string, localBlockchainHttpEndpoint string,
	localBlockchainWsEnpoint string, snapshotDir string) config.NodeConfig {

	var nodeConfig config.NodeConfig

	book := addresses.GetTestBook()

	//Log
	nodeConfig.LogLevel = slog.LevelInfo
	nodeConfig.LogPretty = false

	//Postgres
	nodeConfig.PostgresEndpoint =
		config.Redacted[string]{Value: localPostgresEnpoint}

	//Epoch
	nodeConfig.RollupsEpochDuration, _ =
		time.ParseDuration(fmt.Sprintf("%ds", LocalEpochDurationInSeconds))

	//Blochain
	nodeConfig.BlockchainID = LocalBlockchainID
	nodeConfig.BlockchainHttpEndpoint =
		config.Redacted[string]{Value: localBlockchainHttpEndpoint}
	nodeConfig.BlockchainWsEndpoint =
		config.Redacted[string]{Value: localBlockchainWsEnpoint}
	nodeConfig.BlockchainIsLegacy = false
	nodeConfig.BlockchainFinalityOffset = LocalFinalityOffset
	nodeConfig.BlockchainBlockTimeout = LocalBlockTimeout

	//Contracts
	nodeConfig.ContractsApplicationAddress = book.Application.Hex()
	nodeConfig.ContractsIConsensusAddress = book.Authority.Hex()
	nodeConfig.ContractsInputBoxAddress = book.InputBox.Hex()
	nodeConfig.ContractsInputBoxDeploymentBlockNumber = LocalInputBoxDeploymentBlockNumber

	//HTTP endpoint
	nodeConfig.HttpAddress = LocalHttpAddress
	nodeConfig.HttpPort = LocalHttpPort

	//Features
	nodeConfig.FeatureDisableClaimer = false
	nodeConfig.FeatureDisableMachineHashCheck = false

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

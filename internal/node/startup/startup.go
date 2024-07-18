// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package startup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

// Validates the Node Database Schema Version
func ValidateSchema(postgresEndpoint string) error {
	schemaManager, err := repository.NewSchemaManager(postgresEndpoint)
	if err != nil {
		return err
	}
	defer schemaManager.Close()
	err = schemaManager.ValidateSchemaVersion()
	if err != nil {
		return err
	}

	return nil

}

// Configure the node logs
func ConfigLogs(config config.NodeConfig) {
	opts := &tint.Options{
		Level:      config.LogLevel,
		AddSource:  config.LogLevel == slog.LevelDebug,
		NoColor:    !config.LogPrettyEnabled || !isatty.IsTerminal(os.Stdout.Fd()),
		TimeFormat: "2006-01-02T15:04:05.000", // RFC3339 with milliseconds and without timezone
	}
	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

// Handles Persistent Config
func SetupNodePersistentConfig(
	ctx context.Context,
	database *repository.Database,
	config config.NodeConfig,
) (*model.NodePersistentConfig, error) {
	nodePersistentConfig, err := database.GetNodeConfig(ctx)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf(
				"Could not retrieve persistent config from Database. Error %v",
				err,
			)
		}
	}

	if nodePersistentConfig == nil {
		nodePersistentConfig = &model.NodePersistentConfig{
			DefaultBlock:            config.EvmReaderDefaultBlock,
			InputBoxDeploymentBlock: uint64(config.ContractsInputBoxDeploymentBlockNumber),
			InputBoxAddress:         common.HexToAddress(config.ContractsInputBoxAddress),
			ChainId:                 config.BlockchainID,
			IConsensusAddress:       common.HexToAddress(config.ContractsIConsensusAddress),
		}
		slog.Info(
			"No persistent config found at the database. Setting it up",
			"persistent config",
			nodePersistentConfig,
		)

		err = database.InsertNodeConfig(ctx, nodePersistentConfig)
		if err != nil {
			return nil, fmt.Errorf("Couldn't insert database config. Error : %v", err)
		}
	} else {
		slog.Info(
			"Node was already configured. Using previous persistent config",
			"persistent config",
			nodePersistentConfig,
		)
	}

	return nodePersistentConfig, nil
}

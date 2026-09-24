// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const anvilTestEpochLength uint64 = 10

func CreateAnvilSnapshotAndDeployApp(
	ctx context.Context,
	client *ethclient.Client,
	factoryAddr common.Address,
	templateHash common.Hash,
	inputBoxAddress common.Address,
	salt string,
) (common.Address, func(), error) {
	zero := common.Address{}
	if client == nil {
		return zero, nil, fmt.Errorf("ethclient Client is nil")
	}

	// Create a snapshot of the current state
	snapshotID, err := CreateAnvilSnapshot(client.Client())
	if err != nil {
		return zero, nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	chainId, err := client.ChainID(ctx)
	if err != nil {
		_ = RevertToAnvilSnapshot(client.Client(), snapshotID)
		return zero, nil, fmt.Errorf("failed to retrieve chainID from Anvil: %w", err)
	}

	privateKey, err := MnemonicToPrivateKey(FoundryMnemonic, 0)
	if err != nil {
		_ = RevertToAnvilSnapshot(client.Client(), snapshotID)
		return zero, nil, fmt.Errorf("failed to create privateKey: %w", err)
	}

	txOpts, err := bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	if err != nil {
		_ = RevertToAnvilSnapshot(client.Client(), snapshotID)
		return zero, nil, fmt.Errorf("failed to create TransactOpts: %w", err)
	}

	parsedSalt, err := ParseSalt(salt)
	if err != nil {
		_ = RevertToAnvilSnapshot(client.Client(), snapshotID)
		return zero, nil, fmt.Errorf("failed to parse salt: %w", err)
	}

	deployment := &SelfhostedApplicationDeployment{
		FactoryAddress:        factoryAddr,
		AuthorityOwnerAddress: txOpts.From,
		TemplateHash:          templateHash,
		InputBoxAddress:       inputBoxAddress,
		EpochLength:           anvilTestEpochLength,
		Salt:                  parsedSalt,
	}
	applicationAddress, _, err := deployment.Deploy(ctx, client, NewStaticTransactOptsFactory(txOpts))
	if err != nil {
		_ = RevertToAnvilSnapshot(client.Client(), snapshotID)
		return zero, nil, fmt.Errorf("failed to deploy self-hosted application: %w", err)
	}

	// Define a cleanup function to revert to the snapshot
	cleanup := func() {
		err := RevertToAnvilSnapshot(client.Client(), snapshotID)
		if err != nil {
			log.Printf("failed to revert to snapshot: %v", err)
		}
	}

	return applicationAddress, cleanup, nil
}

func CreateAnvilSnapshot(rpcClient *rpc.Client) (string, error) {
	var snapshotID string
	// Using the JSON-RPC method "evm_snapshot" to create a snapshot
	err := rpcClient.Call(&snapshotID, "evm_snapshot")
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}
	return snapshotID, nil
}

func RevertToAnvilSnapshot(rpcClient *rpc.Client, snapshotID string) error {
	var success bool
	// Using the JSON-RPC method "evm_revert" to revert to the snapshot
	err := rpcClient.Call(&success, "evm_revert", snapshotID)
	if err != nil {
		return fmt.Errorf("failed to revert to snapshot: %w", err)
	}
	if !success {
		return fmt.Errorf("failed to revert to snapshot with ID: %s", snapshotID)
	}
	return nil
}

// Mines a new block
func MineNewBlock(
	ctx context.Context,
	client *ethclient.Client,
) (uint64, error) {
	if client == nil {
		return 0, fmt.Errorf("MineNewBlock: client is nil")
	}
	rpcClient := client.Client()
	err := rpcClient.CallContext(ctx, nil, "evm_mine")
	if err != nil {
		return 0, err
	}
	return client.BlockNumber(ctx)
}

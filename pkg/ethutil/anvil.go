// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

func CreateAnvilSnapshotAndDeployApp(ctx context.Context, blockchainHttpEndpoint string, templateHash string) (common.Address, func(), error) {
	var contractAddr common.Address
	// Connect to Anvil (replace with appropriate RPC URL)
	client, err := ethclient.Dial(blockchainHttpEndpoint)
	if err != nil {
		return contractAddr, nil, fmt.Errorf("failed to connect to Anvil: %w", err)
	}

	// Create a snapshot of the current state
	snapshotID, err := CreateAnvilSnapshot(client.Client())
	if err != nil {
		return contractAddr, nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	signer, err := NewMnemonicSigner(ctx, client, FoundryMnemonic, 0)
	if err != nil {
		_ = RevertToAnvilSnapshot(client.Client(), snapshotID)
		return contractAddr, nil, fmt.Errorf("failed to create signer: %w", err)
	}

	factoryAddr := common.HexToAddress("0x0678FAA399F0193Fb9212BE41590316D275b1392") // FIXME get from book
	owner := signer.Account()
	salt := "0000000000000000000000000000000000000000000000000000000000000000"
	// Deploy the application contract
	contractAddr, err = DeploySelfHostedApplication(ctx, client, signer, factoryAddr, owner,
		templateHash, salt)
	if err != nil {
		_ = RevertToAnvilSnapshot(client.Client(), snapshotID)
		return contractAddr, nil, fmt.Errorf("failed to deploy application contract: %w", err)
	}

	// Define a cleanup function to revert to the snapshot
	cleanup := func() {
		err := RevertToAnvilSnapshot(client.Client(), snapshotID)
		if err != nil {
			log.Printf("failed to revert to snapshot: %v", err)
		}
	}

	return contractAddr, cleanup, nil
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

// Advances the Devnet timestamp
func AdvanceDevnetTime(ctx context.Context,
	blockchainHttpEndpoint string,
	timeInSeconds int,
) error {
	client, err := rpc.DialContext(ctx, blockchainHttpEndpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.CallContext(ctx, nil, "evm_increaseTime", timeInSeconds)

}

// Sets the timestamp for the next block at Devnet
func SetNextDevnetBlockTimestamp(
	ctx context.Context,
	blockchainHttpEndpoint string,
	timestamp int64,
) error {

	client, err := rpc.DialContext(ctx, blockchainHttpEndpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.CallContext(ctx, nil, "evm_setNextBlockTimestamp", timestamp)
}

// Mines a new block
func MineNewBlock(
	ctx context.Context,
	blockchainHttpEndpoint string,
) (uint64, error) {
	client, err := rpc.DialContext(ctx, blockchainHttpEndpoint)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	err = client.CallContext(ctx, nil, "evm_mine")
	if err != nil {
		return 0, err
	}
	ethClient, err := ethclient.DialContext(ctx, blockchainHttpEndpoint)
	if err != nil {
		return 0, err
	}
	defer ethClient.Close()
	return ethClient.BlockNumber(ctx)
}

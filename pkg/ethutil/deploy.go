// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthorityfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iselfhostedapplicationfactory"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func DeployApplication(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
	applicationFactoryAddr common.Address,
	authorityAddr common.Address,
	owner common.Address,
	templateHash string,
	dataAvailability []byte,
	salt string,
	quiet bool,
) (common.Address, error) {

	templateHashBytes, err := hex.DecodeString(templateHash)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to decode template hash: %v", err)
	}
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to decode salt: %v", err)
	}

	factory, err := iapplicationfactory.NewIApplicationFactory(applicationFactoryAddr, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to instantiate contract: %v", err)
	}

	tx, err := factory.NewApplication(txOpts, authorityAddr, owner, toBytes32(templateHashBytes), dataAvailability, toBytes32(saltBytes))
	if err != nil {
		return common.Address{}, fmt.Errorf("transaction failed: %v", err)
	}

	if !quiet {
		fmt.Printf("Transaction submitted: %s\n", tx.Hash().Hex())
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to wait for transaction mining: %v", err)
	}

	if receipt.Status == 1 {
		if !quiet {
			fmt.Println("Transaction successful!")
		}
	} else {
		return common.Address{}, fmt.Errorf("transaction failed")
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		// Parse log for ApplicationCreated event
		event, err := factory.ParseApplicationCreated(*vLog)
		if err != nil {
			continue // Skip logs that don't match
		}

		if !quiet {
			fmt.Printf("New Application contract deployed at address: %s\n", event.AppContract.Hex())
		}
		return event.AppContract, nil
	}

	return common.Address{}, fmt.Errorf("failed to find ApplicationCreated event in receipt logs")
}

func DeploySelfHostedApplication(
	ctx context.Context,
	client *ethclient.Client,
	transactionOpts *bind.TransactOpts,
	shAppFactoryAddr common.Address,
	ownerAddr common.Address,
	templateHash common.Hash,
	dataAvailability []byte,
	salt string,
) (common.Address, error) {
	var appAddr common.Address
	if client == nil {
		return appAddr, fmt.Errorf("DeploySelfHostedApplication: client is nil")
	}

	saltBytes := common.Hex2Bytes(salt)

	factory, err := iselfhostedapplicationfactory.NewISelfHostedApplicationFactory(shAppFactoryAddr, client)
	if err != nil {
		return appAddr, fmt.Errorf("Failed to instantiate contract: %v", err)
	}

	receipt, err := sendTransaction(
		ctx, client, transactionOpts, big.NewInt(0), GasLimit,
		func(txOpts *bind.TransactOpts) (*types.Transaction, error) {
			return factory.DeployContracts(txOpts, ownerAddr, big.NewInt(10), ownerAddr, templateHash,
				dataAvailability, toBytes32(saltBytes))
		},
	)
	if err != nil {
		return appAddr, err
	}

	appFactoryAddress, err := factory.GetApplicationFactory(nil)
	if err != nil {
		return appAddr, err
	}

	appFactory, err := iapplicationfactory.NewIApplicationFactory(appFactoryAddress, client)
	if err != nil {
		return appAddr, err
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		event, err := appFactory.ParseApplicationCreated(*vLog)
		if err != nil {
			continue // Skip logs that don't match
		}

		return event.AppContract, nil
	}

	return appAddr, fmt.Errorf("Failed to find ApplicationCreated event in receipt logs")
}

func DeployAuthority(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
	authorityFactoryAddr common.Address,
	owner common.Address,
	epochLength uint64,
	salt string,
	quiet bool,
) (common.Address, error) {
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to decode salt: %v", err)
	}

	contract, err := iauthorityfactory.NewIAuthorityFactory(authorityFactoryAddr, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to instantiate contract: %v", err)
	}

	tx, err := contract.NewAuthority0(txOpts, owner, big.NewInt(int64(epochLength)), toBytes32(saltBytes))
	if err != nil {
		return common.Address{}, fmt.Errorf("transaction failed: %v", err)
	}

	if !quiet {
		fmt.Printf("Transaction submitted: %s\n", tx.Hash().Hex())
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to wait for transaction mining: %v", err)
	}

	if receipt.Status == 1 {
		if !quiet {
			fmt.Println("Transaction successful!")
		}
	} else {
		return common.Address{}, fmt.Errorf("transaction failed")
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		// Parse log for ApplicationCreated event
		event, err := contract.ParseAuthorityCreated(*vLog)
		if err != nil {
			continue // Skip logs that don't match
		}

		if !quiet {
			fmt.Printf("New Authority contract deployed at address: %s\n", event.Authority.Hex())
		}
		return event.Authority, nil
	}

	return common.Address{}, fmt.Errorf("failed to find AuthorityCreated event in receipt logs")
}

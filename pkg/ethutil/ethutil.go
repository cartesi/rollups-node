// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package contains functions to help using the Go-ethereum library.
// It is not the objective of this package to replace or hide Go-ethereum.
package ethutil

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/contracts/iselfhostedapplicationfactory"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Gas limit when sending transactions.
const GasLimit = 30_000_000

// Dev mnemonic used by Foundry/Anvil.
const FoundryMnemonic = "test test test test test test test test test test test junk"

// Interface that sign blockchain transactions.
type Signer interface {

	// Create the base transaction used in the contract bindings.
	MakeTransactor() (*bind.TransactOpts, error)

	// Get the account address of the signer.
	Account() common.Address
}

func DeploySelfHostedApplication(
	ctx context.Context,
	client *ethclient.Client,
	signer Signer,
	shAppFactoryAddr common.Address,
	ownerAddr common.Address,
	templateHash string,
	salt string,
) (common.Address, error) {
	var appAddr common.Address
	templateHashBytes := common.Hex2Bytes(templateHash)
	saltBytes := common.Hex2Bytes(salt)

	factory, err := iselfhostedapplicationfactory.NewISelfHostedApplicationFactory(shAppFactoryAddr, client)
	if err != nil {
		return appAddr, fmt.Errorf("Failed to instantiate contract: %v", err)
	}

	receipt, err := sendTransaction(
		ctx, client, signer, big.NewInt(0), GasLimit,
		func(txOpts *bind.TransactOpts) (*types.Transaction, error) {
			return factory.DeployContracts(txOpts, ownerAddr, big.NewInt(10), ownerAddr, toBytes32(templateHashBytes), toBytes32(saltBytes))
		},
	)
	if err != nil {
		return appAddr, err
	}
	// Parse logs to get the address of the new application contract
	contractABI, err := iapplicationfactory.IApplicationFactoryMetaData.GetAbi()
	if err != nil {
		return appAddr, fmt.Errorf("Failed to parse IApplicationFactory ABI: %v", err)
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		event := struct {
			Consensus    common.Address
			AppOwner     common.Address
			TemplateHash [32]byte
			AppContract  common.Address
		}{}

		// Parse log for ApplicationCreated event
		err := contractABI.UnpackIntoInterface(&event, "ApplicationCreated", vLog.Data)
		if err != nil {
			continue // Skip logs that don't match
		}

		return event.AppContract, nil
	}

	return appAddr, fmt.Errorf("Failed to find ApplicationCreated event in receipt logs")
}

// Add input to the input box for the given DApp address.
// This function waits until the transaction is added to a block and return the input index.
func AddInput(
	ctx context.Context,
	client *ethclient.Client,
	book *addresses.Book,
	application common.Address,
	signer Signer,
	input []byte,
) (int, error) {
	inputBox, err := iinputbox.NewIInputBox(book.InputBox, client)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to InputBox contract: %v", err)
	}
	receipt, err := sendTransaction(
		ctx, client, signer, big.NewInt(0), GasLimit,
		func(txOpts *bind.TransactOpts) (*types.Transaction, error) {
			return inputBox.AddInput(txOpts, application, input)
		},
	)
	if err != nil {
		return 0, err
	}
	return getInputIndex(book, inputBox, receipt)
}

// Get input index in the transaction by looking at the event logs.
func getInputIndex(
	book *addresses.Book,
	inputBox *iinputbox.IInputBox,
	receipt *types.Receipt,
) (int, error) {
	for _, log := range receipt.Logs {
		if log.Address != book.InputBox {
			continue
		}
		inputAdded, err := inputBox.ParseInputAdded(*log)
		if err != nil {
			return 0, fmt.Errorf("failed to parse input added event: %v", err)
		}
		// We assume that int will fit all dapp inputs
		inputIndex := int(inputAdded.Index.Int64())
		return inputIndex, nil
	}
	return 0, fmt.Errorf("input index not found")
}

// Get the given input of the given DApp from the input box.
// Return the event with the input sender and payload.
func GetInputFromInputBox(
	client *ethclient.Client,
	book *addresses.Book,
	application common.Address,
	inputIndex int,
) (*iinputbox.IInputBoxInputAdded, error) {
	inputBox, err := iinputbox.NewIInputBox(book.InputBox, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to InputBox contract: %v", err)
	}
	it, err := inputBox.FilterInputAdded(
		nil,
		[]common.Address{application},
		[]*big.Int{big.NewInt(int64(inputIndex))},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to filter input added: %v", err)
	}
	defer it.Close()
	if !it.Next() {
		return nil, fmt.Errorf("event not found")
	}
	return it.Event, nil
}

// ValidateNotice validates the given notice for the specified Dapp.
// It returns nil if the notice is valid and an execution-reverted error otherwise.
func ValidateOutput(
	ctx context.Context,
	client *ethclient.Client,
	book *addresses.Book,
	appAddr common.Address,
	output []byte,
	proof *iapplication.OutputValidityProof,
) error {
	app, err := iapplication.NewIApplication(appAddr, client)
	if err != nil {
		return fmt.Errorf("failed to connect to CartesiDapp contract: %v", err)
	}
	return app.ValidateOutput(&bind.CallOpts{Context: ctx}, output, *proof)
}

// Executes a voucher given its payload, destination and proof.
// This function waits until the transaction is added to a block and returns the transaction hash.
func ExecuteOutput(
	ctx context.Context,
	client *ethclient.Client,
	book *addresses.Book,
	appAddr common.Address,
	signer Signer,
	output []byte,
	proof *iapplication.OutputValidityProof,
) (*common.Hash, error) {
	app, err := iapplication.NewIApplication(appAddr, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CartesiDapp contract: %v", err)
	}
	receipt, err := sendTransaction(
		ctx, client, signer, big.NewInt(0), GasLimit,
		func(txOpts *bind.TransactOpts) (*types.Transaction, error) {
			return app.ExecuteOutput(txOpts, output, *proof)
		},
	)
	if err != nil {
		return nil, err
	}

	return &receipt.TxHash, nil
}

func toBytes32(data []byte) [32]byte {
	var arr [32]byte
	if len(data) > 32 {
		copy(arr[:], data[:32])
	} else {
		copy(arr[:], data)
	}
	return arr
}

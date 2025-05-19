// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package contains functions to help using the Go-ethereum library.
// It is not the objective of this package to replace or hide Go-ethereum.
package ethutil

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Gas limit when sending transactions.
const GasLimit = 30_000_000

// Dev mnemonic used by Foundry/Anvil.
const FoundryMnemonic = "test test test test test test test test test test test junk"

// Add input to the input box for the given DApp address.
// This function waits until the transaction is added to a block and return the input index.
func AddInput(
	ctx context.Context,
	client *ethclient.Client,
	transactionOpts *bind.TransactOpts,
	inputBoxAddress common.Address,
	application common.Address,
	input []byte,
) (uint64, uint64, error) {
	if client == nil {
		return 0, 0, fmt.Errorf("AddInput: client is nil")
	}
	inputBox, err := iinputbox.NewIInputBox(inputBoxAddress, client)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to connect to InputBox contract: %v", err)
	}
	receipt, err := sendTransaction(
		ctx, client, transactionOpts, big.NewInt(0), GasLimit,
		func(txOpts *bind.TransactOpts) (*types.Transaction, error) {
			return inputBox.AddInput(txOpts, application, input)
		},
	)
	if err != nil {
		return 0, 0, err
	}
	index, err := getInputIndex(inputBoxAddress, inputBox, receipt)
	return index, receipt.BlockNumber.Uint64(), nil
}

// Get input index in the transaction by looking at the event logs.
func getInputIndex(
	inputBoxAddress common.Address,
	inputBox *iinputbox.IInputBox,
	receipt *types.Receipt,
) (uint64, error) {
	for _, log := range receipt.Logs {
		if log.Address != inputBoxAddress {
			continue
		}
		inputAdded, err := inputBox.ParseInputAdded(*log)
		if err != nil {
			return 0, fmt.Errorf("failed to parse input added event: %v", err)
		}
		// We assume that uint64 will fit all dapp inputs for now
		return inputAdded.Index.Uint64(), nil
	}
	return 0, fmt.Errorf("input index not found")
}

// Get the given input of the given DApp from the input box.
// Return the event with the input sender and payload.
func GetInputFromInputBox(
	client *ethclient.Client,
	inputBoxAddress common.Address,
	application common.Address,
	inputIndex uint64,
) (*iinputbox.IInputBoxInputAdded, error) {
	if client == nil {
		return nil, fmt.Errorf("GetInputFromInputBox: client is nil")
	}
	inputBox, err := iinputbox.NewIInputBox(inputBoxAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to InputBox contract: %v", err)
	}
	it, err := inputBox.FilterInputAdded(
		nil,
		[]common.Address{application},
		[]*big.Int{new(big.Int).SetUint64(inputIndex)},
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
	appAddr common.Address,
	index uint64,
	output []byte,
	outputHashesSiblings []common.Hash,
) error {
	if client == nil {
		return fmt.Errorf("ValidateOutput: client is nil")
	}
	proof := iapplication.OutputValidityProof{
		OutputIndex:          index,
		OutputHashesSiblings: make([][32]byte, len(outputHashesSiblings)),
	}

	for i, hash := range outputHashesSiblings {
		copy(proof.OutputHashesSiblings[i][:], hash[:])
	}

	app, err := iapplication.NewIApplication(appAddr, client)
	if err != nil {
		return fmt.Errorf("failed to connect to CartesiDapp contract: %v", err)
	}
	return app.ValidateOutput(&bind.CallOpts{Context: ctx}, output, proof)
}

// Executes a voucher given its payload, destination and proof.
// This function waits until the transaction is added to a block and returns the transaction hash.
func ExecuteOutput(
	ctx context.Context,
	client *ethclient.Client,
	transactionOpts *bind.TransactOpts,
	appAddr common.Address,
	index uint64,
	output []byte,
	outputHashesSiblings []common.Hash,
) (*common.Hash, error) {
	if client == nil {
		return nil, fmt.Errorf("ExecuteOutput: client is nil")
	}
	proof := iapplication.OutputValidityProof{
		OutputIndex:          index,
		OutputHashesSiblings: make([][32]byte, len(outputHashesSiblings)),
	}

	for i, hash := range outputHashesSiblings {
		copy(proof.OutputHashesSiblings[i][:], hash[:])
	}

	app, err := iapplication.NewIApplication(appAddr, client)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CartesiDapp contract: %v", err)
	}
	receipt, err := sendTransaction(
		ctx, client, transactionOpts, big.NewInt(0), GasLimit,
		func(txOpts *bind.TransactOpts) (*types.Transaction, error) {
			return app.ExecuteOutput(txOpts, output, proof)
		},
	)
	if err != nil {
		return nil, err
	}

	return &receipt.TxHash, nil
}

// Retrieves the template hash from the application contract.
func GetTemplateHash(
	ctx context.Context,
	client *ethclient.Client,
	applicationAddress common.Address,
) (*common.Hash, error) {
	if client == nil {
		return nil, fmt.Errorf("get template hash: client is nil")
	}
	cartesiApplication, err := iapplication.NewIApplicationCaller(
		applicationAddress,
		client,
	)
	if err != nil {
		return nil, fmt.Errorf("get template hash failed to instantiate binding: %w", err)
	}
	var hash common.Hash
	hash, err = cartesiApplication.GetTemplateHash(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("get template hash failed to call contract method: %w", err)
	}
	return &hash, nil
}

func GetConsensus(
	ctx context.Context,
	client *ethclient.Client,
	appAddress common.Address,
) (common.Address, error) {
	if client == nil {
		return common.Address{}, fmt.Errorf("get consensus: client is nil")
	}
	app, err := iapplication.NewIApplication(appAddress, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to instantiate contract: %v", err)
	}
	consensus, err := app.GetOutputsMerkleRootValidator(&bind.CallOpts{Context: ctx})
	if err != nil {
		return common.Address{}, fmt.Errorf("error retrieving application epoch length: %v", err)
	}
	return consensus, nil
}

func GetDataAvailability(
	ctx context.Context,
	client *ethclient.Client,
	appAddress common.Address,
) ([]byte, error) {
	if client == nil {
		return nil, fmt.Errorf("get dataAvailability: client is nil")
	}
	app, err := iapplication.NewIApplication(appAddress, client)
	if err != nil {
		return nil, fmt.Errorf("Failed to instantiate contract: %v", err)
	}
	dataAvailability, err := app.GetDataAvailability(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("error retrieving application epoch length: %v", err)
	}
	return dataAvailability, nil
}

func GetEpochLength(
	ctx context.Context,
	client *ethclient.Client,
	consensusAddr common.Address,
) (uint64, error) {
	if client == nil {
		return 0, fmt.Errorf("get epoch length: client is nil")
	}
	consensus, err := iconsensus.NewIConsensus(consensusAddr, client)
	if err != nil {
		return 0, fmt.Errorf("Failed to instantiate contract: %v", err)
	}
	epochLengthRaw, err := consensus.GetEpochLength(&bind.CallOpts{Context: ctx})
	if err != nil {
		return 0, fmt.Errorf("error retrieving application epoch length: %v", err)
	}
	return epochLengthRaw.Uint64(), nil
}

func GetInputBoxDeploymentBlock(
	ctx context.Context,
	client *ethclient.Client,
	inputBoxAddress common.Address,
) (*big.Int, error) {
	if client == nil {
		return nil, fmt.Errorf("get epoch length: client is nil")
	}
	inputbox, err := iinputbox.NewIInputBox(inputBoxAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create input box instance: %w", err)
	}
	block, err := inputbox.GetDeploymentBlockNumber(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("error retrieving inputbox deployment block: %v", err)
	}
	return block, nil
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

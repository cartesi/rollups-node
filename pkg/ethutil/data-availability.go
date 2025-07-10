// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/dataavailability"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func DefaultDA(client *ethclient.Client, inputBoxAddress common.Address) (common.Address, uint64, []byte, error) {
	parsedABI, err := dataavailability.DataAvailabilityMetaData.GetAbi()
	if err != nil {
		return common.Address{}, 0, nil, fmt.Errorf("failed to get data availability ABI: %w", err)
	}

	encodedDA, err := parsedABI.Pack("InputBox", inputBoxAddress)
	if err != nil {
		return common.Address{}, 0, nil, fmt.Errorf("failed pack input box data availability string with: %w", err)
	}

	inputBox, err := iinputbox.NewIInputBox(inputBoxAddress, client)
	if err != nil {
		return common.Address{}, 0, nil, fmt.Errorf("failed to create input box instance: %w", err)
	}

	inputBoxBlock, err := inputBox.GetDeploymentBlockNumber(nil)
	if err != nil {
		return common.Address{}, 0, nil, fmt.Errorf("failed to get deployment block number: %w", err)
	}

	return inputBoxAddress, inputBoxBlock.Uint64(), encodedDA, nil
}

func CustomDA(client *ethclient.Client, dataAvailability string) (common.Address, uint64, []byte, error) {
	if len(dataAvailability) < 3 || (!strings.HasPrefix(dataAvailability, "0x") && !strings.HasPrefix(dataAvailability, "0X")) {
		return common.Address{}, 0, nil, fmt.Errorf("data Availability should be an ABI encoded value")
	}

	s := dataAvailability[2:]
	encodedDA, err := hex.DecodeString(s)
	if err != nil {
		return common.Address{}, 0, nil, fmt.Errorf("error parsing Data Availability value: %w", err)
	}

	inputBoxAddress, inputBoxBlock, err := DecodeDA(client, encodedDA)
	if err != nil {
		return common.Address{}, 0, nil, fmt.Errorf("error decoding Data Availability value: %w", err)
	}

	return inputBoxAddress, inputBoxBlock, encodedDA, nil

}

func DecodeDA(client *ethclient.Client, encodedDA []byte) (common.Address, uint64, error) {
	parsedAbi, err := dataavailability.DataAvailabilityMetaData.GetAbi()
	if err != nil {
		return common.Address{}, 0, fmt.Errorf("failed to get ABI: %w", err)
	}

	if len(encodedDA) < model.DATA_AVAILABILITY_SELECTOR_SIZE {
		return common.Address{}, 0, fmt.Errorf("invalid Data Availability")
	}

	method, err := parsedAbi.MethodById(encodedDA[:model.DATA_AVAILABILITY_SELECTOR_SIZE])
	if err != nil {
		return common.Address{}, 0, fmt.Errorf("failed to get method by ID: %w", err)
	}

	args, err := method.Inputs.Unpack(encodedDA[model.DATA_AVAILABILITY_SELECTOR_SIZE:])
	if err != nil {
		return common.Address{}, 0, fmt.Errorf("failed to unpack inputs: %w", err)
	}

	if len(args) == 0 {
		return common.Address{}, 0, fmt.Errorf("invalid Data Availability. Should at least contain InputBox Address")
	}

	var inputBoxAddress common.Address
	switch addr := args[0].(type) {
	case common.Address:
		inputBoxAddress = addr
	default:
		return common.Address{}, 0, fmt.Errorf("first argument in Data Availability is not an address (got %T)", args[0])
	}

	inputbox, err := iinputbox.NewIInputBox(inputBoxAddress, client)
	if err != nil {
		return common.Address{}, 0, fmt.Errorf("failed to create input box instance: %w", err)
	}

	inputBoxBlock, err := inputbox.GetDeploymentBlockNumber(nil)
	if err != nil {
		return common.Address{}, 0, fmt.Errorf("failed to get deployment block number: %w", err)
	}

	return inputBoxAddress, inputBoxBlock.Uint64(), nil
}

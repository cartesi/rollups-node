// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/cartesi/rollups-node/pkg/contracts/application"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Validates if the hash from the Cartesi Machine at machineDir matches the template hash onchain.
// It returns an error if it doesn't.
func validateMachineHash(
	ctx context.Context,
	machineDir string,
	applicationAddress string,
	ethereumNodeAddr string,
) error {
	offchainHash, err := readHash(machineDir)
	if err != nil {
		return err
	}
	onchainHash, err := getTemplateHash(ctx, applicationAddress, ethereumNodeAddr)
	if err != nil {
		return err
	}
	if offchainHash != onchainHash {
		return fmt.Errorf(
			"validate machine hash: hash mismatch; expected %v but got %v",
			onchainHash,
			offchainHash,
		)
	}
	return nil
}

// Reads the Cartesi Machine hash from machineDir. Returns it as a hex string or
// an error
func readHash(machineDir string) (string, error) {
	path := path.Join(machineDir, "hash")
	hash, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read hash: %w", err)
	} else if len(hash) != common.HashLength {
		return "", fmt.Errorf(
			"read hash: wrong size; expected %v bytes but read %v",
			common.HashLength,
			len(hash),
		)
	}
	return common.Bytes2Hex(hash), nil
}

// Retrieves the template hash from the application contract. Returns it as a
// hex string or an error
func getTemplateHash(
	ctx context.Context,
	applicationAddress string,
	ethereumNodeAddr string,
) (string, error) {
	client, err := ethclient.DialContext(ctx, ethereumNodeAddr)
	if err != nil {
		return "", fmt.Errorf("get template hash: %w", err)
	}
	cartesiApplication, err := application.NewApplicationCaller(
		common.HexToAddress(applicationAddress),
		client,
	)
	if err != nil {
		return "", fmt.Errorf("get template hash: %w", err)
	}
	hash, err := cartesiApplication.GetTemplateHash(&bind.CallOpts{Context: ctx})
	if err != nil {
		return "", fmt.Errorf("get template hash: %w", err)
	}
	return common.Bytes2Hex(hash[:]), nil
}

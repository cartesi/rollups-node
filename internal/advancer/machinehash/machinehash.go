// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machinehash

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/advancer/snapshot"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/common"
)

// Validates if the hash from the Cartesi Machine at machineDir matches the template hash onchain.
// It returns an error if it doesn't.
func ValidateMachineHash(
	ctx context.Context,
	machineDir string,
	applicationAddress common.Address,
	ethereumNodeAddr string,
) error {
	offchainHash, err := snapshot.ReadHash(machineDir)
	if err != nil {
		return err
	}
	onchainHash, err := ethutil.GetTemplateHash(ctx, applicationAddress, ethereumNodeAddr)
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

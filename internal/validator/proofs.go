// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// generateProofs will create the proofs for all Outputs within an Epoch.
// It returns the proofs and the root hash of the Merkle tree used to generate them
func generateProofs(
	ctx context.Context,
	inputRange InputRange,
	machineStateHash hexutil.Bytes,
	outputs []Output,
) ([]Proof, error) {
	proofs := make([]Proof, 0, len(outputs))
	for range outputs {
		proofs = append(proofs, Proof{OutputsEpochRootHash: common.Hex2Bytes("0xdeadbeef")})
	}
	return proofs, nil
}

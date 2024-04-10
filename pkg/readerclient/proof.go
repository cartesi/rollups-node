// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package readerclient

import (
	"fmt"

	"github.com/cartesi/rollups-node/pkg/contracts/application"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Proof struct {
	// Local input index within the context of the related epoch
	InputIndexWithinEpoch int `json:"inputIndexWithinEpoch"`
	// Output index within the context of the input that produced it
	OutputIndexWithinInput int `json:"outputIndexWithinInput"`
	// Merkle root of all output hashes of the related input
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	OutputHashesRootHash hexutil.Bytes `json:"outputHashesRootHash"`
	// Merkle root of all voucher hashes of the related epoch
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	VouchersEpochRootHash hexutil.Bytes `json:"vouchersEpochRootHash"`
	// Merkle root of all notice hashes of the related epoch
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	NoticesEpochRootHash hexutil.Bytes `json:"noticesEpochRootHash"`
	// Hash of the machine state claimed for the related epoch
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	MachineStateHash hexutil.Bytes `json:"machineStateHash"`
	// Proof that this output hash is in the output-hashes merkle tree.
	// This array of siblings is bottom-up ordered (from the leaf to the root).
	// Each hash is given in Ethereum hex binary format (32 bytes), starting with '0x'.
	OutputHashInOutputHashesSiblings []hexutil.Bytes `json:"outputHashInOutputHashesSiblings"`
	// Proof that this output-hashes root hash is in epoch's output merkle tree.
	// This array of siblings is bottom-up ordered (from the leaf to the root).
	// Each hash is given in Ethereum hex binary format (32 bytes), starting with '0x'.
	OutputHashesInEpochSiblings []hexutil.Bytes `json:"outputHashesInEpochSiblings"`
	// Data that allows the validity proof to be contextualized within submitted claims,
	// given as a payload in Ethereum hex binary format, starting with '0x'
	Context hexutil.Bytes `json:"context"`
}

func newProof(
	inputIndexWithinEpoch int,
	outputIndexWithinInput int,
	outputHashesRootHash string,
	vouchersEpochRootHash string,
	noticesEpochRootHash string,
	machineStateHash string,
	outputHashInOutputHashesSiblings []string,
	outputHashesInEpochSiblings []string,
	context string,
) (*Proof, error) {

	var (
		outputHashOutputSiblings []hexutil.Bytes
		outputHashEpochSiblings  []hexutil.Bytes
	)

	// This tests if there's a proof, else it returns nil
	if len(outputHashesRootHash) == 0 {
		return nil, nil
	}

	outputHash, err := hexutil.Decode(outputHashesRootHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode OutputHashesRootHash to bytes: %v", err)
	}

	vouchersHash, err := hexutil.Decode(vouchersEpochRootHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode VouchersEpochRootHash to bytes: %v", err)
	}

	noticesHash, err := hexutil.Decode(noticesEpochRootHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode NoticesEpochRootHash to bytes: %v", err)
	}

	machineHash, err := hexutil.Decode(machineStateHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode MachineStateHash to bytes: %v", err)
	}

	for _, hash := range outputHashInOutputHashesSiblings {
		tempHash, err := hexutil.Decode(hash)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to decode outputHashInOutputHashesSiblings to bytes: %v",
				err,
			)
		}

		outputHashOutputSiblings = append(outputHashOutputSiblings, tempHash)
	}

	for _, hash := range outputHashesInEpochSiblings {
		tempHash, err := hexutil.Decode(hash)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to decode outputHashesInEpochSiblings to bytes: %v",
				err,
			)
		}

		outputHashEpochSiblings = append(outputHashEpochSiblings, tempHash)
	}

	contextBytes, err := hexutil.Decode(context)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Context to bytes: %v", err)
	}

	proof := Proof{
		inputIndexWithinEpoch,
		outputIndexWithinInput,
		outputHash,
		vouchersHash,
		noticesHash,
		machineHash,
		outputHashOutputSiblings,
		outputHashEpochSiblings,
		contextBytes,
	}

	return &proof, err
}

func ConvertToContractProof(proof *Proof) *application.OutputValidityProof {
	return &application.OutputValidityProof{
		// implement this once we have the new GraphQL schema
	}
}

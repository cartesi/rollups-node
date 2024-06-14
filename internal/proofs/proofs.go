// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package proofs

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/node/model"
)

const INPUT_TREE_HEIGHT = 16
const EPOCH_TREE_HEIGHT = 32

// Generate will create the proofs for all Outputs within an Epoch
func Generate(
	ctx context.Context,
	inputRange InputRange,
	machineStateHash Hash,
	epochOutputs []Output, //FIXME: usar map[input_index][[]output]?
) ([]Proof, error) {
	/*
		TODO: outputs pode ser MUITO grande.
			- Estudar usar ponteiros nesse caso
			- Considerar uma estratégia incremental

		*assume que []outputs está ordenado por InputIndex*

		cria um array de Proof
		cria uma árvore de altura 16 cujas folhas são os outputs de um input
		cria uma árvore de altura 32 cujas folhas são os root hashes da árvore dos inputs
		currentInput := input_index do primeiro output
		Para cada output em []output
			se output.InputIndex != currentInput
				outputHashesRootHash, err := generateOutputHashesRootHash(
					outputs[currentInput:output.InputIndex-1],
					&proofs,
				)
				arvore32.Push(outputHashesRootHash)
				currentInput := output.InputIndex
		Fim
		outputsEpochRootHash := arvore32.RootHash()
		for idx, _ := range arvore32.Leaves() {

		}
	*/

	if len(epochOutputs) == 0 {
		return nil, errors.New("proofs: no outputs")
	}

	proofs := make([]Proof, 0, len(epochOutputs))
	// creates the Merkle tree which leaves are the keccak256 hash of the outputs
	inputTree, err := merkle.NewTree(INPUT_TREE_HEIGHT)
	if err != nil {
		return nil, err //FIXME: panic?
	}
	// creates the Merkle tree which leaves are the root hashes of the input trees
	epochTree, err := merkle.NewTree(EPOCH_TREE_HEIGHT)
	if err != nil {
		return nil, err //FIXME: panic?
	}

	firstOutputIndexWithinInput := 0
	currentInput := epochOutputs[0].InputIndex
	inputTree.PushData(epochOutputs[0].Blob)
	// for each output after the first
	for currentOutputIdx, currentOutput := range epochOutputs[1:] {
		// if it is an output from another input
		if currentOutput.InputIndex != currentInput {
			// iterate over all the outputs from the current input...
			for _, output := range epochOutputs[firstOutputIndexWithinInput:currentOutputIdx] {
				siblings, err := inputTree.SiblingsOfLeaf(output.Index)
				if err != nil {
					return nil, err //FIXME: panic?
				}
				inputIndexWithinEpoch, err := inputIndexWithinEpoch(currentInput, inputRange)
				if err != nil {
					return nil, err //FIXME: panic?
				}
				// ...and start creating its proof
				proof := Proof{
					InputRange:                       inputRange,
					InputIndexWithinEpoch:            inputIndexWithinEpoch,
					OutputIndexWithinInput:           output.Index,
					OutputHashesRootHash:             inputTree.RootHash(),
					MachineStateHash:                 machineStateHash,
					OutputHashInOutputHashesSiblings: siblings,
				}
				proofs = append(proofs, proof)
			}
			// add the input tree root hash to the epoch tree...
			epochTree.Push(inputTree.RootHash())
			// ...and clear the input tree for the next input
			inputTree.Clear()
			currentInput = currentOutput.InputIndex
			firstOutputIndexWithinInput = currentOutputIdx

		} else { // otherwise, just add it to the input tree
			inputTree.PushData(currentOutput.Blob)
		}
	}

	// now that epoch tree has all its leaves, lets calculate its root hash
	// and finish the proofs
	for _, proof := range proofs {
		proof.OutputsEpochRootHash = epochTree.RootHash()
		epochSiblings, err := epochTree.SiblingsOfLeaf(proof.InputIndexWithinEpoch)
		if err != nil {
			return nil, err //FIXME: panic?
		}
		proof.OutputHashesInEpochSiblings = epochSiblings
	}

	return proofs, nil
}

// FIXME: should this function be here?
// TODO: verity logic
// inputIndexWithinEpoch returns the index of an input in an epoch,
// given its global index and the index of the epoch's first input
func inputIndexWithinEpoch(inputIndex uint64, epochInputRange InputRange) (uint64, error) {
	if inputIndex < epochInputRange.First || inputIndex > epochInputRange.Last {
		return 0, fmt.Errorf("input index is not in the same epoch")
	}
	return inputIndex - epochInputRange.First, nil
}

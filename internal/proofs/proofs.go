// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package proofs

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/node/model"
)

// Generate will create the proofs for all Outputs within an Epoch
func Generate(
	ctx context.Context,
	inputRange InputRange,
	machineStateHash Hash,
	outputs []Output,
) ([]Proof, error) {
	/*
		TODO: outputs pode ser MUITO grande.
			- Estudar usar ponteiros nesse caso
			- Considerar uma estratégia incremental

		Se len([]Output) é maior do que o tamanho máximo
			retorna erro

		*assume que []outputs está ordenado por InputIndex*
		cria um array de Proof
		cria uma árvore cujas folhas são o root hash da árvore do input
		salva o índice original := 0
		Para cada output em []outputs
			se output no índice atual é diferente do anterior
				cria uma árvore pro input cujas folhas são outputs[original] até outputs[anterior]

				outputHashesRootHash = raiz da árvore do input

				outputMerkleProof = usa a árvore do input para pegar a Merkle Proof do output
				outputHashInOutputHashesSiblings = usa a árvore do input para obter os siblings da outputMerkleProof

				inputIndexWithinEpoch = inputIndexWithinEpoch(output[atual].InputIndex, InputRange)

				outputIndexWithinInput = output[atual].Index

				// Fica faltando o OutputsEpochRootHash e o OutputHashesInEpochSiblings
				cria um Proof { inputRange, outputIndexWithinInput, output[atual].Index, machineStateHash}
				adiciona o Proof no array de Proof

				adiciona output
				original = atual
			senão
				anterior = atual
				atual++
		percorre []outputs até que output.InputIndex mude
		cria uma árvore do índice anterior até o final

		fazer o keccak256 do blob de cada output, essas são as folhas da árvore
	*/

	proofs := make([]Proof, 0, len(outputs))
	for range outputs {
		// create the Merkle Tree whose leaves are the root hashes of input trees
		outputsEpochTree, err := merkle.NewTree(32)
		if err != nil {
			return nil, err //FIXME: panic? should never happen
		}
		first := 0
		// create the Merkle Tree whose leaves are the hashes of outputs
		// from the same input
		inputTree, err := merkle.NewTree(16)
		if err != nil {
			return nil, err //FIXME: panic? should never happen
		}
		for i := 1; i < len(outputs); i++ {
			// start gathering the leaves of the input tree
			inputTree.PushData(outputs[first].Blob)
			// if the current output is from the same input than the previous one
			if outputs[i].InputIndex == outputs[i-1].InputIndex {
				// include its hash as a leaf
				inputTree.PushData(outputs[i].Blob)
				continue
			}
			// otherwise, create a Merkle Tree using the output hashes of a input as leaves
			outputHashesRootHash := inputTree.RootHash()

			for _, output := range outputs[first:i] {
				proof := Proof{
					InputRange:                       inputRange,
					InputIndexWithinEpoch:            0, //TODO:
					OutputIndexWithinInput:           output.Index,
					OutputHashesRootHash:             outputHashesRootHash,
					MachineStateHash:                 machineStateHash,
					OutputHashInOutputHashesSiblings: []Hash{}, //TODO:
				}
				proofs = append(proofs, proof)
			}

			// reset the loop for the next input
			first = i
		}

		// create the epoch tree
		// get its root hash and siblings
		// add them to the proofs

		return proofs, nil
	}
}

// FIXME: should this function be here?
// inputIndexWithinEpoch returns the index of an input in an epoch,
// given its global index and the index of the epoch's first input
func inputIndexWithinEpoch(inputIndex uint64, epochInputRange InputRange) (uint64, error) {
	if !epochInputRange.Contains(inputIndex) {
		return 0, fmt.Errorf("input index is not in the same epoch")
	}
	return inputIndex - epochInputRange.First, nil
}

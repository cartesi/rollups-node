// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

// WriteProofs adapts stored test outputs to bounded claim proof batches.
func WriteProofs(outputs ...*model.Output) repository.ClaimProofWriteFunc {
	return func(ctx context.Context, writer repository.ClaimProofWriter) error {
		for begin := 0; begin < len(outputs); {
			end := min(begin+int(repository.MaxOutputRangeRows), len(outputs))
			proofs := make([]repository.OutputProof, end-begin)
			for i, output := range outputs[begin:end] {
				proofs[i] = repository.OutputProof{Index: output.Index, Hash: *output.Hash, Siblings: output.OutputHashesSiblings}
			}
			if err := writer.StoreOutputProofs(ctx, proofs); err != nil {
				return err
			}
			begin = end
		}
		return nil
	}
}

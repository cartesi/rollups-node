// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

const outputsChunkHeight = 11

// computeMerkleTreeAndProofs reads the epoch's outputs once and returns the
// outputs root. The returned writer reads them again and stores their proofs.
// An epoch without outputs keeps the previous epoch's root and needs no writer.
func (s *Service) computeMerkleTreeAndProofs(
	ctx context.Context, app *model.Application, epoch *model.Epoch,
) (root *common.Hash, write repository.ClaimProofWriteFunc, err error) {
	start := time.Now()
	appAddress := app.IApplicationAddress.String()
	// Epoch identity remains unambiguous when inputs straddle a block boundary.
	span, err := s.repository.GetEpochOutputRange(ctx, app.ID, epoch.Index)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get outputs for epoch %v (%v) of application %v. %w",
			epoch.Index, epoch.VirtualIndex, appAddress, err)
	}
	defer func() {
		if err == nil && root != nil {
			s.Logger.Info("OutputsMerkleRoot Computed", "application", appAddress, "epoch_index", epoch.Index,
				"outputs_merkle_root", *root, "output_count", span.Count, "pass1_elapsed", time.Since(start))
		}
	}()
	var previousEpoch *model.Epoch
	if epoch.VirtualIndex > 0 {
		previousEpoch, err = s.repository.GetEpochByVirtualIndex(ctx, appAddress, epoch.VirtualIndex-1)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get previous epoch for epoch %v (%v) of application %v. %w",
				epoch.Index, epoch.VirtualIndex, appAddress, err)
		}
	}
	if span.Count == 0 {
		if epoch.VirtualIndex != 0 && previousEpoch == nil {
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"epoch %v (%v): previous epoch with virtual index %v is missing",
				epoch.Index, epoch.VirtualIndex, epoch.VirtualIndex-1)
		}
		if epoch.VirtualIndex == 0 {
			return &s.pristineRootHash, nil, nil
		}
		if previousEpoch.TxBufferDataBlock == nil {
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"invalid application state for epoch %v (%v) of application %v. Previous epoch has no claim.",
				epoch.Index, epoch.VirtualIndex, appAddress)
		}
		return previousEpoch.TxBufferDataBlock, nil, nil
	}
	if span.Last < span.First || span.Last-span.First != span.Count-1 {
		return nil, nil, s.setApplicationCorrupted(ctx, app,
			"outputs of epoch %v are not contiguous: %v outputs from index %v to %v",
			epoch.Index, span.Count, span.First, span.Last)
	}
	pre := s.pristinePostContext
	if span.First != 0 {
		previousOutput, err := s.repository.GetOutput(ctx, appAddress, span.First-1)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get previous output for epoch %v (%v) of application %v. %w",
				epoch.Index, epoch.VirtualIndex, appAddress, err)
		}
		if previousOutput == nil || previousOutput.Hash == nil || len(previousOutput.OutputHashesSiblings) != merkle.TREE_DEPTH {
			return nil, nil, s.setApplicationCorrupted(ctx, app,
				"Inconsistent application state (%v). Output (%d) preceding epoch %d is missing or has invalid hash siblings.",
				app.Name, span.First-1, epoch.Index)
		}
		pre = merkle.CreatePreContextFromProof(previousOutput.Index, *previousOutput.Hash, previousOutput.OutputHashesSiblings)
	}
	builder, err := merkle.NewOutputsTreeBuilder(pre, s.pristinePostContext, span.First, span.Count, outputsChunkHeight)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid outputs builder: %w: %v", merkle.ErrInvariant, err)
	}
	scan := func(ctx context.Context, first, last uint64, fn repository.OutputScanFunc) error {
		return s.repository.ScanOutputs(ctx, app.ID, first, last, fn)
	}
	for first, last := range builder.ChunkRanges() {
		if err := scanOutputChunk(ctx, first, last, scan, func(hash common.Hash) error {
			builder.Append(hash)
			return nil
		}); err != nil {
			return nil, nil, err
		}
	}
	tree := builder.Build()
	hash := tree.Root()
	return &hash, func(ctx context.Context, writer repository.ClaimProofWriter) error {
		return writeOutputProofs(ctx, writer, tree)
	}, nil
}

type outputChunkScan func(context.Context, uint64, uint64, repository.OutputScanFunc) error

// scanOutputChunk hashes each borrowed row before the scan advances.
func scanOutputChunk(
	ctx context.Context, first, last uint64, scan outputChunkScan, appendHash func(common.Hash) error,
) error {
	next := first
	if err := scan(ctx, first, last, func(index uint64, rawData []byte) error {
		if index != next || index > last {
			return fmt.Errorf("expected output %d in range %d..%d, got %d: %w",
				next, first, last, index, merkle.ErrOutputsChanged)
		}
		if err := appendHash(crypto.Keccak256Hash(rawData)); err != nil {
			return err
		}
		next++
		return nil
	}); err != nil {
		return fmt.Errorf("scan outputs %d..%d: %w", first, last, err)
	}
	if next != last+1 {
		return fmt.Errorf("outputs %d..%d ended at %d: %w", first, last, next, merkle.ErrOutputsChanged)
	}
	return nil
}

// writeOutputProofs stores one batch of proofs for each chunk of the tree.
func writeOutputProofs(ctx context.Context, writer repository.ClaimProofWriter, tree *merkle.OutputsTree) error {
	prover := tree.NewProver()
	for first, last := range tree.ChunkRanges() {
		var chunk *merkle.ChunkProofs // Each chunk range completes one proof chunk.
		err := scanOutputChunk(ctx, first, last, writer.ScanOutputs, func(hash common.Hash) error {
			proofs, err := prover.Append(hash)
			if proofs != nil {
				chunk = proofs
			}
			return err
		})
		if err != nil {
			return err
		}
		if chunk == nil {
			return fmt.Errorf("missing proof chunk: %w", merkle.ErrInvariant)
		}
		proofs := make([]repository.OutputProof, len(chunk.Leaves))
		for index, hash := range chunk.Leaves {
			proofs[index] = repository.OutputProof{Index: first + uint64(index), Hash: hash,
				Siblings: chunk.Siblings[index*merkle.TREE_DEPTH : (index+1)*merkle.TREE_DEPTH]}
		}
		// Rows are closed before this call. pgx cannot write during ScanOutputs.
		if err := writer.StoreOutputProofs(ctx, proofs); err != nil {
			return err
		}
	}
	return nil
}

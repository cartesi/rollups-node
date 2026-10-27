// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func (s *BulkOperationsSuite) TestClaimProofWriterTransaction() {
	const (
		noWriter                  = "nil"
		scanOutputs               = "scan"
		errorAfterBatches         = "two batches then error"
		outputCount        uint64 = 2
		missingOutputIndex uint64 = 999
	)
	for _, outcome := range []string{noWriter, scanOutputs, errorAfterBatches, "missing output", "over cap"} {
		s.Run(outcome, func() {
			seed := Seed(s.Ctx, s.T(), s.Repo)
			raw := [][]byte{[]byte("first"), []byte("second")}
			StoreAdvanceResult(s.Ctx, s.T(), s.Repo, seed.App.ID, seed.Epoch.Index, seed.Input.Index,
				model.InputCompletionStatus_Accepted, raw, nil)
			AdvanceEpochStatus(s.Ctx, s.T(), s.Repo, seed.App.Name, seed.Epoch, model.EpochStatus_InputsProcessed)
			hash := UniqueHash()
			proofs := []repository.OutputProof{{Index: 0, Hash: hash, Siblings: []common.Hash{UniqueHash()}},
				{Index: 1, Hash: hash, Siblings: []common.Hash{UniqueHash()}}}
			sentinel := errors.New("failure after both batches")
			called := false
			var write repository.ClaimProofWriteFunc
			if outcome != noWriter {
				write = func(ctx context.Context, writer repository.ClaimProofWriter) error {
					called = true
					switch outcome {
					case scanOutputs:
						rows := uint64(0)
						err := writer.ScanOutputs(ctx, 0, 1, func(index uint64, data []byte) error {
							s.Require().Less(rows, outputCount)
							s.Equal(rows, index)
							s.Equal(raw[rows], data)
							rows++
							return nil
						})
						s.Require().NoError(err)
						s.Equal(outputCount, rows)
						return writer.StoreOutputProofs(ctx, proofs)
					case errorAfterBatches:
						for _, proof := range proofs {
							if err := writer.StoreOutputProofs(ctx, []repository.OutputProof{proof}); err != nil {
								return err
							}
						}
						return sentinel
					case "missing output":
						if err := writer.StoreOutputProofs(ctx, proofs[:1]); err != nil {
							return err
						}
						return writer.StoreOutputProofs(ctx, []repository.OutputProof{{Index: missingOutputIndex, Hash: hash}})
					default:
						return writer.StoreOutputProofs(ctx, make([]repository.OutputProof, repository.MaxOutputRangeRows+1))
					}
				}
			}
			err := s.Repo.StoreClaimAndProofs(s.Ctx, seed.Epoch, write)
			success := outcome == noWriter || outcome == scanOutputs
			if success {
				s.Require().NoError(err)
			} else {
				s.Require().Error(err)
			}
			if outcome == errorAfterBatches {
				s.ErrorIs(err, sentinel)
			}
			s.Equal(outcome != noWriter, called)
			stored, err := s.Repo.GetEpoch(s.Ctx, seed.App.Name, seed.Epoch.Index)
			s.Require().NoError(err)
			status := model.EpochStatus_InputsProcessed
			if success {
				status = model.EpochStatus_ClaimComputed
			}
			s.Equal(status, stored.Status)
			for _, proof := range proofs {
				output, err := s.Repo.GetOutput(s.Ctx, seed.App.Name, proof.Index)
				s.Require().NoError(err)
				if outcome == scanOutputs {
					s.Equal(&proof.Hash, output.Hash)
					s.Equal(proof.Siblings, output.OutputHashesSiblings)
				} else {
					s.Nil(output.Hash)
					s.Empty(output.OutputHashesSiblings)
				}
			}
		})
	}
}

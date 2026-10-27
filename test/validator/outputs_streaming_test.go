// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"encoding/binary"
	"log/slog"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type outputsStatusWrites struct {
	repository.Repository
	writes int
}

func (repo *outputsStatusWrites) UpdateApplicationStatus(
	ctx context.Context, appID int64, status model.ApplicationStatus, reason *string,
) error {
	repo.writes++
	return repo.Repository.UpdateApplicationStatus(ctx, appID, status, reason)
}

func (repo *outputsStatusWrites) StoreInvalidOutputsRoot(ctx context.Context, appID int64, epoch uint64, reason string) error {
	repo.writes++
	return repo.Repository.StoreInvalidOutputsRoot(ctx, appID, epoch, reason)
}

func (s *ValidatorRepositoryIntegrationSuite) TestStreamingOutputsAgainstWholeHistoryProofs() {
	s.Run("physical epochs differ from virtual epochs", func() {
		const page = 2048
		counts := []int{page + 7, 2*page + 9}
		app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_Authority).Create(s.ctx, s.T(), s.repository)
		epochs := []*model.Epoch{
			repotest.NewEpochBuilder(app.ID).WithIndex(0).WithStatus(model.EpochStatus_Closed).WithBlocks(0, 9).Build(),
			repotest.NewEpochBuilder(app.ID).WithIndex(2).WithStatus(model.EpochStatus_Closed).WithBlocks(20, 29).Build(),
		}
		inputs := []*model.Input{
			repotest.NewInputBuilder().WithIndex(0).WithBlockNumber(9).Build(),
			repotest.NewInputBuilder().WithIndex(1).WithEpochIndex(2).WithBlockNumber(29).Build(),
		}
		s.Require().NoError(s.repository.CreateEpochsAndInputs(s.ctx, app.Name,
			map[*model.Epoch][]*model.Input{epochs[0]: {inputs[0]}, epochs[1]: {inputs[1]}}, 30))
		raw := make([][]byte, counts[0]+counts[1])
		leaves := make([]common.Hash, len(raw))
		for i := range raw {
			raw[i] = make([]byte, 132)
			binary.BigEndian.PutUint64(raw[i], uint64(i))
			leaves[i] = crypto.Keccak256Hash(raw[i])
		}
		s.Require().Len(raw, 6160)
		expected := make([][]common.Hash, len(epochs))
		roots := make([]common.Hash, len(epochs))
		offset := 0
		for i, epoch := range epochs {
			end := offset + counts[i]
			root, proofs, err := merkletest.CreateProofs(leaves[:end], merkle.TREE_DEPTH)
			s.Require().NoError(err)
			roots[i], expected[i] = root, proofs
			result := &model.AdvanceResult{EpochIndex: epoch.Index, InputIndex: inputs[i].Index,
				Status: model.InputCompletionStatus_Accepted, Outputs: raw[offset:end],
				StateProof: completeStateProof(crypto.Keccak256Hash([]byte{byte(i), 42}), root)}
			s.Require().NoError(s.repository.StoreAdvanceResult(s.ctx, app.ID, result))
			s.Require().NoError(s.repository.UpdateEpochInputsProcessed(s.ctx, app.Name, epoch.Index, &result.StateProof))
			offset = end
		}
		tracked := &outputsStatusWrites{Repository: s.repository}
		service, err := validator.Create(s.ctx, &validator.CreateInfo{
			Config: config.ValidatorConfig{LogLevel: slog.LevelDebug}, Repository: tracked})
		s.Require().NoError(err)
		_, err = service.(*validator.Service).Tick(s.ctx)
		s.Require().NoError(err)
		s.Equal(0, tracked.writes)
		offset = 0
		for i, epoch := range epochs {
			stored, err := s.repository.GetEpoch(s.ctx, app.Name, epoch.Index)
			s.Require().NoError(err)
			s.Equal(uint64(i), stored.VirtualIndex)
			s.Equal(model.EpochStatus_ClaimComputed, stored.Status)
			s.Equal(&roots[i], stored.TxBufferDataBlock)
			outputs, total, err := s.repository.ListOutputs(s.ctx, app.Name, repository.OutputFilter{EpochIndex: &epoch.Index},
				repository.Pagination{}, false)
			s.Require().NoError(err)
			s.EqualValues(counts[i], total)
			s.Require().Len(outputs, counts[i])
			for local, output := range outputs {
				index := offset + local
				s.Equal(uint64(index), output.Index)
				s.Equal(&leaves[index], output.Hash)
				s.Equal(expected[i][index*merkle.TREE_DEPTH:(index+1)*merkle.TREE_DEPTH], output.OutputHashesSiblings)
			}
			offset += counts[i]
		}
		storedApp, err := s.repository.GetApplication(s.ctx, app.Name)
		s.Require().NoError(err)
		s.Equal(model.ApplicationStatus_OK, storedApp.Status)
	})
}

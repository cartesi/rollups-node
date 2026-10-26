// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"math"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/crypto"
)

type StateHashSuite struct {
	BaseSuite
}

func NewStateHashSuite(factory RepositoryFactory) *StateHashSuite {
	return &StateHashSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *StateHashSuite) TestListStateHashes() {
	s.Run("EmptyResult", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		hashes, total, err := s.Repo.ListStateHashes(
			s.Ctx, app.IApplicationAddress.String(),
			repository.StateHashFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(hashes)
		s.Equal(uint64(0), total)
	})

	s.Run("FilterByEpochIndex", func() {
		// StateHashes are created by StoreAdvanceResult, tested in BulkOperationsSuite.
		// This test verifies the filter works even with no data.
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epochIdx := uint64(0)
		hashes, total, err := s.Repo.ListStateHashes(
			s.Ctx, app.IApplicationAddress.String(),
			repository.StateHashFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(hashes)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsStateHashesFromDaveConsensus", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		machineHash := crypto.Keccak256Hash([]byte("dave-list-machine"))
		txBufferDataBlock := crypto.Keccak256Hash([]byte("dave-list-outputs"))
		proof := DummyStateProof()
		proof.MachineHash = machineHash
		proof.TxBufferDataBlock = txBufferDataBlock

		hash1 := [32]byte(crypto.Keccak256Hash([]byte("list-state-1")))
		hash2 := [32]byte(crypto.Keccak256Hash([]byte("list-state-2")))
		collectedHashes := [][32]byte{hash1, hash2}

		result := &AdvanceResult{
			EpochIndex:          0,
			InputIndex:          0,
			Status:              InputCompletionStatus_Accepted,
			PeriodicStateHashes: collectedHashes,
			PaddingRepetitions:  InputHashCollectionCapacity - uint64(len(collectedHashes)),
			IsDaveConsensus:     true,
			StateProof:          *proof,
		}

		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		// List all state hashes
		expectedRows := len(collectedHashes) + 1
		hashes, total, err := s.Repo.ListStateHashes(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.StateHashFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(hashes, expectedRows)
		s.Equal(uint64(expectedRows), total)

		// List with epoch filter
		epochIdx := uint64(0)
		hashes, total, err = s.Repo.ListStateHashes(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.StateHashFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(hashes, expectedRows)
		s.Equal(uint64(expectedRows), total)

		// Verify pagination
		pageSize := uint64(len(collectedHashes))
		hashes, total, err = s.Repo.ListStateHashes(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.StateHashFilter{},
			repository.Pagination{Limit: pageSize, Offset: 0}, false)
		s.Require().NoError(err)
		s.Len(hashes, len(collectedHashes))
		s.Equal(uint64(expectedRows), total)
	})
}

func (s *StateHashSuite) seedEpochStateHashes(app *Application, epochIndex, firstInput uint64) []EpochStateHash {
	s.T().Helper()
	const (
		inputCount     = 2
		blocksPerEpoch = 10
		expectedRows   = 5
	)
	epoch := NewEpochBuilder(app.ID).WithIndex(epochIndex).WithStatus(EpochStatus_Closed).
		WithBlocks(epochIndex*blocksPerEpoch, (epochIndex+1)*blocksPerEpoch-1).
		WithInputBounds(firstInput, firstInput+inputCount).Build()
	firstInputBlock := epoch.FirstBlock + 1
	inputs := []*Input{
		NewInputBuilder().WithIndex(firstInput).WithBlockNumber(firstInputBlock).Build(),
		NewInputBuilder().WithIndex(firstInput + 1).WithBlockNumber(firstInputBlock + 1).Build(),
	}
	s.Require().NoError(s.Repo.CreateEpochsAndInputs(s.Ctx, app.Name, map[*Epoch][]*Input{epoch: inputs}, epoch.LastBlock+1))
	expected := make([]EpochStateHash, 0, expectedRows)
	for offset, input := range inputs {
		entries := [][32]byte{UniqueHash()}
		if offset == 0 {
			entries = append(entries, UniqueHash())
		}
		proof := DummyStateProof()
		s.Require().NoError(s.Repo.StoreAdvanceResult(s.Ctx, app.ID, &AdvanceResult{
			EpochIndex: epoch.Index, InputIndex: input.Index, Status: InputCompletionStatus_Accepted,
			StateProof: *proof, PeriodicStateHashes: entries,
			PaddingRepetitions: InputHashCollectionCapacity - uint64(len(entries)), IsDaveConsensus: true,
		}))
		for _, entry := range entries {
			expected = append(expected, EpochStateHash{
				Index: uint64(len(expected)), InputIndex: input.Index, MachineHash: entry, Repetitions: 1,
			})
		}
		expected = append(expected, EpochStateHash{
			Index: uint64(len(expected)), InputIndex: input.Index,
			MachineHash: FinalStateEntry(proof.MachineHash), Repetitions: InputHashCollectionCapacity - uint64(len(entries)),
		})
	}
	return expected
}

func (s *StateHashSuite) TestEpochStateHashPages() {
	const (
		pageSize              = 2
		secondEpochFirstInput = 2
	)
	s.Run("EmptyEpoch", func() {
		app := NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).WithStatus(EpochStatus_Closed).Build()
		s.Require().NoError(s.Repo.CreateEpochsAndInputs(s.Ctx, app.Name, map[*Epoch][]*Input{epoch: nil}, epoch.LastBlock+1))
		count, err := s.Repo.CountEpochStateHashes(s.Ctx, app.ID, epoch.Index)
		s.Require().NoError(err)
		s.Zero(count)
		rows, err := s.Repo.ListEpochStateHashes(s.Ctx, app.ID, epoch.Index, nil, pageSize)
		s.Require().NoError(err)
		s.Empty(rows)
	})

	s.Run("IsolationAndCursorBoundaries", func() {
		app := NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(s.Ctx, s.T(), s.Repo)
		expected := s.seedEpochStateHashes(app, 0, 0)
		secondEpoch := s.seedEpochStateHashes(app, 1, secondEpochFirstInput)
		otherApp := NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(s.Ctx, s.T(), s.Repo)
		otherRows := s.seedEpochStateHashes(otherApp, 0, 0)
		s.Equal(uint64(0), secondEpoch[0].Index)
		s.Equal(uint64(0), otherRows[0].Index)
		count, err := s.Repo.CountEpochStateHashes(s.Ctx, app.ID, 0)
		s.Require().NoError(err)
		s.Equal(uint64(len(expected)), count)

		var paged []EpochStateHash
		var cursor *uint64
		for {
			page, err := s.Repo.ListEpochStateHashes(s.Ctx, app.ID, 0, cursor, pageSize)
			s.Require().NoError(err)
			if len(page) == 0 {
				break
			}
			s.LessOrEqual(len(page), pageSize)
			paged = append(paged, page...)
			s.Require().LessOrEqual(len(paged), len(expected))
			last := page[len(page)-1].Index
			cursor = &last
		}
		s.Equal(expected, paged)
		s.Equal(count, uint64(len(paged)))

		epochIndex := uint64(0)
		legacy, legacyCount, err := s.Repo.ListStateHashes(s.Ctx, app.Name,
			repository.StateHashFilter{EpochIndex: &epochIndex}, repository.Pagination{}, false)
		s.Require().NoError(err)
		s.Equal(count, legacyCount)
		for index, row := range legacy {
			s.Equal(EpochStateHash{
				Index: row.Index, InputIndex: row.InputIndex, MachineHash: row.MachineHash, Repetitions: row.Repetitions,
			}, paged[index])
		}

		zero := uint64(0)
		afterZero, err := s.Repo.ListEpochStateHashes(s.Ctx, app.ID, 0, &zero, pageSize)
		s.Require().NoError(err)
		s.Equal(expected[1:3], afterZero)
		full, err := s.Repo.ListEpochStateHashes(s.Ctx, app.ID, 0, nil, uint64(len(expected)))
		s.Require().NoError(err)
		s.Equal(expected, full)
		for _, after := range []uint64{expected[len(expected)-1].Index, math.MaxUint64} {
			page, err := s.Repo.ListEpochStateHashes(s.Ctx, app.ID, 0, &after, pageSize)
			s.Require().NoError(err)
			s.Empty(page)
		}
		full, err = s.Repo.ListEpochStateHashes(s.Ctx, app.ID, 0, nil, repository.MaxStateHashPageSize)
		s.Require().NoError(err)
		s.Equal(expected, full)
		for _, limit := range []uint64{0, repository.MaxStateHashPageSize + 1, math.MaxUint64} {
			_, err := s.Repo.ListEpochStateHashes(s.Ctx, app.ID, 0, nil, limit)
			s.Require().ErrorContains(err, "state hash page limit must be between 1 and 100000")
		}
	})

	s.Run("ApplicationDeleteCascades", func() {
		app := NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(s.Ctx, s.T(), s.Repo)
		expected := s.seedEpochStateHashes(app, 0, 0)
		count, err := s.Repo.CountEpochStateHashes(s.Ctx, app.ID, 0)
		s.Require().NoError(err)
		s.Equal(uint64(len(expected)), count)
		s.Require().NoError(s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false))
		s.Require().NoError(s.Repo.DeleteApplication(s.Ctx, app.ID))
		count, err = s.Repo.CountEpochStateHashes(s.Ctx, app.ID, 0)
		s.Require().NoError(err)
		s.Zero(count)
		page, err := s.Repo.ListEpochStateHashes(s.Ctx, app.ID, 0, nil, pageSize)
		s.Require().NoError(err)
		s.Empty(page)
	})
}

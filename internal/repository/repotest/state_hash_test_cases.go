// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
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

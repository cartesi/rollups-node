// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"errors"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

type EpochSuite struct {
	BaseSuite
}

func NewEpochSuite(factory RepositoryFactory) *EpochSuite {
	return &EpochSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *EpochSuite) TestCreateEpochsAndInputs() {
	s.Run("SingleEpochSingleInput", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).WithIndex(0).WithStatus(EpochStatus_Closed).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(uint64(0), got.Index)
		s.Equal(EpochStatus_Closed, got.Status)
	})

	s.Run("MultipleEpochsMultipleInputs", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).WithBlocks(0, 9).Build()
		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Open).WithBlocks(10, 19).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).WithBlockNumber(15).Build()

		epochInputMap := map[*Epoch][]*Input{
			epoch0: {input0},
			epoch1: {input1},
		}
		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(), epochInputMap, 20)
		s.Require().NoError(err)

		got0, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_Closed, got0.Status)

		got1, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 1)
		s.Require().NoError(err)
		s.Equal(EpochStatus_Open, got1.Status)
	})

	s.Run("EpochWithNoInputs", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).WithBlocks(0, 9).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {}}, 10)
		s.Require().NoError(err)

		// Epoch should exist
		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Require().NotNil(got)
		s.Equal(EpochStatus_Closed, got.Status)

		// No inputs should exist
		inputs, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(inputs)
		s.Equal(uint64(0), total)
	})

	s.Run("UpsertExistingEpoch", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Open).WithBlocks(0, 9).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		// Upsert the same epoch with updated status
		epoch2 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).WithBlocks(0, 9).Build()
		input2 := NewInputBuilder().WithIndex(1).WithBlockNumber(8).Build()

		err = s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch2: {input2}}, 10)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_Closed, got.Status)
	})
}

func (s *EpochSuite) TestGetEpoch() {
	s.Run("ExistingEpoch", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetEpoch(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(seed.App.ID, got.ApplicationID)
		s.Equal(uint64(0), got.Index)
		s.Equal(seed.Epoch.FirstBlock, got.FirstBlock)
		s.Equal(seed.Epoch.LastBlock, got.LastBlock)
		s.Equal(seed.Epoch.InputIndexLowerBound, got.InputIndexLowerBound)
		s.Equal(seed.Epoch.InputIndexUpperBound, got.InputIndexUpperBound)
		s.Equal(EpochStatus_Closed, got.Status)
		s.Equal(uint64(0), got.VirtualIndex)
		s.Nil(got.MachineHash)
		s.Nil(got.OutputsMerkleRoot)
		s.Nil(got.ClaimTransactionHash)
		s.Nil(got.Commitment)
		s.False(got.CreatedAt.IsZero(), "CreatedAt should be set")
		s.False(got.UpdatedAt.IsZero(), "UpdatedAt should be set")
	})

	s.Run("NotFound", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 99)
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *EpochSuite) TestGetEpochByVirtualIndex() {
	s.Run("ExistingVirtualIndex", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetEpochByVirtualIndex(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(uint64(0), got.VirtualIndex)
	})

	s.Run("NotFound", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetEpochByVirtualIndex(
			s.Ctx, app.IApplicationAddress.String(), 99)
		s.Require().NoError(err)
		s.Nil(got)
	})

	s.Run("DivergentVirtualAndPhysicalIndex", func() {
		// CreateEpochsAndInputs auto-assigns VirtualIndex as MAX(VirtualIndex)+1.
		// By creating an epoch with Index=5, VirtualIndex will be auto-assigned as 0.
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(5).
			WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		// Lookup by virtual index 0 (auto-assigned) should find the epoch with Index=5
		got, err := s.Repo.GetEpochByVirtualIndex(
			s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Require().NotNil(got)
		s.Equal(uint64(5), got.Index)
		s.Equal(uint64(0), got.VirtualIndex)

		// Lookup by virtual index 5 should NOT find it
		got, err = s.Repo.GetEpochByVirtualIndex(
			s.Ctx, app.IApplicationAddress.String(), 5)
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *EpochSuite) TestGetLastAcceptedEpochIndex() {
	s.Run("WithAcceptedEpoch", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_ClaimAccepted).WithBlocks(0, 9).Build()
		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Closed).WithBlocks(10, 19).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}}, 20)
		s.Require().NoError(err)

		idx, err := s.Repo.GetLastAcceptedEpochIndex(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), idx)
	})

	s.Run("ErrorWhenNoAcceptedEpochs", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		_, err = s.Repo.GetLastAcceptedEpochIndex(
			s.Ctx, app.IApplicationAddress.String())
		s.Require().Error(err)
		s.True(errors.Is(err, repository.ErrNotFound))
	})
}

func (s *EpochSuite) TestGetLastNonOpenEpoch() {
	s.Run("ReturnsClosedEpoch", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).WithBlocks(0, 9).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		got, err := s.Repo.GetLastNonOpenEpoch(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(EpochStatus_Closed, got.Status)
	})

	s.Run("NilWhenAllEpochsAreOpen", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Open).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		got, err := s.Repo.GetLastNonOpenEpoch(
			s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *EpochSuite) TestListEpochs() {
	s.Run("EmptyResult", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epochs, total, err := s.Repo.ListEpochs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.EpochFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(epochs)
		s.Equal(uint64(0), total)
	})

	s.Run("FilterByStatus", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).WithBlocks(0, 9).Build()
		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Open).WithBlocks(10, 19).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}}, 20)
		s.Require().NoError(err)

		epochs, total, err := s.Repo.ListEpochs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.EpochFilter{Status: []EpochStatus{EpochStatus_Closed}},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(epochs, 1)
		s.Equal(uint64(1), total)
		s.Equal(EpochStatus_Closed, epochs[0].Status)
	})

	s.Run("FilterByBeforeBlock", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Closed).
			WithBlocks(10, 19).WithInputBounds(1, 1).Build()
		epoch2 := NewEpochBuilder(app.ID).
			WithIndex(2).WithStatus(EpochStatus_Open).
			WithBlocks(20, 29).WithInputBounds(2, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).WithBlockNumber(15).Build()
		input2 := NewInputBuilder().WithIndex(2).WithEpochIndex(2).WithBlockNumber(25).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}, epoch2: {input2}}, 30)
		s.Require().NoError(err)

		// BeforeBlock=15 means LastBlock < 15, so epoch0 (LastBlock=9) matches
		beforeBlock := uint64(15)
		epochs, total, err := s.Repo.ListEpochs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.EpochFilter{BeforeBlock: &beforeBlock},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(epochs, 1)
		s.Equal(uint64(1), total)
		s.Equal(uint64(0), epochs[0].Index)
	})

	s.Run("Pagination", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epochInputMap := make(map[*Epoch][]*Input)
		for i := range uint64(5) {
			e := NewEpochBuilder(app.ID).
				WithIndex(i).WithStatus(EpochStatus_Closed).
				WithBlocks(i*10, i*10+9).WithInputBounds(i, i).Build()
			inp := NewInputBuilder().WithIndex(i).WithEpochIndex(i).WithBlockNumber(i*10 + 5).Build()
			epochInputMap[e] = []*Input{inp}
		}
		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(), epochInputMap, 50)
		s.Require().NoError(err)

		epochs, total, err := s.Repo.ListEpochs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.EpochFilter{},
			repository.Pagination{Limit: 2, Offset: 0}, false)
		s.Require().NoError(err)
		s.Len(epochs, 2)
		s.Equal(uint64(5), total)
	})

	s.Run("Descending", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epochInputMap := make(map[*Epoch][]*Input)
		for i := range uint64(3) {
			e := NewEpochBuilder(app.ID).
				WithIndex(i).WithStatus(EpochStatus_Closed).
				WithBlocks(i*10, i*10+9).WithInputBounds(i, i).Build()
			inp := NewInputBuilder().WithIndex(i).WithEpochIndex(i).
				WithBlockNumber(i*10 + 5).Build()
			epochInputMap[e] = []*Input{inp}
		}
		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(), epochInputMap, 30)
		s.Require().NoError(err)

		epochs, _, err := s.Repo.ListEpochs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.EpochFilter{},
			repository.Pagination{Limit: 10}, true)
		s.Require().NoError(err)
		s.Require().Len(epochs, 3)
		// Descending: highest index first
		s.Equal(uint64(2), epochs[0].Index)
		s.Equal(uint64(1), epochs[1].Index)
		s.Equal(uint64(0), epochs[2].Index)
	})

	s.Run("FilterByMultipleStatuses", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Open).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Closed).
			WithBlocks(10, 19).WithInputBounds(1, 1).Build()
		epoch2 := NewEpochBuilder(app.ID).
			WithIndex(2).WithStatus(EpochStatus_InputsProcessed).
			WithBlocks(20, 29).WithInputBounds(2, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).WithBlockNumber(15).Build()
		input2 := NewInputBuilder().WithIndex(2).WithEpochIndex(2).WithBlockNumber(25).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}, epoch2: {input2}}, 30)
		s.Require().NoError(err)

		// Filter for both Closed and InputsProcessed
		epochs, total, err := s.Repo.ListEpochs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.EpochFilter{
				Status: []EpochStatus{EpochStatus_Closed, EpochStatus_InputsProcessed},
			},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(epochs, 2)
		s.Equal(uint64(2), total)
		for _, e := range epochs {
			s.True(
				e.Status == EpochStatus_Closed || e.Status == EpochStatus_InputsProcessed,
				"unexpected status: %s", e.Status)
		}
	})
}

func (s *EpochSuite) TestUpdateEpochStatus() {
	s.Run("UpdatesStatus", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		epoch := seed.Epoch
		epoch.Status = EpochStatus_InputsProcessed

		err := s.Repo.UpdateEpochStatus(s.Ctx, seed.App.IApplicationAddress.String(), epoch)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_InputsProcessed, got.Status)
	})

	s.Run("NotFoundForNonExistentEpoch", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		nonExistentEpoch := &Epoch{Index: 99, Status: EpochStatus_InputsProcessed}

		err := s.Repo.UpdateEpochStatus(
			s.Ctx, app.IApplicationAddress.String(), nonExistentEpoch)
		s.Require().Error(err)
		s.Require().ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *EpochSuite) TestUpdateEpochInputsProcessed() {
	s.Run("MarksEpochProcessed", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		err := s.Repo.UpdateEpochInputsProcessed(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_InputsProcessed, got.Status)
	})

	// The update should be a no-op when the previous epoch is still Open
	// (i.e., not yet past Closed). The SQL condition requires that the
	// previous epoch status is NOT IN (Open, Closed).
	s.Run("NoOpWhenPreviousEpochStillOpen", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Open).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Closed).
			WithBlocks(10, 19).WithInputBounds(1, 1).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).
			WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}}, 20)
		s.Require().NoError(err)

		// Process input1 so the inputs-present condition is satisfied
		result := &AdvanceResult{
			EpochIndex: 1,
			InputIndex: 1,
			Status:     InputCompletionStatus_Accepted,
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}
		err = s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result)
		s.Require().NoError(err)

		// Try to mark epoch1 as InputsProcessed; previous epoch0 is Open
		err = s.Repo.UpdateEpochInputsProcessed(
			s.Ctx, app.IApplicationAddress.String(), 1)
		s.Require().NoError(err) // returns nil (no-op), not an error

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 1)
		s.Require().NoError(err)
		// Epoch1 should remain Closed -- not promoted to InputsProcessed
		s.Equal(EpochStatus_Closed, got.Status)
	})

	// The update should be a no-op when the epoch still has pending
	// (unprocessed) inputs. The SQL requires pending_count == 0.
	s.Run("NoOpWhenPendingInputsRemain", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// Create a single-epoch setup with 2 inputs
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(3).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(7).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1}}, 10)
		s.Require().NoError(err)

		// Process only 1 of the 2 inputs
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}
		err = s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result)
		s.Require().NoError(err)

		// Try to mark epoch as InputsProcessed; input1 is still pending
		err = s.Repo.UpdateEpochInputsProcessed(
			s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err) // returns nil (no-op)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		// Epoch should remain Closed since not all inputs are processed
		s.Equal(EpochStatus_Closed, got.Status)
	})

	// The update should be a no-op when not all expected inputs are present.
	// total_count != (upper_bound - lower_bound).
	s.Run("NoOpWhenInputCountDoesNotMatchBounds", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// Epoch expects 3 inputs (bounds 0..3) but we only provide 1.
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 3).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0}}, 10)
		s.Require().NoError(err)

		// Process the single input we do have
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}
		err = s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result)
		s.Require().NoError(err)

		// Try to mark epoch as InputsProcessed
		err = s.Repo.UpdateEpochInputsProcessed(
			s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err) // returns nil (no-op)

		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		// Epoch should remain Closed since not all expected inputs are present
		s.Equal(EpochStatus_Closed, got.Status)
	})

	// Non-existent epoch should be a silent no-op (returns nil).
	s.Run("NoOpForNonExistentEpoch", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		err := s.Repo.UpdateEpochInputsProcessed(
			s.Ctx, app.IApplicationAddress.String(), 99)
		s.Require().NoError(err)
	})
}

func (s *EpochSuite) TestUpdateEpochClaimTransactionHash() {
	s.Run("SetsTransactionHash", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		txHash := UniqueHash()
		seed.Epoch.ClaimTransactionHash = &txHash

		err := s.Repo.UpdateEpochClaimTransactionHash(
			s.Ctx, seed.App.IApplicationAddress.String(), seed.Epoch)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Require().NotNil(got.ClaimTransactionHash)
		s.Equal(txHash, *got.ClaimTransactionHash)
	})

	s.Run("NotFoundForNonExistentEpoch", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		txHash := UniqueHash()
		nonExistentEpoch := &Epoch{Index: 99, ClaimTransactionHash: &txHash}

		err := s.Repo.UpdateEpochClaimTransactionHash(
			s.Ctx, app.IApplicationAddress.String(), nonExistentEpoch)
		s.Require().Error(err)
		s.Require().ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *EpochSuite) TestUpdateEpochOutputsProof() {
	s.Run("SetsOutputsProof", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		proof := &OutputsProof{
			OutputsHash: UniqueHash(),
			MachineHash: UniqueHash(),
			OutputsHashProof: [][32]byte{
				[32]byte(common.HexToHash("0xaabb")),
			},
		}

		err := s.Repo.UpdateEpochOutputsProof(s.Ctx, seed.App.ID, 0, proof)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Require().NotNil(got.OutputsMerkleRoot)
	})
}

func (s *EpochSuite) TestRepeatPreviousEpochOutputsProof() {
	s.Run("CopiesProofFromPreviousEpoch", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Closed).
			WithBlocks(10, 19).WithInputBounds(1, 1).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}}, 20)
		s.Require().NoError(err)

		// Set proof on epoch 0
		outputsHash := UniqueHash()
		machineHash := UniqueHash()
		proof := &OutputsProof{
			OutputsHash: outputsHash,
			MachineHash: machineHash,
			OutputsHashProof: [][32]byte{
				[32]byte(UniqueHash()),
			},
		}
		err = s.Repo.UpdateEpochOutputsProof(s.Ctx, app.ID, 0, proof)
		s.Require().NoError(err)

		// Copy proof from epoch 0 to epoch 1
		err = s.Repo.RepeatPreviousEpochOutputsProof(s.Ctx, app.ID, 1)
		s.Require().NoError(err)

		// Verify epoch 1 has epoch 0's proof
		got, err := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 1)
		s.Require().NoError(err)
		s.Require().NotNil(got.OutputsMerkleRoot)
		s.Equal(outputsHash, *got.OutputsMerkleRoot)
		s.Require().NotNil(got.MachineHash)
		s.Equal(machineHash, *got.MachineHash)
	})

	s.Run("ErrorsForEpochZero", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		err := s.Repo.RepeatPreviousEpochOutputsProof(s.Ctx, seed.App.ID, 0)
		s.Require().Error(err)
		s.Contains(err.Error(), "epoch 0")
	})

	s.Run("ErrorsForNonExistentEpoch", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		err := s.Repo.RepeatPreviousEpochOutputsProof(s.Ctx, seed.App.ID, 99)
		s.Require().Error(err)
	})
}

// TestUpsertPreservesNonOpenEpoch verifies the CASE/WHEN crash-recovery guard
// in CreateEpochsAndInputs: re-upserting an epoch that has advanced past OPEN
// must preserve the existing row's fields (status, block range, input bounds).
func (s *EpochSuite) TestUpsertPreservesNonOpenEpoch() {
	s.Run("PreservesClosedEpochFields", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// Create and persist an epoch as CLOSED with known field values.
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 99).WithInputBounds(0, 5).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 100)
		s.Require().NoError(err)

		// Re-upsert the same epoch index with DIFFERENT field values.
		// The guard should silently preserve the existing CLOSED row.
		conflicting := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 200).WithInputBounds(0, 10).Build()

		err = s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{conflicting: {}}, 200)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(
			s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_Closed, got.Status)
		s.Equal(uint64(99), got.LastBlock,
			"LastBlock should be preserved from original epoch")
		s.Equal(uint64(5), got.InputIndexUpperBound,
			"InputIndexUpperBound should be preserved from original epoch")
	})

	s.Run("PreservesInputsProcessedEpochFields", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 50).WithInputBounds(0, 3).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 51)
		s.Require().NoError(err)

		// Advance past CLOSED so it is no longer OPEN.
		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch,
			EpochStatus_InputsProcessed)

		// Re-upsert with different values.
		conflicting := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 999).WithInputBounds(0, 99).Build()

		err = s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{conflicting: {}}, 999)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(
			s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_InputsProcessed, got.Status,
			"status should be preserved, not overwritten")
		s.Equal(uint64(50), got.LastBlock,
			"LastBlock should be preserved from original epoch")
		s.Equal(uint64(3), got.InputIndexUpperBound,
			"InputIndexUpperBound should be preserved from original epoch")
	})

	s.Run("AllowsUpdateOfOpenEpoch", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		// Create an OPEN epoch.
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Open).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		// Upsert with CLOSED status and new LastBlock.
		updated := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 50).WithInputBounds(0, 3).Build()

		err = s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{updated: {}}, 50)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(
			s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_Closed, got.Status,
			"OPEN epoch should be updated to CLOSED")
		s.Equal(uint64(50), got.LastBlock,
			"LastBlock should be updated for OPEN epoch")
	})
}

// TestEpochStatusTransitionTrigger verifies the database trigger that enforces
// valid epoch status transitions.
func (s *EpochSuite) TestEpochStatusTransitionTrigger() {
	s.Run("RejectsSkippedTransition", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// CLOSED -> CLAIM_COMPUTED skips INPUTS_PROCESSED — must fail.
		seed.Epoch.Status = EpochStatus_ClaimComputed
		err := s.Repo.UpdateEpochStatus(
			s.Ctx, seed.App.IApplicationAddress.String(), seed.Epoch)
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid epoch status transition")
	})

	s.Run("RejectsBackwardsTransition", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			seed.App.IApplicationAddress.String(), seed.Epoch,
			EpochStatus_InputsProcessed)

		// INPUTS_PROCESSED -> CLOSED is backwards — must fail.
		seed.Epoch.Status = EpochStatus_Closed
		err := s.Repo.UpdateEpochStatus(
			s.Ctx, seed.App.IApplicationAddress.String(), seed.Epoch)
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid epoch status transition")
	})

	s.Run("RejectsOpenToInputsProcessed", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Open).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		// OPEN -> INPUTS_PROCESSED skips CLOSED — must fail.
		epoch.Status = EpochStatus_InputsProcessed
		err = s.Repo.UpdateEpochStatus(
			s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid epoch status transition")
	})

	// CLAIM_COMPUTED -> CLAIM_ACCEPTED (skipping SUBMITTED) is valid for:
	// PRT apps (no claim submission step), node syncing from scratch when
	// the claim was already accepted, or reader-only mode with tx submission
	// disabled.
	s.Run("AllowsDirectAcceptance", func() {
		app := NewApplicationBuilder().
			WithConsensus(Consensus_PRT).
			Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).
			WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).
			WithInputBounds(0, 0).
			Build()

		input := NewInputBuilder().
			WithIndex(0).
			WithBlockNumber(5).
			Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			app.IApplicationAddress.String(), epoch,
			EpochStatus_ClaimComputed)

		// CLAIM_COMPUTED -> CLAIM_ACCEPTED skips SUBMITTED.
		epoch.Status = EpochStatus_ClaimAccepted
		err = s.Repo.UpdateEpochStatus(
			s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(
			s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimAccepted, got.Status)
	})

	// CLAIM_COMPUTED -> CLAIM_REJECTED is valid when a claim is rejected
	// on-chain before the node submits it (e.g. a conflicting claim was
	// already accepted).
	s.Run("AllowsDirectRejection", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			seed.App.IApplicationAddress.String(), seed.Epoch,
			EpochStatus_ClaimComputed)

		// CLAIM_COMPUTED -> CLAIM_REJECTED skips SUBMITTED.
		seed.Epoch.Status = EpochStatus_ClaimRejected
		err := s.Repo.UpdateEpochStatus(
			s.Ctx, seed.App.IApplicationAddress.String(), seed.Epoch)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimRejected, got.Status)
	})

	s.Run("AllowsSameStatusUpdate", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// CLOSED -> CLOSED should be allowed (idempotent).
		err := s.Repo.UpdateEpochStatus(
			s.Ctx, seed.App.IApplicationAddress.String(), seed.Epoch)
		s.Require().NoError(err)
	})

	// Verify the trigger rejects CLAIM_COMPUTED when proof fields are missing.
	s.Run("RejectsClaimComputedWithoutProofFields", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			seed.App.IApplicationAddress.String(), seed.Epoch,
			EpochStatus_InputsProcessed)

		// Try INPUTS_PROCESSED -> CLAIM_COMPUTED without setting
		// machine_hash, outputs_merkle_root, outputs_merkle_proof.
		seed.Epoch.Status = EpochStatus_ClaimComputed
		err := s.Repo.UpdateEpochStatus(
			s.Ctx, seed.App.IApplicationAddress.String(), seed.Epoch)
		s.Require().Error(err)
		s.Contains(err.Error(), "CLAIM_COMPUTED requires")
	})

	// Verify CLAIM_COMPUTED succeeds when all required fields are present.
	s.Run("AllowsClaimComputedWithProofFields", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			seed.App.IApplicationAddress.String(), seed.Epoch,
			EpochStatus_InputsProcessed)

		// Set the required proof fields.
		proof := &OutputsProof{
			OutputsHash:      UniqueHash(),
			OutputsHashProof: [][32]byte{[32]byte(UniqueHash())},
			MachineHash:      UniqueHash(),
		}
		err := s.Repo.UpdateEpochOutputsProof(
			s.Ctx, seed.App.ID, seed.Epoch.Index, proof)
		s.Require().NoError(err)

		seed.Epoch.Status = EpochStatus_ClaimComputed
		err = s.Repo.UpdateEpochStatus(
			s.Ctx, seed.App.IApplicationAddress.String(), seed.Epoch)
		s.Require().NoError(err)

		got, err := s.Repo.GetEpoch(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimComputed, got.Status)
	})

	// Verify PRT apps require commitment fields for CLAIM_COMPUTED.
	s.Run("RejectsPRTClaimComputedWithoutCommitment", func() {
		app := NewApplicationBuilder().
			WithConsensus(Consensus_PRT).
			Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		// Advance to INPUTS_PROCESSED.
		epoch.Status = EpochStatus_InputsProcessed
		err = s.Repo.UpdateEpochStatus(
			s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().NoError(err)

		// Set base proof fields but NOT commitment.
		proof := &OutputsProof{
			OutputsHash:      UniqueHash(),
			OutputsHashProof: [][32]byte{[32]byte(UniqueHash())},
			MachineHash:      UniqueHash(),
		}
		err = s.Repo.UpdateEpochOutputsProof(
			s.Ctx, app.ID, epoch.Index, proof)
		s.Require().NoError(err)

		// INPUTS_PROCESSED -> CLAIM_COMPUTED without commitment — must fail.
		epoch.Status = EpochStatus_ClaimComputed
		err = s.Repo.UpdateEpochStatus(
			s.Ctx, app.IApplicationAddress.String(), epoch)
		s.Require().Error(err)
		s.Contains(err.Error(), "PRT")
	})
}

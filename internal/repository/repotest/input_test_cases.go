// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

type InputSuite struct {
	BaseSuite
}

func NewInputSuite(factory RepositoryFactory) *InputSuite {
	return &InputSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *InputSuite) TestGetInput() {
	s.Run("ExistingInput", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetInput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(seed.App.ID, got.EpochApplicationID)
		s.Equal(uint64(0), got.EpochIndex)
		s.Equal(uint64(0), got.Index)
		s.Equal(seed.Input.BlockNumber, got.BlockNumber)
		s.Equal(seed.Input.RawData, got.RawData)
		s.Equal(InputCompletionStatus_None, got.Status)
		s.Equal(seed.Input.TransactionHash, got.TransactionHash)
		s.Equal(seed.Input.LogIndex, got.LogIndex)
		s.Nil(got.MachineHash)
		s.Nil(got.OutputsHash)
		s.Nil(got.SnapshotURI)
		s.False(got.CreatedAt.IsZero(), "CreatedAt should be set")
		s.False(got.UpdatedAt.IsZero(), "UpdatedAt should be set")
	})

	s.Run("NotFound", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetInput(s.Ctx, app.IApplicationAddress.String(), 99)
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *InputSuite) TestGetLastInput() {
	s.Run("ReturnsLastInput", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 19).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(10).Build()
		input2 := NewInputBuilder().WithIndex(2).WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 20)
		s.Require().NoError(err)

		got, err := s.Repo.GetLastInput(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(uint64(2), got.Index)
	})
}

func (s *InputSuite) TestGetLastProcessedInput() {
	s.Run("ReturnsLastProcessed", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 19).WithInputBounds(0, 1).Build()

		input0 := NewInputBuilder().
			WithIndex(0).WithBlockNumber(5).
			WithStatus(InputCompletionStatus_Accepted).Build()
		input1 := NewInputBuilder().
			WithIndex(1).WithBlockNumber(10).
			WithStatus(InputCompletionStatus_None).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1}}, 20)
		s.Require().NoError(err)

		got, err := s.Repo.GetLastProcessedInput(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), got.Index)
		s.Equal(InputCompletionStatus_Accepted, got.Status)
	})
}

func (s *InputSuite) TestCreateEpochsAndInputsWithSameTransactionHash() {
	s.Run("AllowsMultipleLogsFromSameTransaction", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 19).WithInputBounds(0, 1).Build()

		txHash := UniqueHash()
		input0 := NewInputBuilder().
			WithIndex(0).
			WithTransactionHash(txHash).
			WithLogIndex(7).
			Build()
		input1 := NewInputBuilder().
			WithIndex(1).
			WithTransactionHash(txHash).
			WithLogIndex(8).
			Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1}}, 20)
		s.Require().NoError(err)

		got0, err := s.Repo.GetInput(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Require().NotNil(got0)
		s.Equal(txHash, got0.TransactionHash)
		s.Equal(uint64(7), got0.LogIndex)

		got1, err := s.Repo.GetInput(s.Ctx, app.IApplicationAddress.String(), 1)
		s.Require().NoError(err)
		s.Require().NotNil(got1)
		s.Equal(txHash, got1.TransactionHash)
		s.Equal(uint64(8), got1.LogIndex)

		lastBlock, err := s.Repo.GetEventLastCheckBlock(s.Ctx, app.ID, MonitoredEvent_InputAdded)
		s.Require().NoError(err)
		s.Equal(uint64(20), lastBlock)

		err = s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1}}, 20)
		s.Require().NoError(err)

		count, err := s.Repo.GetNumberOfInputs(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(2), count)
	})

	s.Run("RejectsDuplicateLogIdentity", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 19).WithInputBounds(0, 1).Build()

		blockBefore, blockErr := s.Repo.GetEventLastCheckBlock(s.Ctx, app.ID, MonitoredEvent_InputAdded)
		s.Require().NoError(blockErr)

		txHash := UniqueHash()
		input0 := NewInputBuilder().
			WithIndex(0).
			WithTransactionHash(txHash).
			WithLogIndex(7).
			Build()
		input1 := NewInputBuilder().
			WithIndex(1).
			WithTransactionHash(txHash).
			WithLogIndex(7).
			Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1}}, 20)
		s.Require().ErrorIs(err, repository.ErrInputLogIdentityConflict)

		// The whole batch must roll back: no inputs, no epoch row, and the
		// input cursor must keep its exact previous value.
		count, countErr := s.Repo.GetNumberOfInputs(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(countErr)
		s.Equal(uint64(0), count)

		gotEpoch, epochErr := s.Repo.GetEpoch(s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(epochErr)
		s.Nil(gotEpoch)

		blockAfter, blockErr := s.Repo.GetEventLastCheckBlock(s.Ctx, app.ID, MonitoredEvent_InputAdded)
		s.Require().NoError(blockErr)
		s.Equal(blockBefore, blockAfter)
	})

	s.Run("AllowsSameLogIdentityInDifferentApplications", func() {
		// The L1 log identity constraint is scoped per application: the same
		// (transaction_hash, log_index) must persist independently for two
		// different applications.
		appA := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		appB := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		txHash := UniqueHash()
		for _, app := range []*Application{appA, appB} {
			epoch := NewEpochBuilder(app.ID).
				WithIndex(0).WithStatus(EpochStatus_Closed).
				WithBlocks(0, 19).WithInputBounds(0, 0).Build()
			input := NewInputBuilder().
				WithIndex(0).
				WithTransactionHash(txHash).
				WithLogIndex(7).
				Build()

			err := s.Repo.CreateEpochsAndInputs(
				s.Ctx, app.IApplicationAddress.String(),
				map[*Epoch][]*Input{epoch: {input}}, 20)
			s.Require().NoError(err)
		}

		for _, app := range []*Application{appA, appB} {
			got, err := s.Repo.GetInput(s.Ctx, app.IApplicationAddress.String(), 0)
			s.Require().NoError(err)
			s.Require().NotNil(got)
			s.Equal(txHash, got.TransactionHash)
			s.Equal(uint64(7), got.LogIndex)
		}
	})
}

func (s *InputSuite) TestListInputs() {
	s.Run("EmptyResult", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		inputs, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(inputs)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsAllInputs", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 29).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(10).Build()
		input2 := NewInputBuilder().WithIndex(2).WithBlockNumber(20).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 30)
		s.Require().NoError(err)

		inputs, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(inputs, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("FilterByEpochIndex", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch0 := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()
		epoch1 := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Open).
			WithBlocks(10, 19).WithInputBounds(1, 1).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithEpochIndex(1).WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch0: {input0}, epoch1: {input1}}, 20)
		s.Require().NoError(err)

		epochIdx := uint64(1)
		inputs, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(inputs, 1)
		s.Equal(uint64(1), total)
		s.Equal(uint64(1), inputs[0].Index)
	})

	s.Run("FilterByStatus", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 19).WithInputBounds(0, 1).Build()

		input0 := NewInputBuilder().
			WithIndex(0).WithBlockNumber(5).
			WithStatus(InputCompletionStatus_Accepted).Build()
		input1 := NewInputBuilder().
			WithIndex(1).WithBlockNumber(10).
			WithStatus(InputCompletionStatus_None).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1}}, 20)
		s.Require().NoError(err)

		status := InputCompletionStatus_Accepted
		inputs, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{Status: &status},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(inputs, 1)
		s.Equal(uint64(1), total)
		s.Equal(InputCompletionStatus_Accepted, inputs[0].Status)
	})

	s.Run("FilterByNotStatus", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 19).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().
			WithIndex(0).WithBlockNumber(5).
			WithStatus(InputCompletionStatus_Accepted).Build()
		input1 := NewInputBuilder().
			WithIndex(1).WithBlockNumber(10).
			WithStatus(InputCompletionStatus_Rejected).Build()
		input2 := NewInputBuilder().
			WithIndex(2).WithBlockNumber(15).
			WithStatus(InputCompletionStatus_None).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 20)
		s.Require().NoError(err)

		notStatus := InputCompletionStatus_None
		inputs, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{NotStatus: &notStatus},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(inputs, 2)
		s.Equal(uint64(2), total)
		for _, inp := range inputs {
			s.NotEqual(InputCompletionStatus_None, inp.Status)
		}
	})

	s.Run("FilterBySender", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 19).WithInputBounds(0, 1).Build()

		// The Sender filter uses SUBSTR(raw_data, 81, 20) to extract a
		// 20-byte sender address from the ABI-encoded input payload.
		senderAddr := UniqueAddress()
		rawWithSender := make([]byte, 101)
		copy(rawWithSender[80:100], senderAddr.Bytes())

		otherAddr := UniqueAddress()
		rawWithOther := make([]byte, 101)
		copy(rawWithOther[80:100], otherAddr.Bytes())

		input0 := NewInputBuilder().
			WithIndex(0).WithBlockNumber(5).
			WithRawData(rawWithSender).Build()
		input1 := NewInputBuilder().
			WithIndex(1).WithBlockNumber(10).
			WithRawData(rawWithOther).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1}}, 20)
		s.Require().NoError(err)

		inputs, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{Sender: &senderAddr},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(inputs, 1)
		s.Equal(uint64(1), total)
		s.Equal(uint64(0), inputs[0].Index)
	})

	s.Run("FilterByTransactionHash", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		otherApp := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 29).WithInputBounds(0, 2).Build()
		otherEpoch := NewEpochBuilder(otherApp.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 9).WithInputBounds(0, 0).Build()

		txHash := UniqueHash()
		input0 := NewInputBuilder().
			WithIndex(0).WithTransactionHash(txHash).WithLogIndex(10).Build()
		input1 := NewInputBuilder().
			WithIndex(1).WithTransactionHash(UniqueHash()).WithLogIndex(11).Build()
		input2 := NewInputBuilder().
			WithIndex(2).WithTransactionHash(txHash).WithLogIndex(12).Build()
		otherInput := NewInputBuilder().
			WithIndex(0).WithTransactionHash(txHash).WithLogIndex(13).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 30)
		s.Require().NoError(err)
		err = s.Repo.CreateEpochsAndInputs(
			s.Ctx, otherApp.IApplicationAddress.String(),
			map[*Epoch][]*Input{otherEpoch: {otherInput}}, 10)
		s.Require().NoError(err)

		inputs, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{TransactionHash: &txHash},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Require().Len(inputs, 2)
		s.Equal(uint64(2), total)
		s.Equal(uint64(0), inputs[0].Index)
		s.Equal(uint64(2), inputs[1].Index)
		for _, input := range inputs {
			s.Equal(txHash, input.TransactionHash)
		}

		missing := UniqueHash()
		inputs, total, err = s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{TransactionHash: &missing},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(inputs)
		s.Equal(uint64(0), total)
	})

	s.Run("Pagination", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 49).WithInputBounds(0, 4).Build()

		inputs := make([]*Input, 5)
		for i := range uint64(5) {
			inputs[i] = NewInputBuilder().WithIndex(i).WithBlockNumber(i*10 + 5).Build()
		}
		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: inputs}, 50)
		s.Require().NoError(err)

		got, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{},
			repository.Pagination{Limit: 2, Offset: 0}, false)
		s.Require().NoError(err)
		s.Len(got, 2)
		s.Equal(uint64(5), total)
	})

	s.Run("TotalCountCorrectWhenOffsetExceedsRows", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 29).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(10).Build()
		input2 := NewInputBuilder().WithIndex(2).WithBlockNumber(20).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 30)
		s.Require().NoError(err)

		// OFFSET == total rows: 0 data rows, but totalCount must still be 3
		got, total, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{},
			repository.Pagination{Limit: 10, Offset: 3}, false)
		s.Require().NoError(err)
		s.Empty(got)
		s.Equal(uint64(3), total, "totalCount must be correct even when OFFSET skips all rows")

		// OFFSET > total rows: same expectation
		got, total, err = s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{},
			repository.Pagination{Limit: 10, Offset: 100}, false)
		s.Require().NoError(err)
		s.Empty(got)
		s.Equal(uint64(3), total, "totalCount must be correct even when OFFSET far exceeds rows")
	})

	s.Run("Descending", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 29).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(10).Build()
		input2 := NewInputBuilder().WithIndex(2).WithBlockNumber(20).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 30)
		s.Require().NoError(err)

		inputs, _, err := s.Repo.ListInputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.InputFilter{},
			repository.Pagination{Limit: 10}, true)
		s.Require().NoError(err)
		s.Require().Len(inputs, 3)
		// Descending: highest index first
		s.Equal(uint64(2), inputs[0].Index)
		s.Equal(uint64(1), inputs[1].Index)
		s.Equal(uint64(0), inputs[2].Index)
	})
}

func (s *InputSuite) TestGetNumberOfInputs() {
	s.Run("ReturnsCount", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 19).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(10).Build()
		input2 := NewInputBuilder().WithIndex(2).WithBlockNumber(15).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 20)
		s.Require().NoError(err)

		count, err := s.Repo.GetNumberOfInputs(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(3), count)
	})

	s.Run("ZeroWhenEmpty", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		count, err := s.Repo.GetNumberOfInputs(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), count)
	})
}

func (s *InputSuite) TestUpdateInputSnapshotURI() {
	s.Run("SetsSnapshotURI", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		uri := "/snapshots/test"

		err := s.Repo.UpdateInputSnapshotURI(s.Ctx, seed.App.ID, 0, uri)
		s.Require().NoError(err)

		got, err := s.Repo.GetInput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Require().NotNil(got.SnapshotURI)
		s.Equal(uri, *got.SnapshotURI)
	})
}

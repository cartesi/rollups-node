// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/crypto"
)

type OutputSuite struct {
	BaseSuite
}

func NewOutputSuite(factory RepositoryFactory) *OutputSuite {
	return &OutputSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *OutputSuite) TestGetOutput() {
	s.Run("ExistingOutput", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		out := NewOutputBuilder(seed.App.ID).
			WithEpochIndex(0).WithInputIndex(0).WithIndex(0).
			WithRawData([]byte("output-data")).Build()
		err := s.Repo.CreateOutput(s.Ctx, out)
		s.Require().NoError(err)

		got, err := s.Repo.GetOutput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(uint64(0), got.Index)
		s.Equal([]byte("output-data"), got.RawData)
	})

	s.Run("NotFound", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetOutput(s.Ctx, app.IApplicationAddress.String(), 99)
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *OutputSuite) TestListOutputs() {
	s.Run("EmptyResult", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(outputs)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsAllOutputs", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		for i := range uint64(3) {
			out := NewOutputBuilder(seed.App.ID).
				WithEpochIndex(0).WithInputIndex(0).WithIndex(i).Build()
			err := s.Repo.CreateOutput(s.Ctx, out)
			s.Require().NoError(err)
		}

		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("FilterByEpochIndex", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// EpochIndex filter also requires input.status = ACCEPTED,
		// so use StoreAdvanceResult to create the output with accepted input.
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("epoch-output")},
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}
		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		epochIdx := uint64(0)
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 1)
		s.Equal(uint64(1), total)
	})

	s.Run("FilterByInputIndex", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		for i := range uint64(3) {
			out := NewOutputBuilder(seed.App.ID).
				WithEpochIndex(0).WithInputIndex(0).WithIndex(i).Build()
			err := s.Repo.CreateOutput(s.Ctx, out)
			s.Require().NoError(err)
		}

		inputIdx := uint64(0)
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{InputIndex: &inputIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("FilterByBlockRange", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 99).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(10).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(50).Build()
		input2 := NewInputBuilder().WithIndex(2).WithBlockNumber(90).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 100)
		s.Require().NoError(err)

		// Store advance results to create outputs with accepted inputs
		for i := range uint64(3) {
			result := &AdvanceResult{
				EpochIndex: 0,
				InputIndex: i,
				Status:     InputCompletionStatus_Accepted,
				Outputs:    [][]byte{[]byte("output-data")},
				OutputsProof: OutputsProof{
					OutputsHash: UniqueHash(),
					MachineHash: UniqueHash(),
				},
			}
			err = s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result)
			s.Require().NoError(err)
		}

		// Filter for block range 40-60 (should match input1 at block 50)
		blockRange := repository.Range{Start: 40, End: 60}
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, app.IApplicationAddress.String(),
			repository.OutputFilter{BlockRange: &blockRange},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 1)
		s.Equal(uint64(1), total)
	})

	s.Run("FilterByOutputType", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// OutputType filter uses SUBSTR(raw_data, 1, 4) to match the first 4 bytes
		targetType := []byte{0xef, 0x01, 0xab, 0xcd}
		rawWithType := make([]byte, 32)
		copy(rawWithType[0:4], targetType)

		otherType := []byte{0x00, 0x00, 0x00, 0x00}
		rawWithOther := make([]byte, 32)
		copy(rawWithOther[0:4], otherType)

		out0 := NewOutputBuilder(seed.App.ID).
			WithEpochIndex(0).WithInputIndex(0).WithIndex(0).
			WithRawData(rawWithType).Build()
		err := s.Repo.CreateOutput(s.Ctx, out0)
		s.Require().NoError(err)

		out1 := NewOutputBuilder(seed.App.ID).
			WithEpochIndex(0).WithInputIndex(0).WithIndex(1).
			WithRawData(rawWithOther).Build()
		err = s.Repo.CreateOutput(s.Ctx, out1)
		s.Require().NoError(err)

		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{OutputType: &targetType},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 1)
		s.Equal(uint64(1), total)
		s.Equal(rawWithType, outputs[0].RawData)
	})

	s.Run("FilterByVoucherAddress", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// VoucherAddress filter uses SUBSTR(raw_data, 17, 20)
		// to extract a 20-byte address at bytes 17-36 (1-indexed)
		voucherAddr := UniqueAddress()
		rawWithVoucher := make([]byte, 64)
		copy(rawWithVoucher[16:36], voucherAddr.Bytes())

		otherAddr := UniqueAddress()
		rawWithOther := make([]byte, 64)
		copy(rawWithOther[16:36], otherAddr.Bytes())

		out0 := NewOutputBuilder(seed.App.ID).
			WithEpochIndex(0).WithInputIndex(0).WithIndex(0).
			WithRawData(rawWithVoucher).Build()
		err := s.Repo.CreateOutput(s.Ctx, out0)
		s.Require().NoError(err)

		out1 := NewOutputBuilder(seed.App.ID).
			WithEpochIndex(0).WithInputIndex(0).WithIndex(1).
			WithRawData(rawWithOther).Build()
		err = s.Repo.CreateOutput(s.Ctx, out1)
		s.Require().NoError(err)

		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{VoucherAddress: &voucherAddr},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 1)
		s.Equal(uint64(1), total)
		s.Equal(rawWithVoucher, outputs[0].RawData)
	})

	s.Run("Pagination", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		for i := range uint64(5) {
			out := NewOutputBuilder(seed.App.ID).
				WithEpochIndex(0).WithInputIndex(0).WithIndex(i).Build()
			err := s.Repo.CreateOutput(s.Ctx, out)
			s.Require().NoError(err)
		}

		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{},
			repository.Pagination{Limit: 2, Offset: 0}, false)
		s.Require().NoError(err)
		s.Len(outputs, 2)
		s.Equal(uint64(5), total)
	})

	s.Run("Descending", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		for i := range uint64(3) {
			out := NewOutputBuilder(seed.App.ID).
				WithEpochIndex(0).WithInputIndex(0).WithIndex(i).Build()
			err := s.Repo.CreateOutput(s.Ctx, out)
			s.Require().NoError(err)
		}

		outputs, _, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{},
			repository.Pagination{Limit: 10}, true)
		s.Require().NoError(err)
		s.Require().Len(outputs, 3)
		// Descending: highest index first
		s.Equal(uint64(2), outputs[0].Index)
		s.Equal(uint64(1), outputs[1].Index)
		s.Equal(uint64(0), outputs[2].Index)
	})
}

func (s *OutputSuite) TestUpdateOutputsExecution() {
	s.Run("UpdatesExecutionHash", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		out := NewOutputBuilder(seed.App.ID).
			WithEpochIndex(0).WithInputIndex(0).WithIndex(0).Build()
		err := s.Repo.CreateOutput(s.Ctx, out)
		s.Require().NoError(err)

		txHash := UniqueHash()
		out.ExecutionTransactionHash = &txHash
		err = s.Repo.UpdateOutputsExecution(
			s.Ctx, seed.App.IApplicationAddress.String(), []*Output{out}, 100)
		s.Require().NoError(err)

		got, err := s.Repo.GetOutput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Require().NotNil(got.ExecutionTransactionHash)
		s.Equal(txHash, *got.ExecutionTransactionHash)
	})

	// Regression guard: all output updates must be transactional.
	// Verify multiple outputs are updated atomically in a single call.
	s.Run("MultipleOutputsUpdatedAtomically", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// Create 3 outputs
		for i := range uint64(3) {
			out := NewOutputBuilder(seed.App.ID).
				WithEpochIndex(0).WithInputIndex(0).WithIndex(i).Build()
			err := s.Repo.CreateOutput(s.Ctx, out)
			s.Require().NoError(err)
		}

		txHash := UniqueHash()
		outputs := make([]*Output, 3)
		for i := range uint64(3) {
			outputs[i] = &Output{
				InputEpochApplicationID:  seed.App.ID,
				Index:                    i,
				ExecutionTransactionHash: &txHash,
			}
		}

		err := s.Repo.UpdateOutputsExecution(
			s.Ctx, seed.App.IApplicationAddress.String(), outputs, 200)
		s.Require().NoError(err)

		// Verify all 3 were updated
		for i := range uint64(3) {
			got, err := s.Repo.GetOutput(
				s.Ctx, seed.App.IApplicationAddress.String(), i)
			s.Require().NoError(err)
			s.Require().NotNil(got.ExecutionTransactionHash,
				"output %d should have execution hash", i)
			s.Equal(txHash, *got.ExecutionTransactionHash)
		}
	})

	s.Run("NilExecutionHashReturnsError", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		out := NewOutputBuilder(seed.App.ID).
			WithEpochIndex(0).WithInputIndex(0).WithIndex(0).Build()
		err := s.Repo.CreateOutput(s.Ctx, out)
		s.Require().NoError(err)

		// ExecutionTransactionHash is nil — should fail
		badOutput := &Output{
			InputEpochApplicationID: seed.App.ID,
			Index:                   0,
		}
		err = s.Repo.UpdateOutputsExecution(
			s.Ctx, seed.App.IApplicationAddress.String(),
			[]*Output{badOutput}, 100)
		s.Require().Error(err)
	})

	// Verify that a failure mid-loop rolls back all prior output updates.
	// We create 3 outputs, set valid hashes on the first two, and use a
	// non-existent index for the third. The third update should fail
	// (RowsAffected == 0), rolling back the first two.
	s.Run("RollbackOnPartialFailure", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// Create 2 valid outputs (index 0 and 1)
		for i := range uint64(2) {
			out := NewOutputBuilder(seed.App.ID).
				WithEpochIndex(0).WithInputIndex(0).WithIndex(i).Build()
			err := s.Repo.CreateOutput(s.Ctx, out)
			s.Require().NoError(err)
		}

		txHash := UniqueHash()
		outputs := []*Output{
			{
				InputEpochApplicationID:  seed.App.ID,
				Index:                    0,
				ExecutionTransactionHash: &txHash,
			},
			{
				InputEpochApplicationID:  seed.App.ID,
				Index:                    1,
				ExecutionTransactionHash: &txHash,
			},
			{
				InputEpochApplicationID:  seed.App.ID,
				Index:                    999, // non-existent
				ExecutionTransactionHash: &txHash,
			},
		}

		err := s.Repo.UpdateOutputsExecution(
			s.Ctx, seed.App.IApplicationAddress.String(), outputs, 200)
		s.Require().Error(err)

		// Verify that the first two outputs were NOT updated (rolled back)
		for i := range uint64(2) {
			got, err := s.Repo.GetOutput(
				s.Ctx, seed.App.IApplicationAddress.String(), i)
			s.Require().NoError(err)
			s.Nil(got.ExecutionTransactionHash,
				"output %d should not have execution hash after rollback", i)
		}

		// Verify no executed outputs exist
		count, err := s.Repo.GetNumberOfExecutedOutputs(
			s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), count)
	})

	// Verify that a nil hash on the second output rolls back the first
	// output's successful update. This exercises the nil-check rollback
	// path at the top of the loop.
	s.Run("NilHashMidLoopRollsBackPrior", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		for i := range uint64(2) {
			out := NewOutputBuilder(seed.App.ID).
				WithEpochIndex(0).WithInputIndex(0).WithIndex(i).Build()
			err := s.Repo.CreateOutput(s.Ctx, out)
			s.Require().NoError(err)
		}

		txHash := UniqueHash()
		outputs := []*Output{
			{
				InputEpochApplicationID:  seed.App.ID,
				Index:                    0,
				ExecutionTransactionHash: &txHash,
			},
			{
				InputEpochApplicationID: seed.App.ID,
				Index:                   1,
				// nil ExecutionTransactionHash — triggers error
			},
		}

		err := s.Repo.UpdateOutputsExecution(
			s.Ctx, seed.App.IApplicationAddress.String(), outputs, 200)
		s.Require().Error(err)

		// First output should NOT have been updated (rolled back)
		got, err := s.Repo.GetOutput(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Nil(got.ExecutionTransactionHash,
			"output 0 should not have execution hash after rollback")
	})
}

func (s *OutputSuite) TestGetLastOutputBeforeBlock() {
	s.Run("NoOutputReturnsNil", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetLastOutputBeforeBlock(
			s.Ctx, app.IApplicationAddress.String(), 100)
		s.Require().NoError(err)
		s.Nil(got)
	})

	s.Run("ReturnsLastOutputBeforeBlock", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).
			WithBlocks(0, 99).WithInputBounds(0, 2).Build()

		input0 := NewInputBuilder().WithIndex(0).WithBlockNumber(10).Build()
		input1 := NewInputBuilder().WithIndex(1).WithBlockNumber(50).Build()
		input2 := NewInputBuilder().WithIndex(2).WithBlockNumber(90).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input0, input1, input2}}, 100)
		s.Require().NoError(err)

		// Store advance results to create outputs with accepted inputs
		for i := range uint64(3) {
			result := &AdvanceResult{
				EpochIndex: 0,
				InputIndex: i,
				Status:     InputCompletionStatus_Accepted,
				Outputs:    [][]byte{[]byte("output-data")},
				OutputsProof: OutputsProof{
					OutputsHash: UniqueHash(),
					MachineHash: crypto.Keccak256Hash([]byte("machine")),
				},
			}
			err = s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result)
			s.Require().NoError(err)
		}

		// Query for outputs before block 60 (should return output from input1 at block 50)
		got, err := s.Repo.GetLastOutputBeforeBlock(
			s.Ctx, app.IApplicationAddress.String(), 60)
		s.Require().NoError(err)
		s.Require().NotNil(got)
		// The last output before block 60 should be from input1 (index 1)
		s.Equal(uint64(1), got.InputIndex)
	})
}

func (s *OutputSuite) TestGetNumberOfExecutedOutputs() {
	s.Run("ReturnsZeroWhenNone", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		count, err := s.Repo.GetNumberOfExecutedOutputs(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), count)
	})

	s.Run("ReturnsCountAfterExecution", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// Create outputs
		for i := range uint64(3) {
			out := NewOutputBuilder(seed.App.ID).
				WithEpochIndex(0).WithInputIndex(0).WithIndex(i).Build()
			err := s.Repo.CreateOutput(s.Ctx, out)
			s.Require().NoError(err)
		}

		// Execute 2 of the 3 outputs
		txHash := UniqueHash()
		out0 := &Output{
			InputEpochApplicationID:  seed.App.ID,
			Index:                    0,
			ExecutionTransactionHash: &txHash,
		}
		out1 := &Output{
			InputEpochApplicationID:  seed.App.ID,
			Index:                    1,
			ExecutionTransactionHash: &txHash,
		}
		err := s.Repo.UpdateOutputsExecution(
			s.Ctx, seed.App.IApplicationAddress.String(),
			[]*Output{out0, out1}, 200)
		s.Require().NoError(err)

		count, err := s.Repo.GetNumberOfExecutedOutputs(
			s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(2), count)
	})
}

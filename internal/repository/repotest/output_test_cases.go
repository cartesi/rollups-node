// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("output-data")}, nil)

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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("o0"), []byte("o1"), []byte("o2")}, nil)

		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("IndexRangeComposesWithPaginationAndDescending", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("o0"), []byte("o1"), []byte("o2"), []byte("o3"), []byte("o4")}, nil)

		indexRange := repository.Range{Start: 1, End: 3}
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{IndexRange: &indexRange},
			repository.Pagination{Limit: 1, Offset: 1}, true)
		s.Require().NoError(err)
		s.Require().Len(outputs, 1)
		s.Equal(uint64(3), total)
		s.Equal(uint64(2), outputs[0].Index)
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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("o0"), []byte("o1"), []byte("o2")}, nil)

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

	// Regression: DaveConsensus epochs overlap by one block — a sealed epoch's
	// last block equals the next epoch's first block. When inputs straddle that
	// boundary block (one in the sealed epoch, one in the open epoch, both on the
	// shared block), a block-range output query is ambiguous and returns both
	// epochs' outputs, while an epoch-index query stays unambiguous. The validator
	// scopes epoch outputs by epoch index for exactly this reason.
	s.Run("EpochIndexDisambiguatesOverlappingBoundaryBlock", func() {
		app := NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(s.Ctx, s.T(), s.Repo)
		addr := app.IApplicationAddress.String()

		const sealBlock = 100
		sealedEpoch := NewEpochBuilder(app.ID).
			WithIndex(1).WithStatus(EpochStatus_Closed).
			WithBlocks(90, sealBlock).WithInputBounds(0, 1).Build()
		openEpoch := NewEpochBuilder(app.ID).
			WithIndex(2).WithStatus(EpochStatus_Open).
			WithBlocks(sealBlock, 110).WithInputBounds(1, 2).Build()

		// Both inputs sit on the shared seal block, one on each side of the seal.
		inputBefore := NewInputBuilder().WithIndex(0).WithEpochIndex(1).WithBlockNumber(sealBlock).Build()
		inputAfter := NewInputBuilder().WithIndex(1).WithEpochIndex(2).WithBlockNumber(sealBlock).Build()

		err := s.Repo.CreateEpochsAndInputs(s.Ctx, addr,
			map[*Epoch][]*Input{
				sealedEpoch: {inputBefore},
				openEpoch:   {inputAfter},
			}, 111)
		s.Require().NoError(err)

		for _, e := range []struct{ epoch, input uint64 }{{1, 0}, {2, 1}} {
			result := &AdvanceResult{
				EpochIndex: e.epoch,
				InputIndex: e.input,
				Status:     InputCompletionStatus_Accepted,
				Outputs:    [][]byte{[]byte("output-data")},
				OutputsProof: OutputsProof{
					OutputsHash: UniqueHash(),
					MachineHash: UniqueHash(),
				},
			}
			s.Require().NoError(s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result))
		}

		// Epoch-index scoping returns exactly one output per epoch.
		sealedIdx := uint64(1)
		sealedOutputs, _, err := s.Repo.ListOutputs(s.Ctx, addr,
			repository.OutputFilter{EpochIndex: &sealedIdx}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(sealedOutputs, 1, "sealed epoch must own only its own output")

		openIdx := uint64(2)
		openOutputs, _, err := s.Repo.ListOutputs(s.Ctx, addr,
			repository.OutputFilter{EpochIndex: &openIdx}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(openOutputs, 1, "open epoch must own only its own output")

		// Block-range scoping over the sealed epoch is ambiguous at the boundary:
		// it also returns the open epoch's output, since both are on the seal block.
		boundary := repository.Range{Start: sealedEpoch.FirstBlock, End: sealBlock}
		rangeOutputs, _, err := s.Repo.ListOutputs(s.Ctx, addr,
			repository.OutputFilter{BlockRange: &boundary}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(rangeOutputs, 2,
			"block-range scoping is ambiguous at the overlap — it returns both epochs' "+
				"outputs, which is why the validator scopes by epoch index instead")
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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{rawWithType, rawWithOther}, nil)

		targetTypes := [][]byte{targetType}
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{OutputType: &targetTypes},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 1)
		s.Equal(uint64(1), total)
		s.Equal(rawWithType, outputs[0].RawData)
	})

	s.Run("FilterByOutputTypesAndExecutionStatus", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		voucherSelector := []byte{0x23, 0x7a, 0x81, 0x6f}
		delegateCallVoucherSelector := []byte{0x10, 0x32, 0x1e, 0x8b}
		voucher := append([]byte{}, voucherSelector...)
		delegateCallVoucher := append([]byte{}, delegateCallVoucherSelector...)
		notice := []byte{0xc2, 0x58, 0xd6, 0xe5}
		executedVoucher := append([]byte{}, voucherSelector...)
		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{voucher, delegateCallVoucher, notice, executedVoucher}, nil)

		txHash := UniqueHash()
		err := s.Repo.UpdateOutputsExecution(
			s.Ctx,
			seed.App.IApplicationAddress.String(),
			[]*Output{{
				InputEpochApplicationID:  seed.App.ID,
				Index:                    3,
				ExecutionTransactionHash: &txHash,
			}},
			200,
		)
		s.Require().NoError(err)

		outputTypes := [][]byte{voucherSelector, delegateCallVoucherSelector}
		executed := false
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{OutputType: &outputTypes, Executed: &executed},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Require().Len(outputs, 2)
		s.Equal(uint64(2), total)
		s.Equal(uint64(0), outputs[0].Index)
		s.Equal(uint64(1), outputs[1].Index)

		executed = true
		outputs, total, err = s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{OutputType: &outputTypes, Executed: &executed},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Require().Len(outputs, 1)
		s.Equal(uint64(1), total)
		s.Equal(uint64(3), outputs[0].Index)

		// The validator uses the nil-filter path to reproduce epoch claims;
		// it must continue to include every output type and execution state.
		outputs, total, err = s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{},
			repository.Pagination{}, false)
		s.Require().NoError(err)
		s.Len(outputs, 4)
		s.Equal(uint64(4), total)
	})

	s.Run("FilterByVoucherAddress", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// VoucherAddress filter matches substring(raw_data FROM 17 FOR 20)
		// (the ABI head destination) but only on voucher-typed outputs:
		// bytes 17-36 of other output types are arbitrary payload.
		voucherSelector := []byte{0x23, 0x7a, 0x81, 0x6f}
		delegateCallVoucherSelector := []byte{0x10, 0x32, 0x1e, 0x8b}
		noticeSelector := []byte{0xc2, 0x58, 0xd6, 0xe5}

		voucherAddr := UniqueAddress()
		rawWithVoucher := make([]byte, 64)
		copy(rawWithVoucher[0:4], voucherSelector)
		copy(rawWithVoucher[16:36], voucherAddr.Bytes())

		otherAddr := UniqueAddress()
		rawWithOther := make([]byte, 64)
		copy(rawWithOther[0:4], voucherSelector)
		copy(rawWithOther[16:36], otherAddr.Bytes())

		// A notice whose payload happens to contain the searched address at
		// bytes 17-36 must not match the voucher-address filter.
		rawNoticeLookalike := make([]byte, 64)
		copy(rawNoticeLookalike[0:4], noticeSelector)
		copy(rawNoticeLookalike[16:36], voucherAddr.Bytes())

		// Delegate-call vouchers carry a destination too and must match.
		rawWithDelegateCall := make([]byte, 64)
		copy(rawWithDelegateCall[0:4], delegateCallVoucherSelector)
		copy(rawWithDelegateCall[16:36], voucherAddr.Bytes())

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{rawWithVoucher, rawWithOther, rawNoticeLookalike, rawWithDelegateCall}, nil)

		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{VoucherAddress: &voucherAddr},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 2)
		s.Equal(uint64(2), total)
		s.Equal(rawWithVoucher, outputs[0].RawData)
		s.Equal(rawWithDelegateCall, outputs[1].RawData)
	})

	s.Run("Pagination", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		data := make([][]byte, 5)
		for i := range data {
			data[i] = []byte("output-data")
		}
		s.storeAdvanceResult(seed.App.ID, 0, 0, data, nil)

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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("o0"), []byte("o1"), []byte("o2")}, nil)

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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("output-data")}, nil)

		txHash := UniqueHash()
		out := &Output{
			InputEpochApplicationID:  seed.App.ID,
			Index:                    0,
			ExecutionTransactionHash: &txHash,
		}
		err := s.Repo.UpdateOutputsExecution(
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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("o0"), []byte("o1"), []byte("o2")}, nil)

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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("output-data")}, nil)

		// ExecutionTransactionHash is nil — should fail
		badOutput := &Output{
			InputEpochApplicationID: seed.App.ID,
			Index:                   0,
		}
		err := s.Repo.UpdateOutputsExecution(
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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("o0"), []byte("o1")}, nil)

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

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("o0"), []byte("o1")}, nil)

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

func (s *OutputSuite) TestGetNumberOfExecutedOutputs() {
	s.Run("ReturnsZeroWhenNone", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		count, err := s.Repo.GetNumberOfExecutedOutputs(s.Ctx, app.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), count)
	})

	s.Run("ReturnsCountAfterExecution", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{[]byte("o0"), []byte("o1"), []byte("o2")}, nil)

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

func (s *OutputSuite) TestGetNumberOfPendingExecutableOutputs() {
	s.Run("CountsOnlyUnexecutedVouchers", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			[][]byte{
				{0x23, 0x7a, 0x81, 0x6f, 0x01}, // Voucher
				{0x10, 0x32, 0x1e, 0x8b, 0x02}, // DelegateCallVoucher
				{0xba, 0xad, 0xf0, 0x0d, 0x03}, // non-executable output type
			}, nil)

		count, err := s.Repo.GetNumberOfPendingExecutableOutputs(s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(2), count)

		txHash := UniqueHash()
		err = s.Repo.UpdateOutputsExecution(
			s.Ctx,
			seed.App.IApplicationAddress.String(),
			[]*Output{{
				InputEpochApplicationID:  seed.App.ID,
				Index:                    0,
				ExecutionTransactionHash: &txHash,
			}},
			200,
		)
		s.Require().NoError(err)

		count, err = s.Repo.GetNumberOfPendingExecutableOutputs(s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(1), count)
	})
}

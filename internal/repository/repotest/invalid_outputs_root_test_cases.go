// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

func (s *ApplicationSuite) TestInvalidOutputsRootStatus() {
	const diagnosis = "epoch 0: declared outputs root differs from calculated root"
	for _, current := range model.ApplicationStatusAllValues {
		s.Run("EnterFrom/"+current.String(), func() {
			app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
			var oldReason *string
			if current != model.ApplicationStatus_OK {
				reason := "earlier diagnosis"
				oldReason = &reason
				s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, current, oldReason))
			}
			// The caller still has its original OK object. The stored status,
			// not that stale snapshot, must decide whether this write is valid.
			s.Equal(model.ApplicationStatus_OK, app.Status)
			reason := diagnosis
			err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, model.ApplicationStatus_InvalidOutputsRoot, &reason)
			allowed := current == model.ApplicationStatus_OK || current == model.ApplicationStatus_Failed
			if allowed {
				s.Require().NoError(err)
			} else {
				s.Require().Error(err)
			}
			stored, err := s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			if allowed {
				s.Equal(model.ApplicationStatus_InvalidOutputsRoot, stored.Status)
				s.Equal(&reason, stored.Reason)
			} else {
				s.Equal(current, stored.Status)
				s.Equal(oldReason, stored.Reason)
			}
		})
	}
	for _, target := range model.ApplicationStatusAllValues {
		s.Run("CannotReplaceWith/"+target.String(), func() {
			app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
			reason := diagnosis
			s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, model.ApplicationStatus_InvalidOutputsRoot, &reason))
			newReason := "later finding"
			s.Require().Error(s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, target, &newReason))
			stored, err := s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			s.Equal(model.ApplicationStatus_InvalidOutputsRoot, stored.Status)
			s.Equal(&reason, stored.Reason)
		})
	}
	for _, missing := range []struct {
		name   string
		reason *string
	}{
		{name: "null"},
		{name: "empty", reason: new("")},
	} {
		s.Run("RequiresReason/"+missing.name, func() {
			app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
			err := s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, model.ApplicationStatus_InvalidOutputsRoot, missing.reason)
			s.Require().Error(err)
			stored, err := s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			s.Equal(model.ApplicationStatus_OK, stored.Status)
			s.Nil(stored.Reason)
		})
	}
	s.Run("EnablePreservesDiagnosis", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		reason := diagnosis
		s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, app.ID, model.ApplicationStatus_InvalidOutputsRoot, &reason))
		s.Require().NoError(s.Repo.UpdateApplicationEnabled(s.Ctx, app.ID, false))
		s.Require().NoError(s.Repo.EnableApplicationAndClearFailed(s.Ctx, app.ID))
		stored, err := s.Repo.GetApplication(s.Ctx, app.Name)
		s.Require().NoError(err)
		s.True(stored.Enabled)
		s.Equal(model.ApplicationStatus_InvalidOutputsRoot, stored.Status)
		s.Equal(&reason, stored.Reason)
	})
}

func (s *BulkOperationsSuite) TestInvalidOutputsRootAdvance() {
	const (
		epochLength = 10
		inputBlock  = 5
	)
	for _, consensus := range model.ConsensusAllValues {
		s.Run(consensus.String(), func() {
			app := NewApplicationBuilder().WithConsensus(consensus).Create(s.Ctx, s.T(), s.Repo)
			input := NewInputBuilder().WithIndex(0).WithBlockNumber(inputBlock).Build()
			next := NewInputBuilder().WithIndex(1).WithBlockNumber(inputBlock + 1).Build()
			inputs := []*model.Input{input, next}
			epoch := NewEpochBuilder(app.ID).WithIndex(0).WithStatus(model.EpochStatus_Closed).
				WithBlocks(0, epochLength-1).WithInputBounds(0, uint64(len(inputs))).Build()
			s.Require().NoError(s.Repo.CreateEpochsAndInputs(
				s.Ctx, app.Name, map[*model.Epoch][]*model.Input{epoch: inputs}, epochLength))
			result := &model.AdvanceResult{
				EpochIndex:      epoch.Index,
				InputIndex:      input.Index,
				Status:          model.InputCompletionStatus_InvalidOutputsRoot,
				StateProof:      *DummyStateProof(),
				IsDaveConsensus: consensus == model.Consensus_PRT,
			}
			if result.IsDaveConsensus {
				result.PeriodicStateHashes = [][32]byte{UniqueHash()}
				result.PaddingRepetitions = model.InputHashCollectionCapacity - uint64(len(result.PeriodicStateHashes))
			}
			s.Require().NoError(s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result))
			storedInput, err := s.Repo.GetInput(s.Ctx, app.Name, input.Index)
			s.Require().NoError(err)
			s.Equal(model.InputCompletionStatus_InvalidOutputsRoot, storedInput.Status)
			s.Equal(&result.MachineHash, storedInput.MachineHash)
			s.Equal(&result.TxBufferDataBlock, storedInput.TxBufferDataBlock)
			s.Nil(storedInput.ExceptionData)
			storedEpoch, err := s.Repo.GetEpoch(s.Ctx, app.Name, epoch.Index)
			s.Require().NoError(err)
			proof, err := storedEpoch.StateProof()
			s.Require().NoError(err)
			s.Equal(result.StateProof, proof)
			storedApp, err := s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			s.Equal(uint64(1), storedApp.ProcessedInputs)
			s.Equal(model.ApplicationStatus_InvalidOutputsRoot, storedApp.Status)
			s.Require().NotNil(storedApp.Reason)
			s.Equal("input 0 completed with INVALID_OUTPUTS_ROOT: an accepted yield must declare exactly 32 bytes", *storedApp.Reason)
			summary, err := s.Repo.ReplaySummary(s.Ctx, app.IApplicationAddress, repository.ReplayVerificationFull)
			s.Require().NoError(err)
			s.Equal(uint64(1), summary.ProcessedInputs)
			records, err := s.Repo.ReplayPage(s.Ctx, repository.ReplayPageRequest{
				ApplicationID: app.ID, FromInput: 0, ToInputExclusive: 1, Limit: 1,
				Verification: repository.ReplayVerificationFull,
			})
			s.Require().NoError(err)
			s.Require().Len(records, 1)
			s.Equal(result.Status, records[0].Input.Status)
			s.Equal(&result.MachineHash, records[0].Input.MachineHash)
			s.Equal(&result.TxBufferDataBlock, records[0].Input.TxBufferDataBlock)
			if result.IsDaveConsensus {
				hashes, total, err := s.Repo.ListStateHashes(s.Ctx, app.Name,
					repository.StateHashFilter{EpochIndex: &epoch.Index}, repository.Pagination{}, false)
				s.Require().NoError(err)
				expectedHashRows := len(result.PeriodicStateHashes) + 1
				s.Equal(uint64(expectedHashRows), total)
				s.Require().Len(hashes, expectedHashRows)
				s.Equal(common.Hash(result.PeriodicStateHashes[0]), hashes[0].MachineHash)
				s.Equal(uint64(1), hashes[0].Repetitions)
				s.Equal(result.MachineHash, hashes[1].MachineHash)
				s.Equal(result.PaddingRepetitions, hashes[1].Repetitions)
				s.Equal([]model.ReplayStateHash{
					{Index: 0, MachineHash: common.Hash(result.PeriodicStateHashes[0]), Repetitions: 1},
					{Index: 1, MachineHash: result.MachineHash, Repetitions: result.PaddingRepetitions},
				}, records[0].StateHashes)
			} else {
				s.Empty(records[0].StateHashes)
			}
			// A repeated save cannot consume the input twice, and a stale
			// writer cannot append an accepted result for its successor.
			s.Require().ErrorIs(s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result), repository.ErrAdvanceAfterTerminal)
			result.InputIndex = next.Index
			result.Status = model.InputCompletionStatus_Accepted
			result.Outputs = [][]byte{[]byte("must not be stored")}
			s.Require().ErrorIs(s.Repo.StoreAdvanceResult(s.Ctx, app.ID, result), repository.ErrAdvanceAfterTerminal)
			pending, err := s.Repo.GetInput(s.Ctx, app.Name, next.Index)
			s.Require().NoError(err)
			s.Equal(model.InputCompletionStatus_None, pending.Status)
			outputs, total, err := s.Repo.ListOutputs(s.Ctx, app.Name, repository.OutputFilter{}, repository.Pagination{}, false)
			s.Require().NoError(err)
			s.Empty(outputs)
			s.Zero(total)
			storedApp, err = s.Repo.GetApplication(s.Ctx, app.Name)
			s.Require().NoError(err)
			s.Equal(uint64(1), storedApp.ProcessedInputs)
		})
	}
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_Failed, model.ApplicationStatus_InvalidOutputsRoot} {
		s.Run("RejectsStaleResult/"+status.String(), func() {
			seed := Seed(s.Ctx, s.T(), s.Repo)
			reason := "another service stopped the application"
			s.Require().NoError(s.Repo.UpdateApplicationStatus(s.Ctx, seed.App.ID, status, &reason))
			err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, &model.AdvanceResult{
				EpochIndex: seed.Epoch.Index,
				InputIndex: seed.Input.Index,
				Status:     model.InputCompletionStatus_InvalidOutputsRoot,
				StateProof: *DummyStateProof(),
			})
			s.Require().ErrorIs(err, repository.ErrApplicationNotRunnable)
			input, err := s.Repo.GetInput(s.Ctx, seed.App.Name, seed.Input.Index)
			s.Require().NoError(err)
			s.Equal(model.InputCompletionStatus_None, input.Status)
			s.Nil(input.MachineHash)
			epoch, err := s.Repo.GetEpoch(s.Ctx, seed.App.Name, seed.Epoch.Index)
			s.Require().NoError(err)
			s.False(epoch.HasCompleteStateProof())
			app, err := s.Repo.GetApplication(s.Ctx, seed.App.Name)
			s.Require().NoError(err)
			s.Zero(app.ProcessedInputs)
			s.Equal(status, app.Status)
			s.Equal(&reason, app.Reason)
		})
	}
}

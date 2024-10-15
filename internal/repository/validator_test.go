// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

func (s *RepositorySuite) TestGetOutputsProducedInBlockRange() {
	// get outputs from the existing app
	outputs, err := s.database.GetOutputsProducedInBlockRange(
		s.ctx,
		common.HexToAddress("deadbeef"),
		1, 3,
	)
	s.Require().Nil(err)
	s.Require().Len(outputs, 3)

	// add an output from another app
	app := Application{
		ContractAddress: common.HexToAddress("deadbeee"),
		TemplateHash:    common.BytesToHash([]byte("template")),
		Status:          ApplicationStatusRunning,
	}
	_, err = s.database.InsertApplication(s.ctx, &app)
	s.Require().Nil(err)

	epoch := Epoch{
		Index:      0,
		AppAddress: app.ContractAddress,
		FirstBlock: 0,
		LastBlock:  99,
		Status:     EpochStatusProcessedAllInputs,
	}
	epoch.Id, err = s.database.InsertEpoch(s.ctx, &epoch)
	s.Require().Nil(err)

	expectedHash := common.BytesToHash([]byte("outputs hash"))
	input := Input{
		Index:            0,
		CompletionStatus: InputStatusAccepted,
		BlockNumber:      1,
		OutputsHash:      &expectedHash,
		RawData:          []byte("data"),
		AppAddress:       app.ContractAddress,
		EpochId:          epoch.Id,
	}
	input.Id, err = s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	newOutput := &Output{
		Index:   0,
		RawData: []byte("data"),
		InputId: input.Id,
	}
	newOutput.Id, err = s.database.InsertOutput(s.ctx, newOutput)
	s.Require().Nil(err)

	// the output from the other application is not considered
	outputs, err = s.database.GetOutputsProducedInBlockRange(
		s.ctx,
		common.HexToAddress("deadbeef"),
		1, 3,
	)
	s.Require().Nil(err)
	s.Require().Len(outputs, 3)
	for _, output := range outputs {
		s.NotEqual(newOutput.Id, output.Id)
	}
}

func (s *RepositorySuite) TestGetProcessedEpochs() {
	app := Application{
		ContractAddress: common.HexToAddress("deadbeed"),
		TemplateHash:    common.BytesToHash([]byte("template")),
		Status:          ApplicationStatusRunning,
	}
	_, err := s.database.InsertApplication(s.ctx, &app)
	s.Require().Nil(err)

	// no epochs, should return nothing
	epochs, err := s.database.GetProcessedEpochs(s.ctx, app.ContractAddress)
	s.Require().Nil(err)
	s.Len(epochs, 0)

	epoch := Epoch{
		AppAddress: app.ContractAddress,
		Index:      0,
		FirstBlock: 0,
		LastBlock:  99,
		Status:     EpochStatusOpen,
	}
	epoch.Id, err = s.database.InsertEpoch(s.ctx, &epoch)
	s.Require().Nil(err)

	// a single non-processed epoch, should return nothing
	epochs, err = s.database.GetProcessedEpochs(s.ctx, app.ContractAddress)
	s.Require().Nil(err)
	s.Len(epochs, 0)

	epoch2 := Epoch{
		AppAddress: app.ContractAddress,
		Index:      1,
		FirstBlock: 100,
		LastBlock:  199,
		Status:     EpochStatusProcessedAllInputs,
	}
	epoch2.Id, err = s.database.InsertEpoch(s.ctx, &epoch2)
	s.Require().Nil(err)

	// should return the processed epoch
	epochs, err = s.database.GetProcessedEpochs(s.ctx, app.ContractAddress)
	s.Require().Nil(err)
	s.Len(epochs, 1)
	s.Contains(epochs, epoch2)
}

func (s *RepositorySuite) TestGetLastInputOutputHash() {
	app := Application{
		ContractAddress: common.HexToAddress("deadbeec"),
		TemplateHash:    common.BytesToHash([]byte("template")),
		Status:          ApplicationStatusRunning,
	}
	_, err := s.database.InsertApplication(s.ctx, &app)
	s.Require().Nil(err)

	epoch := Epoch{
		Index:      0,
		AppAddress: app.ContractAddress,
		FirstBlock: 0,
		LastBlock:  99,
		Status:     EpochStatusOpen,
	}
	epoch.Id, err = s.database.InsertEpoch(s.ctx, &epoch)
	s.Require().Nil(err)

	// should fail
	hash, err := s.database.GetLastInputOutputsHash(s.ctx, epoch.Index, epoch.AppAddress)
	s.Require().NotNil(err)
	s.Nil(hash)
	s.ErrorContains(err, "still being processed")

	epoch2 := Epoch{
		Index:      1,
		AppAddress: app.ContractAddress,
		FirstBlock: 100,
		LastBlock:  199,
		Status:     EpochStatusClosed,
	}
	epoch2.Id, err = s.database.InsertEpoch(s.ctx, &epoch2)
	s.Require().Nil(err)

	// should fail
	hash, err = s.database.GetLastInputOutputsHash(s.ctx, epoch2.Index, epoch2.AppAddress)
	s.Require().NotNil(err)
	s.Nil(hash)
	s.ErrorContains(err, "still being processed")

	epoch3 := Epoch{
		Index:      2,
		AppAddress: app.ContractAddress,
		FirstBlock: 200,
		LastBlock:  299,
		Status:     EpochStatusProcessedAllInputs,
	}
	epoch3.Id, err = s.database.InsertEpoch(s.ctx, &epoch3)
	s.Require().Nil(err)

	expectedHash := common.BytesToHash([]byte("outputs hash"))
	input := &Input{
		Index:            0,
		CompletionStatus: InputStatusAccepted,
		BlockNumber:      1,
		OutputsHash:      &expectedHash,
		RawData:          []byte("data"),
		AppAddress:       app.ContractAddress,
		EpochId:          epoch3.Id,
	}
	input.Id, err = s.database.InsertInput(s.ctx, input)
	s.Require().Nil(err)

	hash, err = s.database.GetLastInputOutputsHash(s.ctx, epoch3.Index, epoch3.AppAddress)
	s.Require().Nil(err)
	s.Require().NotNil(hash)
	s.Equal(expectedHash, *hash)
}

func (s *RepositorySuite) TestGetPreviousEpoch() {
	app := &Application{
		ContractAddress: common.HexToAddress("deadbeeb"),
		TemplateHash:    common.BytesToHash([]byte("template")),
		Status:          ApplicationStatusRunning,
	}
	_, err := s.database.InsertApplication(s.ctx, app)
	s.Require().Nil(err)

	epoch := Epoch{
		Index:      0,
		AppAddress: app.ContractAddress,
		FirstBlock: 0,
		LastBlock:  99,
		Status:     EpochStatusClaimAccepted,
	}
	epoch.Id, err = s.database.InsertEpoch(s.ctx, &epoch)
	s.Require().Nil(err)

	// first epoch, should return nil
	previousEpoch, err := s.database.GetPreviousEpoch(s.ctx, epoch)
	s.Require().Nil(previousEpoch)
	s.Require().Nil(err)

	epoch2 := Epoch{
		Index:      1,
		AppAddress: app.ContractAddress,
		FirstBlock: 100,
		LastBlock:  199,
		Status:     EpochStatusClaimAccepted,
	}
	epoch2.Id, err = s.database.InsertEpoch(s.ctx, &epoch2)
	s.Require().Nil(err)

	// second epoch, should return first
	previousEpoch, err = s.database.GetPreviousEpoch(s.ctx, epoch2)
	s.Require().Nil(err)
	s.Require().NotNil(previousEpoch)
	s.Require().Equal(previousEpoch.Id, epoch.Id)
}

func (s *RepositorySuite) TestSetEpochClaimAndInsertProofsTransaction() {
	app := Application{
		ContractAddress: common.HexToAddress("deadbeea"),
		TemplateHash:    common.BytesToHash([]byte("template")),
		Status:          ApplicationStatusRunning,
	}
	_, err := s.database.InsertApplication(s.ctx, &app)
	s.Require().Nil(err)

	epoch := Epoch{
		Index:      0,
		AppAddress: app.ContractAddress,
		FirstBlock: 0,
		LastBlock:  99,
		Status:     EpochStatusProcessedAllInputs,
	}
	epoch.Id, err = s.database.InsertEpoch(s.ctx, &epoch)
	s.Require().Nil(err)

	input := Input{
		Index:            0,
		CompletionStatus: InputStatusAccepted,
		BlockNumber:      1,
		RawData:          []byte("data"),
		AppAddress:       app.ContractAddress,
		EpochId:          epoch.Id,
	}
	input.Id, err = s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	output1 := Output{
		Index:   100,
		RawData: []byte("data"),
		InputId: input.Id,
	}
	output1.Id, err = s.database.InsertOutput(s.ctx, &output1)
	s.Require().Nil(err)

	output2 := Output{
		Index:   101,
		RawData: []byte("data"),
		InputId: input.Id,
	}
	output2.Id, err = s.database.InsertOutput(s.ctx, &output2)
	s.Require().Nil(err)

	expectedClaim := common.BytesToHash([]byte("claim"))
	epoch.ClaimHash = &expectedClaim
	epoch.Status = EpochStatusClaimComputed
	expectedSiblings1 := []Hash{{}, {}}
	expectedHash1 := common.BytesToHash([]byte("output"))
	output1.Hash = &expectedHash1
	output1.OutputHashesSiblings = expectedSiblings1
	expectedHash2 := common.BytesToHash([]byte("output"))
	expectedSiblings2 := []Hash{{}, {}, {}}
	output2.Hash = &expectedHash2
	output2.OutputHashesSiblings = expectedSiblings2

	err = s.database.SetEpochClaimAndInsertProofsTransaction(
		s.ctx,
		epoch,
		[]Output{output1, output2},
	)
	s.Require().Nil(err)

	updatedClaim, err := s.database.GetEpoch(s.ctx, 0, epoch.AppAddress)
	s.Require().Nil(err)
	s.Require().NotNil(updatedClaim)
	s.Require().NotNil(updatedClaim.ClaimHash)
	s.Equal(expectedClaim, *updatedClaim.ClaimHash)
	s.Equal(EpochStatusClaimComputed, updatedClaim.Status)

	updatedOutput1, err := s.database.GetOutput(s.ctx, 100, input.AppAddress)
	s.Require().Nil(err)
	s.Require().NotNil(updatedOutput1)
	s.Require().NotNil(updatedOutput1.Hash)
	s.Equal(expectedHash1, *updatedOutput1.Hash)
	s.Equal(expectedSiblings1, updatedOutput1.OutputHashesSiblings)

	updatedOutput2, err := s.database.GetOutput(s.ctx, 101, input.AppAddress)
	s.Require().Nil(err)
	s.Require().NotNil(updatedOutput2)
	s.Require().NotNil(updatedOutput2.Hash)
	s.Equal(expectedHash2, *updatedOutput2.Hash)
	s.Equal(expectedSiblings2, updatedOutput2.OutputHashesSiblings)
}

func (s *RepositorySuite) TestSetEpochClaimAndInsertProofsTransactionRollback() {
	app := Application{
		ContractAddress: common.HexToAddress("deadbeff"),
		TemplateHash:    common.BytesToHash([]byte("template")),
		Status:          ApplicationStatusRunning,
	}
	_, err := s.database.InsertApplication(s.ctx, &app)
	s.Require().Nil(err)

	epoch := Epoch{
		Index:      0,
		AppAddress: app.ContractAddress,
		FirstBlock: 0,
		LastBlock:  99,
		Status:     EpochStatusProcessedAllInputs,
	}
	epoch.Id, err = s.database.InsertEpoch(s.ctx, &epoch)
	s.Require().Nil(err)

	input := Input{
		Index:            0,
		CompletionStatus: InputStatusAccepted,
		BlockNumber:      1,
		RawData:          []byte("data"),
		AppAddress:       app.ContractAddress,
		EpochId:          epoch.Id,
	}
	input.Id, err = s.database.InsertInput(s.ctx, &input)
	s.Require().Nil(err)

	output1 := Output{
		Index:   102,
		RawData: []byte("data"),
		InputId: input.Id,
	}
	output1.Id, err = s.database.InsertOutput(s.ctx, &output1)
	s.Require().Nil(err)

	output1.Id = 978233982398 // non-existing id
	claim := common.BytesToHash([]byte("claim"))
	epoch.ClaimHash = &claim
	epoch.Status = EpochStatusClaimComputed

	err = s.database.SetEpochClaimAndInsertProofsTransaction(
		s.ctx,
		epoch,
		[]Output{output1},
	)
	s.Require().NotNil(err)
	s.ErrorContains(err, "No rows affected")

	nonUpdatedEpoch, err := s.database.GetEpoch(s.ctx, epoch.Index, epoch.AppAddress)
	s.Require().Nil(err)
	s.Require().NotNil(nonUpdatedEpoch)
	s.Nil(nonUpdatedEpoch.ClaimHash)
	s.Equal(EpochStatusProcessedAllInputs, nonUpdatedEpoch.Status)
}

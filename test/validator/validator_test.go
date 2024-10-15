// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 300 * time.Second

type ValidatorRepositoryIntegrationSuite struct {
	suite.Suite
	ctx              context.Context
	cancel           context.CancelFunc
	validator        *validator.Validator
	database         *repository.Database
	postgresEndpoint string
}

func TestValidatorRepositoryIntegration(t *testing.T) {
	suite.Run(t, new(ValidatorRepositoryIntegrationSuite))
}

func (s *ValidatorRepositoryIntegrationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	// build database URL
	s.postgresEndpoint, err = db.GetPostgresTestEndpoint()
	s.Require().Nil(err)

	err = db.SetupTestPostgres(s.postgresEndpoint)
	s.Require().Nil(err)
}

func (s *ValidatorRepositoryIntegrationSuite) SetupSubTest() {
	var err error
	s.database, err = repository.Connect(s.ctx, s.postgresEndpoint)
	s.Require().Nil(err)

	s.validator = validator.NewValidator(s.database, 0)

	err = db.SetupTestPostgres(s.postgresEndpoint)
	s.Require().Nil(err)
}

func (s *ValidatorRepositoryIntegrationSuite) TearDownSubTest() {
	s.validator = nil

	s.database.Close()
}

func (s *ValidatorRepositoryIntegrationSuite) TearDownSuite() {
	// TODO reset database and anvil
	s.cancel()
}

func (s *ValidatorRepositoryIntegrationSuite) TestItReturnsPristineClaim() {
	s.Run("WhenThereAreNoOutputsAndNoPreviousEpoch", func() {
		app := &model.Application{
			ContractAddress: common.BytesToAddress([]byte("deadbeef")),
			Status:          model.ApplicationStatusRunning,
		}
		_, err := s.database.InsertApplication(s.ctx, app)
		s.Require().Nil(err)

		epoch := &model.Epoch{
			AppAddress: app.ContractAddress,
			Status:     model.EpochStatusProcessedAllInputs,
			FirstBlock: 0,
			LastBlock:  9,
		}
		epoch.Id, err = s.database.InsertEpoch(s.ctx, epoch)
		s.Require().Nil(err)

		// if there are no outputs and no previous claim,
		// a pristine claim is expected with no proofs
		expectedClaim, _, err := merkle.CreateProofs(nil, validator.MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)

		input := &model.Input{
			AppAddress:       app.ContractAddress,
			EpochId:          epoch.Id,
			BlockNumber:      9,
			RawData:          []byte("data"),
			OutputsHash:      &expectedClaim,
			CompletionStatus: model.InputStatusAccepted,
		}
		input.Id, err = s.database.InsertInput(s.ctx, input)
		s.Require().Nil(err)

		err = s.validator.Run(s.ctx)
		s.Require().Nil(err)

		updatedEpoch, err := s.database.GetEpoch(s.ctx, epoch.Index, epoch.AppAddress)
		s.Require().Nil(err)
		s.Require().NotNil(updatedEpoch)
		s.Require().NotNil(updatedEpoch.ClaimHash)

		// epoch status was updated
		s.Equal(model.EpochStatusClaimComputed, updatedEpoch.Status)
		// claim is pristine claim
		s.Equal(expectedClaim, *updatedEpoch.ClaimHash)
	})
}

func (s *ValidatorRepositoryIntegrationSuite) TestItReturnsPreviousClaim() {
	s.Run("WhenThereAreNoOutputsAndThereIsAPreviousEpoch", func() {
		app := &model.Application{
			ContractAddress: common.BytesToAddress([]byte("deadbeef")),
			Status:          model.ApplicationStatusRunning,
		}
		_, err := s.database.InsertApplication(s.ctx, app)
		s.Require().Nil(err)

		// insert the first epoch with a claim
		firstEpochClaim := common.BytesToHash([]byte("claim"))
		firstEpoch := &model.Epoch{
			AppAddress: app.ContractAddress,
			Status:     model.EpochStatusClaimComputed,
			ClaimHash:  &firstEpochClaim,
			FirstBlock: 0,
			LastBlock:  9,
		}
		firstEpoch.Id, err = s.database.InsertEpoch(s.ctx, firstEpoch)
		s.Require().Nil(err)

		// we add an input to the epoch because they must have at least one and
		// because without it the claim hash check will fail
		firstEpochInput := &model.Input{
			AppAddress:       app.ContractAddress,
			EpochId:          firstEpoch.Id,
			BlockNumber:      9,
			RawData:          []byte("data"),
			OutputsHash:      &firstEpochClaim,
			CompletionStatus: model.InputStatusAccepted,
		}
		firstEpochInput.Id, err = s.database.InsertInput(s.ctx, firstEpochInput)
		s.Require().Nil(err)

		// create the second epoch with no outputs
		secondEpoch := &model.Epoch{
			Index:      1,
			AppAddress: app.ContractAddress,
			Status:     model.EpochStatusProcessedAllInputs,
			FirstBlock: 10,
			LastBlock:  19,
		}
		secondEpoch.Id, err = s.database.InsertEpoch(s.ctx, secondEpoch)
		s.Require().Nil(err)

		secondEpochInput := &model.Input{
			Index:       1,
			AppAddress:  app.ContractAddress,
			EpochId:     secondEpoch.Id,
			BlockNumber: 19,
			RawData:     []byte("data2"),
			// since there are no new outputs in the second epoch,
			// the machine OutputsHash will remain the same
			OutputsHash:      &firstEpochClaim,
			CompletionStatus: model.InputStatusAccepted,
		}
		secondEpochInput.Id, err = s.database.InsertInput(s.ctx, secondEpochInput)
		s.Require().Nil(err)

		err = s.validator.Run(s.ctx)
		s.Require().Nil(err)

		updatedEpoch, err := s.database.GetEpoch(s.ctx, secondEpoch.Index, secondEpoch.AppAddress)
		s.Require().Nil(err)
		s.Require().NotNil(updatedEpoch)
		s.Require().NotNil(updatedEpoch.ClaimHash)

		// epoch status was updated
		s.Equal(model.EpochStatusClaimComputed, updatedEpoch.Status)
		// claim is the same from previous epoch
		s.Equal(firstEpochClaim, *updatedEpoch.ClaimHash)
	})
}

func (s *ValidatorRepositoryIntegrationSuite) TestItReturnsANewClaimAndProofs() {
	s.Run("WhenThereAreOutputsAndNoPreviousEpoch", func() {
		app := &model.Application{
			ContractAddress: common.BytesToAddress([]byte("deadbeef")),
			Status:          model.ApplicationStatusRunning,
		}
		_, err := s.database.InsertApplication(s.ctx, app)
		s.Require().Nil(err)

		epoch := &model.Epoch{
			AppAddress: app.ContractAddress,
			Status:     model.EpochStatusProcessedAllInputs,
			FirstBlock: 0,
			LastBlock:  9,
		}
		epoch.Id, err = s.database.InsertEpoch(s.ctx, epoch)
		s.Require().Nil(err)

		input := &model.Input{
			AppAddress:       app.ContractAddress,
			EpochId:          epoch.Id,
			BlockNumber:      9,
			RawData:          []byte("data"),
			CompletionStatus: model.InputStatusAccepted,
		}

		outputRawData := []byte("output")
		output := model.Output{RawData: outputRawData}

		// calculate the expected claim and proofs
		expectedOutputHash := crypto.Keccak256Hash(outputRawData)
		expectedClaim, expectedProofs, err := merkle.CreateProofs(
			[]model.Hash{expectedOutputHash},
			validator.MAX_OUTPUT_TREE_HEIGHT,
		)
		s.Require().Nil(err)
		s.Require().NotNil(expectedClaim)
		s.Require().NotNil(expectedProofs)

		// update the input with its OutputsHash and insert it in the db
		input.OutputsHash = &expectedClaim
		input.Id, err = s.database.InsertInput(s.ctx, input)
		s.Require().Nil(err)

		// update the output with its input id and insert it in the db
		output.InputId = input.Id
		output.Id, err = s.database.InsertOutput(s.ctx, &output)
		s.Require().Nil(err)

		err = s.validator.Run(s.ctx)
		s.Require().Nil(err)

		updatedEpoch, err := s.database.GetEpoch(s.ctx, epoch.Index, epoch.AppAddress)
		s.Require().Nil(err)
		s.Require().NotNil(updatedEpoch)
		s.Require().NotNil(updatedEpoch.ClaimHash)

		// epoch status was updated
		s.Equal(model.EpochStatusClaimComputed, updatedEpoch.Status)
		// claim is the expected new claim
		s.Equal(expectedClaim, *updatedEpoch.ClaimHash)

		updatedOutput, err := s.database.GetOutput(s.ctx, output.Index, app.ContractAddress)
		s.Require().Nil(err)
		s.Require().NotNil(updatedOutput)
		s.Require().NotNil(updatedOutput.Hash)

		// output was updated with its hash
		s.Equal(expectedOutputHash, *updatedOutput.Hash)
		// output has proof
		s.Len(updatedOutput.OutputHashesSiblings, validator.MAX_OUTPUT_TREE_HEIGHT)
	})

	s.Run("WhenThereAreOutputsAndAPreviousEpoch", func() {
		app := &model.Application{
			ContractAddress: common.BytesToAddress([]byte("deadbeef")),
			Status:          model.ApplicationStatusRunning,
		}
		_, err := s.database.InsertApplication(s.ctx, app)
		s.Require().Nil(err)

		firstEpoch := &model.Epoch{
			Index:      0,
			AppAddress: app.ContractAddress,
			Status:     model.EpochStatusClaimComputed,
			FirstBlock: 0,
			LastBlock:  9,
		}

		firstInput := &model.Input{
			AppAddress:       app.ContractAddress,
			EpochId:          firstEpoch.Id,
			BlockNumber:      9,
			RawData:          []byte("data"),
			CompletionStatus: model.InputStatusAccepted,
		}

		firstOutputData := []byte("output1")
		firstOutputHash := crypto.Keccak256Hash(firstOutputData)
		firstOutput := model.Output{
			RawData: firstOutputData,
			Hash:    &firstOutputHash,
		}

		// calculate first epoch claim
		firstEpochClaim, firstEpochProofs, err := merkle.CreateProofs(
			[]model.Hash{firstOutputHash},
			validator.MAX_OUTPUT_TREE_HEIGHT,
		)
		s.Require().Nil(err)
		s.Require().NotNil(firstEpochClaim)

		// update epoch with its claim and insert it in the db
		firstEpoch.ClaimHash = &firstEpochClaim
		firstEpoch.Id, err = s.database.InsertEpoch(s.ctx, firstEpoch)
		s.Require().Nil(err)

		// update input with its epoch id and OutputsHash and insert it in the db
		firstInput.EpochId = firstEpoch.Id
		firstInput.OutputsHash = &firstEpochClaim
		firstInput.Id, err = s.database.InsertInput(s.ctx, firstInput)
		s.Require().Nil(err)

		// update output with its input id and insert it in the database
		firstOutput.InputId = firstInput.Id
		firstOutput.OutputHashesSiblings = firstEpochProofs
		firstOutput.Id, err = s.database.InsertOutput(s.ctx, &firstOutput)
		s.Require().Nil(err)

		// setup second epoch
		secondEpoch := &model.Epoch{
			Index:      1,
			AppAddress: app.ContractAddress,
			Status:     model.EpochStatusProcessedAllInputs,
			FirstBlock: 10,
			LastBlock:  19,
		}
		secondEpoch.Id, err = s.database.InsertEpoch(s.ctx, secondEpoch)
		s.Require().Nil(err)

		secondInput := &model.Input{
			Index:            1,
			AppAddress:       app.ContractAddress,
			EpochId:          secondEpoch.Id,
			BlockNumber:      19,
			RawData:          []byte("data2"),
			CompletionStatus: model.InputStatusAccepted,
		}

		secondOutputData := []byte("output2")
		secondOutput := model.Output{
			Index:   1,
			RawData: secondOutputData,
		}

		// calculate the expected claim
		secondOutputHash := crypto.Keccak256Hash(secondOutputData)
		expectedEpochClaim, expectedProofs, err := merkle.CreateProofs(
			[]model.Hash{firstOutputHash, secondOutputHash},
			validator.MAX_OUTPUT_TREE_HEIGHT,
		)
		s.Require().Nil(err)
		s.Require().NotNil(expectedEpochClaim)
		s.Require().NotNil(expectedProofs)

		// update second input with its OutputsHash and insert it in the db
		secondInput.OutputsHash = &expectedEpochClaim
		secondInput.Id, err = s.database.InsertInput(s.ctx, secondInput)
		s.Require().Nil(err)

		// update second output with its input id and insert it in the database
		secondOutput.InputId = secondInput.Id
		secondOutput.Id, err = s.database.InsertOutput(s.ctx, &secondOutput)
		s.Require().Nil(err)

		err = s.validator.Run(s.ctx)
		s.Require().Nil(err)

		updatedSecondEpoch, err := s.database.GetEpoch(
			s.ctx,
			secondEpoch.Index,
			secondEpoch.AppAddress,
		)
		s.Require().Nil(err)
		s.Require().NotNil(updatedSecondEpoch)
		s.Require().NotNil(updatedSecondEpoch.ClaimHash)

		// assert epoch status was changed
		s.Equal(model.EpochStatusClaimComputed, updatedSecondEpoch.Status)
		// assert second epoch claim is a new claim
		s.NotEqual(firstEpochClaim, *updatedSecondEpoch.ClaimHash)
		s.Equal(expectedEpochClaim, *updatedSecondEpoch.ClaimHash)

		updatedSecondOutput, err := s.database.GetOutput(
			s.ctx,
			secondOutput.Index,
			app.ContractAddress,
		)
		s.Require().Nil(err)
		s.Require().NotNil(updatedSecondOutput)
		s.Require().NotNil(updatedSecondOutput.Hash)

		// assert output hash was updated
		s.Equal(secondOutputHash, *updatedSecondOutput.Hash)
		// assert output has proof
		s.Len(updatedSecondOutput.OutputHashesSiblings, validator.MAX_OUTPUT_TREE_HEIGHT)
	})
}

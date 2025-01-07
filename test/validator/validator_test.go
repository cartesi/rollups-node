// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/cartesi/rollups-node/pkg/service"
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
	validator        validator.Service
	repository       repository.Repository
	postgresEndpoint config.Redacted[string]
}

func TestValidatorRepositoryIntegration(t *testing.T) {
	suite.Run(t, new(ValidatorRepositoryIntegrationSuite))
}

func (s *ValidatorRepositoryIntegrationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)

	var err error
	// build database URL
	s.postgresEndpoint.Value, err = db.GetPostgresTestEndpoint()
	s.Require().Nil(err)

	err = db.SetupTestPostgres(s.postgresEndpoint.Value)
	s.Require().Nil(err)
}

func (s *ValidatorRepositoryIntegrationSuite) SetupSubTest() {
	var err error
	s.repository, err = factory.NewRepositoryFromConnectionString(s.ctx, s.postgresEndpoint.Value)
	s.Require().Nil(err)

	err = db.SetupTestPostgres(s.postgresEndpoint.Value)
	s.Require().Nil(err)

	c := validator.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:     "validator",
			Impl:     &s.validator,
			LogLevel: service.LogLevel(slog.LevelDebug),
		},
		Repository:     s.repository,
		MaxStartupTime: 1 * time.Second,
	}
	s.Require().Nil(validator.Create(&c, &s.validator))
}

func (s *ValidatorRepositoryIntegrationSuite) TearDownSubTest() {
	// FIXME add close method
	//s.repository.Close()
}

func (s *ValidatorRepositoryIntegrationSuite) TearDownSuite() {
	// TODO reset database and anvil
	s.cancel()
}

func (s *ValidatorRepositoryIntegrationSuite) TestItReturnsPristineClaim() {
	s.Run("WhenThereAreNoOutputsAndNoPreviousEpoch", func() {
		app := &model.Application{
			Name:                "test-app",
			IApplicationAddress: common.BytesToAddress([]byte("deadbeef")),
			IConsensusAddress:   common.BytesToAddress([]byte("beadbeef")),
			TemplateHash:        common.BytesToHash([]byte("template")),
			TemplateURI:         "/template/path",
			EpochLength:         10,
			State:               model.ApplicationState_Enabled,
		}
		_, err := s.repository.CreateApplication(s.ctx, app)
		s.Require().Nil(err)

		epoch := model.Epoch{
			ApplicationID: 1,
			Index:         0,
			VirtualIndex:  0,
			Status:        model.EpochStatus_InputsProcessed,
			FirstBlock:    0,
			LastBlock:     9,
		}

		// if there are no outputs and no previous claim,
		// a pristine claim is expected with no proofs
		expectedClaim, _, err := merkle.CreateProofs(nil, validator.MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)

		input := model.Input{
			Index:                0,
			BlockNumber:          9,
			RawData:              []byte("data"),
			Status:               model.InputCompletionStatus_Accepted,
			TransactionReference: common.BigToHash(big.NewInt(0)),
		}

		var epochInputMap = make(map[*model.Epoch][]*model.Input)
		epochInputMap[&epoch] = []*model.Input{&input}
		err = s.repository.CreateEpochsAndInputs(s.ctx, app.IApplicationAddress.String(), epochInputMap, 10)
		s.Require().Nil(err)

		// Store the input advance result
		machinehash1 := crypto.Keccak256Hash([]byte("machine-hash1"))
		advanceResult := model.AdvanceResult{
			InputIndex:  input.Index,
			Status:      model.InputCompletionStatus_Accepted,
			OutputsHash: expectedClaim,
			MachineHash: &machinehash1,
		}
		err = s.repository.StoreAdvanceResult(s.ctx, 1, &advanceResult)
		s.Require().Nil(err)

		err = s.validator.Run(s.ctx)
		s.Require().Nil(err)

		updatedEpoch, err := s.repository.GetEpoch(s.ctx, app.IApplicationAddress.String(), epoch.Index)
		s.Require().Nil(err)
		s.Require().NotNil(updatedEpoch)
		s.Require().NotNil(updatedEpoch.ClaimHash)

		// epoch status was updated
		s.Equal(model.EpochStatus_ClaimComputed, updatedEpoch.Status)
		// claim is pristine claim
		s.Equal(expectedClaim, *updatedEpoch.ClaimHash)
	})
}

func (s *ValidatorRepositoryIntegrationSuite) TestItReturnsPreviousClaim() {
	s.Run("WhenThereAreNoOutputsAndThereIsAPreviousEpoch", func() {
		app := &model.Application{
			Name:                "test-app",
			IApplicationAddress: common.BytesToAddress([]byte("deadbeef")),
			IConsensusAddress:   common.BytesToAddress([]byte("beadbeef")),
			TemplateHash:        common.BytesToHash([]byte("template")),
			TemplateURI:         "/template/path",
			EpochLength:         10,
			State:               model.ApplicationState_Enabled,
		}
		_, err := s.repository.CreateApplication(s.ctx, app)
		s.Require().Nil(err)

		// insert the first epoch with a claim
		firstEpochClaim := common.BytesToHash([]byte("claim"))
		firstEpoch := model.Epoch{
			ApplicationID: 1,
			Index:         0,
			VirtualIndex:  0,
			Status:        model.EpochStatus_ClaimComputed,
			ClaimHash:     &firstEpochClaim,
			FirstBlock:    0,
			LastBlock:     9,
		}

		// we add an input to the epoch because they must have at least one and
		// because without it the claim hash check will fail
		firstEpochInput := model.Input{
			Index:                0,
			BlockNumber:          9,
			RawData:              []byte("data"),
			Status:               model.InputCompletionStatus_Accepted,
			TransactionReference: common.BigToHash(big.NewInt(0)),
		}

		// create the second epoch with no outputs
		secondEpoch := model.Epoch{
			ApplicationID: 1,
			Index:         1,
			VirtualIndex:  1,
			Status:        model.EpochStatus_InputsProcessed,
			FirstBlock:    10,
			LastBlock:     19,
		}

		secondEpochInput := model.Input{
			Index:                1,
			BlockNumber:          19,
			RawData:              []byte("data2"),
			Status:               model.InputCompletionStatus_Accepted,
			TransactionReference: common.BigToHash(big.NewInt(1)),
		}

		var epochInputMap = make(map[*model.Epoch][]*model.Input)
		epochInputMap[&firstEpoch] = []*model.Input{&firstEpochInput}
		epochInputMap[&secondEpoch] = []*model.Input{&secondEpochInput}
		err = s.repository.CreateEpochsAndInputs(s.ctx, app.IApplicationAddress.String(), epochInputMap, 20)
		s.Require().Nil(err)

		// Store the input advance result
		machinehash1 := crypto.Keccak256Hash([]byte("machine-hash1"))
		advanceResult := model.AdvanceResult{
			InputIndex:  firstEpochInput.Index,
			Status:      model.InputCompletionStatus_Accepted,
			OutputsHash: firstEpochClaim,
			MachineHash: &machinehash1,
		}
		err = s.repository.StoreAdvanceResult(s.ctx, 1, &advanceResult)
		s.Require().Nil(err)

		err = s.repository.StoreClaimAndProofs(s.ctx, &firstEpoch, []*model.Output{})
		s.Require().Nil(err)

		// Store the input advance result
		machinehash2 := crypto.Keccak256Hash([]byte("machine-hash2"))
		advanceResult = model.AdvanceResult{
			InputIndex: secondEpochInput.Index,
			Status:     model.InputCompletionStatus_Accepted,
			// since there are no new outputs in the second epoch,
			// the machine OutputsHash will remain the same
			OutputsHash: firstEpochClaim,
			MachineHash: &machinehash2,
		}
		err = s.repository.StoreAdvanceResult(s.ctx, 1, &advanceResult)
		s.Require().Nil(err)

		err = s.validator.Run(s.ctx)
		s.Require().Nil(err)

		updatedEpoch, err := s.repository.GetEpoch(s.ctx, app.IApplicationAddress.String(), secondEpoch.Index)
		s.Require().Nil(err)
		s.Require().NotNil(updatedEpoch)
		s.Require().NotNil(updatedEpoch.ClaimHash)

		// epoch status was updated
		s.Equal(model.EpochStatus_ClaimComputed, updatedEpoch.Status)
		// claim is the same from previous epoch
		s.Equal(firstEpochClaim, *updatedEpoch.ClaimHash)
	})
}

func (s *ValidatorRepositoryIntegrationSuite) TestItReturnsANewClaimAndProofs() {
	s.Run("WhenThereAreOutputsAndNoPreviousEpoch", func() {
		app := &model.Application{
			Name:                "test-app",
			IApplicationAddress: common.BytesToAddress([]byte("deadbeef")),
			IConsensusAddress:   common.BytesToAddress([]byte("beadbeef")),
			TemplateHash:        common.BytesToHash([]byte("template")),
			TemplateURI:         "/template/path",
			EpochLength:         10,
			State:               model.ApplicationState_Enabled,
		}
		_, err := s.repository.CreateApplication(s.ctx, app)
		s.Require().Nil(err)

		epoch := model.Epoch{
			ApplicationID: 1,
			Index:         0,
			VirtualIndex:  0,
			Status:        model.EpochStatus_InputsProcessed,
			FirstBlock:    0,
			LastBlock:     9,
		}

		input := model.Input{
			Index:                0,
			BlockNumber:          9,
			RawData:              []byte("data"),
			Status:               model.InputCompletionStatus_Accepted,
			TransactionReference: common.BigToHash(big.NewInt(0)),
		}

		var epochInputMap = make(map[*model.Epoch][]*model.Input)
		epochInputMap[&epoch] = []*model.Input{&input}
		err = s.repository.CreateEpochsAndInputs(s.ctx, app.IApplicationAddress.String(), epochInputMap, 10)
		s.Require().Nil(err)

		outputRawData := []byte("output")
		output := model.Output{RawData: outputRawData}

		// calculate the expected claim and proofs
		expectedOutputHash := crypto.Keccak256Hash(outputRawData)
		expectedClaim, expectedProofs, err := merkle.CreateProofs(
			[]common.Hash{expectedOutputHash},
			validator.MAX_OUTPUT_TREE_HEIGHT,
		)
		s.Require().Nil(err)
		s.Require().NotNil(expectedClaim)
		s.Require().NotNil(expectedProofs)

		// Store the input advance result
		machinehash1 := crypto.Keccak256Hash([]byte("machine-hash1"))
		advanceResult := model.AdvanceResult{
			InputIndex:  input.Index,
			Status:      model.InputCompletionStatus_Accepted,
			OutputsHash: expectedClaim,
			Outputs:     [][]byte{outputRawData},
			MachineHash: &machinehash1,
		}
		err = s.repository.StoreAdvanceResult(s.ctx, 1, &advanceResult)
		s.Require().Nil(err)

		err = s.validator.Run(s.ctx)
		s.Require().Nil(err)

		updatedEpoch, err := s.repository.GetEpoch(s.ctx, app.IApplicationAddress.String(), epoch.Index)
		s.Require().Nil(err)
		s.Require().NotNil(updatedEpoch)
		s.Require().NotNil(updatedEpoch.ClaimHash)

		// epoch status was updated
		s.Equal(model.EpochStatus_ClaimComputed, updatedEpoch.Status)
		// claim is the expected new claim
		s.Equal(expectedClaim, *updatedEpoch.ClaimHash)

		updatedOutput, err := s.repository.GetOutput(s.ctx, app.IApplicationAddress.String(), output.Index)
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
			Name:                "test-app",
			IApplicationAddress: common.BytesToAddress([]byte("deadbeef")),
			IConsensusAddress:   common.BytesToAddress([]byte("beadbeef")),
			TemplateHash:        common.BytesToHash([]byte("template")),
			TemplateURI:         "/template/path",
			EpochLength:         10,
			State:               model.ApplicationState_Enabled,
		}
		_, err := s.repository.CreateApplication(s.ctx, app)
		s.Require().Nil(err)

		firstEpoch := model.Epoch{
			ApplicationID: 1,
			Index:         0,
			VirtualIndex:  0,
			Status:        model.EpochStatus_ClaimComputed,
			FirstBlock:    0,
			LastBlock:     9,
		}

		firstInput := model.Input{
			Index:                0,
			BlockNumber:          9,
			RawData:              []byte("data"),
			Status:               model.InputCompletionStatus_Accepted,
			TransactionReference: common.BigToHash(big.NewInt(0)),
		}

		var epochInputMap = make(map[*model.Epoch][]*model.Input)
		epochInputMap[&firstEpoch] = []*model.Input{&firstInput}
		err = s.repository.CreateEpochsAndInputs(s.ctx, app.IApplicationAddress.String(), epochInputMap, 10)
		s.Require().Nil(err)

		firstOutputData := []byte("output1")
		firstOutputHash := crypto.Keccak256Hash(firstOutputData)
		firstOutput := model.Output{
			InputEpochApplicationID: 1,
			InputIndex:              firstInput.Index,
			Index:                   0,
			RawData:                 firstOutputData,
			Hash:                    &firstOutputHash,
		}

		// calculate first epoch claim
		firstEpochClaim, firstEpochProofs, err := merkle.CreateProofs(
			[]common.Hash{firstOutputHash},
			validator.MAX_OUTPUT_TREE_HEIGHT,
		)
		s.Require().Nil(err)
		s.Require().NotNil(firstEpochClaim)

		machinehash1 := crypto.Keccak256Hash([]byte("machine-hash1"))
		advanceResult := model.AdvanceResult{
			InputIndex:  firstInput.Index,
			Status:      model.InputCompletionStatus_Accepted,
			OutputsHash: firstEpochClaim,
			Outputs:     [][]byte{firstOutputData},
			MachineHash: &machinehash1,
		}
		err = s.repository.StoreAdvanceResult(s.ctx, 1, &advanceResult)
		s.Require().Nil(err)

		// update epoch with its claim and insert it in the db
		firstEpoch.ClaimHash = &firstEpochClaim
		firstOutput.OutputHashesSiblings = firstEpochProofs
		err = s.repository.StoreClaimAndProofs(s.ctx, &firstEpoch, []*model.Output{&firstOutput})
		s.Require().Nil(err)

		// setup second epoch
		secondEpoch := model.Epoch{
			ApplicationID: 1,
			Index:         1,
			VirtualIndex:  1,
			Status:        model.EpochStatus_InputsProcessed,
			FirstBlock:    10,
			LastBlock:     19,
		}

		secondInput := model.Input{
			Index:                1,
			BlockNumber:          19,
			RawData:              []byte("data2"),
			Status:               model.InputCompletionStatus_Accepted,
			TransactionReference: common.BigToHash(big.NewInt(1)),
		}

		epochInputMap = make(map[*model.Epoch][]*model.Input)
		epochInputMap[&secondEpoch] = []*model.Input{&secondInput}
		err = s.repository.CreateEpochsAndInputs(s.ctx, app.IApplicationAddress.String(), epochInputMap, 20)
		s.Require().Nil(err)

		// calculate the expected claim
		secondOutputData := []byte("output2")
		secondOutputHash := crypto.Keccak256Hash(secondOutputData)
		expectedEpochClaim, expectedProofs, err := merkle.CreateProofs(
			[]common.Hash{firstOutputHash, secondOutputHash},
			validator.MAX_OUTPUT_TREE_HEIGHT,
		)
		s.Require().Nil(err)
		s.Require().NotNil(expectedEpochClaim)
		s.Require().NotNil(expectedProofs)

		machinehash2 := crypto.Keccak256Hash([]byte("machine-hash2"))
		advanceResult = model.AdvanceResult{
			InputIndex:  secondInput.Index,
			Status:      model.InputCompletionStatus_Accepted,
			OutputsHash: expectedEpochClaim,
			Outputs:     [][]byte{secondOutputData},
			MachineHash: &machinehash2,
		}
		err = s.repository.StoreAdvanceResult(s.ctx, 1, &advanceResult)
		s.Require().Nil(err)

		err = s.validator.Run(s.ctx)
		s.Require().Nil(err)

		updatedSecondEpoch, err := s.repository.GetEpoch(
			s.ctx,
			app.IApplicationAddress.String(),
			secondEpoch.Index,
		)
		s.Require().Nil(err)
		s.Require().NotNil(updatedSecondEpoch)
		s.Require().NotNil(updatedSecondEpoch.ClaimHash)

		// assert epoch status was changed
		s.Equal(model.EpochStatus_ClaimComputed, updatedSecondEpoch.Status)
		// assert second epoch claim is a new claim
		s.NotEqual(firstEpochClaim, *updatedSecondEpoch.ClaimHash)
		s.Equal(expectedEpochClaim, *updatedSecondEpoch.ClaimHash)

		updatedSecondOutput, err := s.repository.GetOutput(
			s.ctx,
			app.IApplicationAddress.String(),
			1,
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

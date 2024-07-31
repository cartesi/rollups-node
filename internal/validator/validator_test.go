// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	crand "crypto/rand"
	mrand "math/rand"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type ValidatorSuite struct {
	suite.Suite
}

func TestValidatorSuite(t *testing.T) {
	suite.Run(t, new(ValidatorSuite))
}

var (
	validator               *Validator
	repository              *MockRepository
	dummyEpochs             []Epoch
	inputBoxDeploymentBlock uint64
)

func (s *ValidatorSuite) SetupSubTest() {
	repository = newMockRepository()
	validator = NewValidator(repository, 0, 500*time.Millisecond)
	dummyEpochs = []Epoch{
		{StartBlock: 0, EndBlock: 9},
		{StartBlock: 10, EndBlock: 19},
		{StartBlock: 20, EndBlock: 29},
		{StartBlock: 30, EndBlock: 39},
	}
}

func (s *ValidatorSuite) TearDownSubTest() {
	repository = nil
	validator = nil
}

func (s *ValidatorSuite) TestItCreatesClaimAndProofs() {
	// returns pristine claim and no proofs
	s.Run("WhenThereAreNoOutputsAndNoPreviousEpoch", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		epoch := dummyEpochs[0]

		repository.On(
			"GetOutputs",
			mock.Anything, epoch.AppAddress, epoch.StartBlock, epoch.EndBlock,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, &epoch).Return(nil, nil)

		claim, outputs, err := validator.createClaimAndProofs(ctx, &epoch)
		s.Require().NotNil(claim)
		s.Require().Nil(err)

		expectedClaim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)
		s.Require().NotNil(expectedClaim)

		s.Equal(expectedClaim, *claim)
		s.Nil(outputs)
		repository.AssertExpectations(s.T())
	})

	// returns previous epoch
	s.Run("WhenThereAreNoOutputsAndThereIsAPreviousEpoch", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		previousEpoch := dummyEpochs[0]
		expectedClaim := randomHash()
		previousEpoch.Claim = &expectedClaim
		epoch := dummyEpochs[1]

		repository.On(
			"GetOutputs",
			mock.Anything, epoch.AppAddress, epoch.StartBlock, epoch.EndBlock,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, &epoch).Return(&previousEpoch, nil)

		claim, outputs, err := validator.createClaimAndProofs(ctx, &epoch)
		s.Require().Nil(err)
		s.Require().NotNil(claim)

		s.Equal(previousEpoch.Claim, claim)
		s.Nil(outputs)
		repository.AssertExpectations(s.T())
	})

	// returns new claim and proofs
	s.Run("WhenThereAreOutputsAndNoPreviousEpoch", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		epoch := dummyEpochs[0]
		outputs := randomOutputs(2, 0, false)

		repository.On(
			"GetOutputs",
			mock.Anything, epoch.AppAddress, epoch.StartBlock, epoch.EndBlock,
		).Return(outputs, nil).Once()
		repository.On("GetPreviousEpoch", mock.Anything, &epoch).Return(nil, nil)

		claim, updatedOutputs, err := validator.createClaimAndProofs(ctx, &epoch)
		s.Require().Nil(err)
		s.Require().NotNil(claim)

		s.Len(updatedOutputs, len(outputs))
		for idx, output := range updatedOutputs {
			s.Equal(outputs[idx].Id, output.Id)
			s.NotNil(output.Hash)
			s.NotNil(output.OutputHashesSiblings)
		}
		repository.AssertExpectations(s.T())
	})

	// returns new claim and proofs
	s.Run("WhenThereAreOutputsAndAPreviousEpoch", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		previousEpoch := dummyEpochs[0]
		previousEpochClaim := randomHash()
		previousEpoch.Claim = &previousEpochClaim
		epoch := dummyEpochs[1]
		previousOutputs := randomOutputs(2, 0, true)
		epochOutputs := randomOutputs(2, 2, false)

		repository.On(
			"GetOutputs",
			mock.Anything, epoch.AppAddress, epoch.StartBlock, epoch.EndBlock,
		).Return(epochOutputs, nil)
		repository.On(
			"GetOutputs",
			mock.Anything, epoch.AppAddress, inputBoxDeploymentBlock, previousEpoch.EndBlock,
		).Return(previousOutputs, nil)
		repository.On("GetPreviousEpoch", mock.Anything, &epoch).Return(&previousEpoch, nil)

		claim, updatedOutputs, err := validator.createClaimAndProofs(ctx, &epoch)
		s.Require().Nil(err)
		s.Require().NotNil(claim)

		allOutputs := append(previousOutputs, epochOutputs...)
		leaves := make([]Hash, 0, len(allOutputs))
		for _, output := range allOutputs {
			leaves = append(leaves, *output.Hash)
		}
		expectedClaim, allProofs, err := merkle.CreateProofs(leaves, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)

		s.NotEqual(previousEpoch.Claim, claim)
		s.Equal(&expectedClaim, claim)
		s.Len(updatedOutputs, len(epochOutputs))

		for idx, output := range updatedOutputs {
			s.Equal(epochOutputs[idx].Index, output.Index)
			s.NotNil(output.Hash)
			s.NotNil(output.OutputHashesSiblings)
			s.assertProofs(output, allProofs)
		}
		repository.AssertExpectations(s.T())
	})
}

func (s *ValidatorSuite) TestItFailsWhenClaimDoesNotMatchMachineOutputsHash() {
	s.Run("OneAppSingleEpoch", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app := Application{ContractAddress: randomAddress()}
		epochs := []*Epoch{&dummyEpochs[0]}
		epochs[0].AppAddress = app.ContractAddress
		mismatchedHash := randomHash()
		repository.On(
			"GetProcessedEpochs", mock.Anything, epochs[0].AppAddress,
		).Return(epochs, nil)
		repository.On(
			"GetLastInputOutputHash", mock.Anything, epochs[0],
		).Return(&mismatchedHash, nil)
		repository.On(
			"GetOutputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, mock.Anything).Return(nil, nil)

		err := validator.validateApplication(ctx, app)
		s.NotNil(err)
		s.ErrorContains(err, "hash mismatch")

		repository.AssertExpectations(s.T())
	})

	// fails on the second epoch, do not process the third
	s.Run("OneAppThreeEpochs", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app := Application{ContractAddress: randomAddress()}
		epochs := []*Epoch{&dummyEpochs[0], &dummyEpochs[1], &dummyEpochs[2]}
		for _, epoch := range epochs {
			epoch.AppAddress = app.ContractAddress
		}
		epoch0Claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)
		epochs[0].Claim = &epoch0Claim
		mismatchedHash := randomHash()

		repository.On(
			"GetProcessedEpochs", mock.Anything, epochs[0].AppAddress,
		).Return(epochs, nil).Once()
		repository.On(
			"GetOutputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochs[0]).Return(nil, nil)
		repository.On(
			"GetLastInputOutputHash", mock.Anything, epochs[0],
		).Return(epochs[0].Claim, nil)
		repository.On(
			"GetLastInputOutputHash", mock.Anything, epochs[1],
		).Return(&mismatchedHash, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochs[1]).Return(epochs[0], nil)
		repository.On(
			"SetEpochClaimAndInsertProofsTransaction",
			mock.Anything, epochs[0], mock.Anything,
		).Return(nil)

		err = validator.validateApplication(ctx, app)
		s.NotNil(err)
		s.ErrorContains(err, "hash mismatch")
		repository.AssertExpectations(s.T())
	})

	// validates first app, fails on the first epoch of the second
	s.Run("TwoAppsTwoEpochsEach", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		applications := []Application{
			{ContractAddress: randomAddress()},
			{ContractAddress: randomAddress()},
		}
		epochsApp1 := []*Epoch{&dummyEpochs[0], &dummyEpochs[1]}
		epochsApp2 := []*Epoch{&dummyEpochs[2], &dummyEpochs[3]}
		for _, epoch := range epochsApp1 {
			epoch.AppAddress = applications[0].ContractAddress
		}
		for _, epoch := range epochsApp2 {
			epoch.AppAddress = applications[1].ContractAddress
		}
		epoch0Claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)
		epochsApp1[0].Claim = &epoch0Claim
		mismatchedHash := randomHash()

		repository.On("GetAllRunningApplications", mock.Anything).Return(applications, nil)
		// App1 calls
		repository.On(
			"GetProcessedEpochs", mock.Anything, applications[0].ContractAddress,
		).Return(epochsApp1, nil)
		repository.On(
			"GetOutputs",
			mock.Anything, applications[0].ContractAddress, mock.Anything, mock.Anything,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochsApp1[0]).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochsApp1[1]).Return(epochsApp1[0], nil)
		repository.On(
			"GetLastInputOutputHash", mock.Anything, epochsApp1[0],
		).Return(epochsApp1[0].Claim, nil)
		repository.On(
			"GetLastInputOutputHash", mock.Anything, epochsApp1[1],
		).Return(epochsApp1[0].Claim, nil)
		repository.On(
			"SetEpochClaimAndInsertProofsTransaction",
			mock.Anything, epochsApp1[0], mock.Anything,
		).Return(nil)
		repository.On(
			"SetEpochClaimAndInsertProofsTransaction",
			mock.Anything, epochsApp1[1], mock.Anything,
		).Return(nil)

		// App2 calls
		repository.On(
			"GetProcessedEpochs", mock.Anything, applications[1].ContractAddress,
		).Return(epochsApp2, nil)
		repository.On(
			"GetOutputs",
			mock.Anything, applications[1].ContractAddress, mock.Anything, mock.Anything,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochsApp2[0]).Return(nil, nil)
		repository.On(
			"GetLastInputOutputHash", mock.Anything, epochsApp2[0],
		).Return(&mismatchedHash, nil)

		err = validator.Run(ctx)
		s.NotNil(err)
		s.ErrorContains(err, "hash mismatch")
		repository.AssertExpectations(s.T())
	})
}

func (s *ValidatorSuite) assertProofs(output *Output, allProofs []Hash) {
	start := output.Index * MAX_OUTPUT_TREE_HEIGHT
	end := (output.Index * MAX_OUTPUT_TREE_HEIGHT) + MAX_OUTPUT_TREE_HEIGHT
	s.Equal(allProofs[start:end], output.OutputHashesSiblings)
}

func randomAddress() Address {
	address := make([]byte, 20)
	_, err := crand.Read(address)
	if err != nil {
		panic(err)
	}
	return Address(address)
}

func randomHash() Hash {
	hash := make([]byte, 32)
	_, err := crand.Read(hash)
	if err != nil {
		panic(err)
	}
	return Hash(hash)
}

func randomBytes() []byte {
	size := mrand.Intn(100) + 1
	bytes := make([]byte, size)
	_, err := crand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}

// TODO: document properly
func randomOutputs(size int, firstIdx int, withProofs bool) []*Output {
	slice := make([]*Output, size)
	for idx := 0; idx < size; idx++ {
		output := Output{
			Id:      mrand.Uint64(),
			Index:   uint64(idx + firstIdx),
			RawData: randomBytes(),
		}
		if withProofs {
			proofs := make([]Hash, MAX_OUTPUT_TREE_HEIGHT)
			hash := crypto.Keccak256Hash(output.RawData)
			output.Hash = &hash
			for idx := 0; idx < MAX_OUTPUT_TREE_HEIGHT; idx++ {
				proofs[idx] = randomHash()
			}
			output.OutputHashesSiblings = proofs
		}
		slice[idx] = &output
	}
	return slice
}

type MockRepository struct {
	mock.Mock
}

func newMockRepository() *MockRepository {
	return new(MockRepository)
}

func (m *MockRepository) GetAllRunningApplications(ctx context.Context) ([]Application, error) {
	args := m.Called(ctx)

	apps, ok := args.Get(0).([]Application)
	if ok {
		return apps, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) GetInputs(
	ctx context.Context,
	application Address,
	startBlock, endBlock uint64,
) ([]*Input, error) {
	args := m.Called(ctx, application, startBlock, endBlock)

	if inputs, ok := args.Get(0).([]*Input); ok {
		return inputs, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) GetOutputs(
	ctx context.Context,
	application Address,
	startBlock uint64,
	endBlock uint64,
) ([]*Output, error) {
	args := m.Called(ctx, application, startBlock, endBlock)

	if outputs, ok := args.Get(0).([]*Output); ok {
		return outputs, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) GetProcessedEpochs(
	ctx context.Context,
	application Address,
) ([]*Epoch, error) {
	args := m.Called(ctx, application)

	if epochs, ok := args.Get(0).([]*Epoch); ok {
		return epochs, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) GetLastInputOutputHash(
	ctx context.Context,
	epoch *Epoch,
) (*Hash, error) {
	args := m.Called(ctx, epoch)

	if hash, ok := args.Get(0).(*Hash); ok {
		return hash, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) GetPreviousEpoch(
	ctx context.Context,
	currentEpoch *Epoch,
) (*Epoch, error) {
	args := m.Called(ctx, currentEpoch)

	if epoch, ok := args.Get(0).(*Epoch); ok {
		return epoch, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) SetEpochClaimAndInsertProofsTransaction(
	ctx context.Context,
	epoch *Epoch,
	outputs []*Output,
) error {
	args := m.Called(ctx, epoch, outputs)
	return args.Error(0)
}

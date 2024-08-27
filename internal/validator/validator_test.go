// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	crand "crypto/rand"
	mrand "math/rand"
	"testing"

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
	validator = NewValidator(repository, 0)
	dummyEpochs = []Epoch{
		{Index: 0, FirstBlock: 0, LastBlock: 9},
		{Index: 1, FirstBlock: 10, LastBlock: 19},
		{Index: 2, FirstBlock: 20, LastBlock: 29},
		{Index: 3, FirstBlock: 30, LastBlock: 39},
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
			"GetOutputsProducedInBlockRange",
			mock.Anything, epoch.AppAddress, epoch.FirstBlock, epoch.LastBlock,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epoch).Return(nil, nil)

		claim, outputs, err := validator.createClaimAndProofs(ctx, epoch)
		s.Require().Nil(err)
		s.Require().NotNil(claim)

		expectedClaim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)
		s.Require().NotNil(expectedClaim)

		s.Equal(expectedClaim, *claim)
		s.Nil(outputs)
		repository.AssertExpectations(s.T())
	})

	// returns previous epoch claim and no proofs
	s.Run("WhenThereAreNoOutputsAndThereIsAPreviousEpoch", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		previousEpoch := dummyEpochs[0]
		expectedClaim := randomHash()
		previousEpoch.ClaimHash = &expectedClaim
		epoch := dummyEpochs[1]

		repository.On(
			"GetOutputsProducedInBlockRange",
			mock.Anything, epoch.AppAddress, epoch.FirstBlock, epoch.LastBlock,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epoch).Return(&previousEpoch, nil)

		claim, outputs, err := validator.createClaimAndProofs(ctx, epoch)
		s.Require().Nil(err)
		s.Require().NotNil(claim)

		s.Equal(expectedClaim, *claim)
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
			"GetOutputsProducedInBlockRange",
			mock.Anything, epoch.AppAddress, epoch.FirstBlock, epoch.LastBlock,
		).Return(outputs, nil).Once()
		repository.On("GetPreviousEpoch", mock.Anything, epoch).Return(nil, nil)

		claim, updatedOutputs, err := validator.createClaimAndProofs(ctx, epoch)
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
		previousEpoch.ClaimHash = &previousEpochClaim
		epoch := dummyEpochs[1]
		previousOutputs := randomOutputs(2, 0, true)
		epochOutputs := randomOutputs(2, 2, false)

		repository.On(
			"GetOutputsProducedInBlockRange",
			mock.Anything, epoch.AppAddress, epoch.FirstBlock, epoch.LastBlock,
		).Return(epochOutputs, nil)
		repository.On(
			"GetOutputsProducedInBlockRange",
			mock.Anything, epoch.AppAddress, inputBoxDeploymentBlock, previousEpoch.LastBlock,
		).Return(previousOutputs, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epoch).Return(&previousEpoch, nil)

		claim, updatedOutputs, err := validator.createClaimAndProofs(ctx, epoch)
		s.Require().Nil(err)
		s.Require().NotNil(claim)

		allOutputs := append(previousOutputs, epochOutputs...)
		leaves := make([]Hash, 0, len(allOutputs))
		for _, output := range allOutputs {
			leaves = append(leaves, *output.Hash)
		}
		expectedClaim, allProofs, err := merkle.CreateProofs(leaves, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)

		s.NotEqual(previousEpoch.ClaimHash, claim)
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
		epochs := []Epoch{dummyEpochs[0]}
		epochs[0].AppAddress = app.ContractAddress
		mismatchedHash := randomHash()
		repository.On(
			"GetProcessedEpochs", mock.Anything, epochs[0].AppAddress,
		).Return(epochs, nil)
		repository.On(
			"GetLastInputOutputsHash",
			mock.Anything, epochs[0].Index, epochs[0].AppAddress,
		).Return(&mismatchedHash, nil)
		repository.On(
			"GetOutputsProducedInBlockRange",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, mock.Anything).Return(nil, nil)

		err := validator.validateApplication(ctx, app)
		s.NotNil(err)
		s.ErrorContains(err, "claim does not match")

		repository.AssertExpectations(s.T())
	})

	// fails on the second epoch, do not process the third
	s.Run("OneAppThreeEpochs", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app := Application{ContractAddress: randomAddress()}
		epochs := []Epoch{dummyEpochs[0], dummyEpochs[1], dummyEpochs[2]}
		for idx := range epochs {
			epochs[idx].AppAddress = app.ContractAddress
		}
		epoch0Claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)
		epochs[0].ClaimHash = &epoch0Claim
		mismatchedHash := randomHash()

		repository.On(
			"GetProcessedEpochs", mock.Anything, app.ContractAddress,
		).Return(epochs, nil).Once()
		repository.On(
			"GetOutputsProducedInBlockRange",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochs[0]).Return(nil, nil)
		repository.On(
			"GetLastInputOutputsHash",
			mock.Anything, epochs[0].Index, epochs[0].AppAddress,
		).Return(epochs[0].ClaimHash, nil)
		repository.On(
			"GetLastInputOutputsHash",
			mock.Anything, epochs[1].Index, epochs[1].AppAddress,
		).Return(&mismatchedHash, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochs[1]).Return(epochs[0], nil)
		repository.On(
			"SetEpochClaimAndInsertProofsTransaction",
			mock.Anything, mock.Anything, mock.Anything,
		).Return(nil).Once()

		err = validator.validateApplication(ctx, app)
		s.NotNil(err)
		s.ErrorContains(err, "claim does not match")
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
		epochsApp1 := []Epoch{dummyEpochs[0], dummyEpochs[1]}
		epochsApp2 := []Epoch{dummyEpochs[2], dummyEpochs[3]}
		for idx := range epochsApp1 {
			epochsApp1[idx].AppAddress = applications[0].ContractAddress
		}
		for idx := range epochsApp2 {
			epochsApp2[idx].AppAddress = applications[1].ContractAddress
		}
		epoch0Claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)
		epochsApp1[0].ClaimHash = &epoch0Claim
		mismatchedHash := randomHash()

		repository.On("GetAllRunningApplications", mock.Anything).Return(applications, nil)
		repository.On(
			"GetProcessedEpochs", mock.Anything, applications[0].ContractAddress,
		).Return(epochsApp1, nil)
		repository.On(
			"GetOutputsProducedInBlockRange",
			mock.Anything, applications[0].ContractAddress, mock.Anything, mock.Anything,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochsApp1[0]).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochsApp1[1]).Return(epochsApp1[0], nil)
		repository.On(
			"GetLastInputOutputsHash",
			mock.Anything, epochsApp1[0].Index, epochsApp1[0].AppAddress,
		).Return(epochsApp1[0].ClaimHash, nil)
		repository.On(
			"GetLastInputOutputsHash",
			mock.Anything, epochsApp1[1].Index, epochsApp1[1].AppAddress,
		).Return(epochsApp1[0].ClaimHash, nil)
		repository.On(
			"SetEpochClaimAndInsertProofsTransaction",
			mock.Anything, mock.Anything, mock.Anything,
		).Return(nil).Twice()
		repository.On(
			"GetProcessedEpochs", mock.Anything, applications[1].ContractAddress,
		).Return(epochsApp2, nil)
		repository.On(
			"GetOutputsProducedInBlockRange",
			mock.Anything, applications[1].ContractAddress, mock.Anything, mock.Anything,
		).Return(nil, nil)
		repository.On("GetPreviousEpoch", mock.Anything, epochsApp2[0]).Return(nil, nil)
		repository.On(
			"GetLastInputOutputsHash",
			mock.Anything, epochsApp2[0].Index, epochsApp2[0].AppAddress,
		).Return(&mismatchedHash, nil)

		err = validator.Run(ctx)
		s.NotNil(err)
		s.ErrorContains(err, "claim does not match")
		repository.AssertExpectations(s.T())
	})
}

func (s *ValidatorSuite) assertProofs(output Output, allProofs []Hash) {
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

// randomOutputs generates n new Outputs with sequential indexes starting at
// `firstIdx` and random data. Optionally, it will generate dummy proofs if
// `withProofs` is true. Returns an slice with the new Outputs.
func randomOutputs(n int, firstIdx int, withProofs bool) []Output {
	slice := make([]Output, n)
	for idx := 0; idx < n; idx++ {
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
		slice[idx] = output
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

func (m *MockRepository) GetOutputsProducedInBlockRange(
	ctx context.Context,
	application Address,
	firstBlock, lastBlock uint64,
) ([]Output, error) {
	args := m.Called(ctx, application, firstBlock, lastBlock)

	if outputs, ok := args.Get(0).([]Output); ok {
		return outputs, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) GetProcessedEpochs(
	ctx context.Context,
	application Address,
) ([]Epoch, error) {
	args := m.Called(ctx, application)

	if epochs, ok := args.Get(0).([]Epoch); ok {
		return epochs, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) GetLastInputOutputsHash(
	ctx context.Context,
	epochIndex uint64,
	appAddress Address,
) (*Hash, error) {
	args := m.Called(ctx, epochIndex, appAddress)

	if hash, ok := args.Get(0).(*Hash); ok {
		return hash, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) GetPreviousEpoch(
	ctx context.Context,
	currentEpoch Epoch,
) (*Epoch, error) {
	args := m.Called(ctx, currentEpoch)

	if epoch, ok := args.Get(0).(*Epoch); ok {
		return epoch, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockRepository) SetEpochClaimAndInsertProofsTransaction(
	ctx context.Context,
	epoch Epoch,
	outputs []Output,
) error {
	args := m.Called(ctx, epoch, outputs)
	return args.Error(0)
}

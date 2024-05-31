// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 180 * time.Second

type ValidatorSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	repo   *MockRepository
}

func TestValidatorSuite(t *testing.T) {
	suite.Run(t, new(ValidatorSuite))
}

func (s *ValidatorSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)
}

func (s *ValidatorSuite) TearDownSuite() {
	s.cancel()
}

func (s *ValidatorSuite) SetupTest() {
	s.repo = newMockRepository()
}
func (s *ValidatorSuite) TearDownTest() {
	s.repo = nil
}

func (s *ValidatorSuite) TestItStopsWhenContextIsCanceled() {
	ctx, cancel := context.WithCancel(s.ctx)
	epochDuration := uint64(10)
	inputBoxDeploymentBlock := uint64(0)
	validator := NewValidator(s.repo, epochDuration, inputBoxDeploymentBlock)

	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- validator.Start(ctx, ready)
	}()
	cancel()

	err := <-errChannel
	s.Require().Equal(context.Canceled, err, "stopped for the wrong reason")
}

func (s *ValidatorSuite) TestItEventuallyBecomesReady() {
	epochDuration := uint64(10)
	inputBoxDeploymentBlock := uint64(0)
	validator := NewValidator(s.repo, epochDuration, inputBoxDeploymentBlock)

	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- validator.Start(s.ctx, ready)
	}()

	select {
	case <-ready:
	case err := <-errChannel:
		s.FailNow("unexpected failure", err)
	}
}

func (s *ValidatorSuite) TestItFinishesAnEmptyEpoch() {
	epochDuration := uint64(2)
	inputBoxDeploymentBlock := uint64(0)
	currentEpoch := Epoch{
		StartBlock: inputBoxDeploymentBlock,
		EndBlock:   inputBoxDeploymentBlock + epochDuration,
	}
	expectedEpoch := Epoch{
		StartBlock: currentEpoch.EndBlock + 1,
		EndBlock:   currentEpoch.EndBlock + epochDuration,
	}

	s.repo.Unset("GetCurrentEpoch")
	s.repo.On("GetCurrentEpoch", s.ctx).Return(currentEpoch, nil).Times(3)
	s.repo.On("GetCurrentEpoch", s.ctx).Return(expectedEpoch, nil)

	s.repo.Unset("GetMostRecentBlock")
	stop := fmt.Errorf("stop")
	s.repo.On("GetMostRecentBlock", s.ctx).Return(uint64(0), nil).Once()
	s.repo.On("GetMostRecentBlock", s.ctx).Return(uint64(1), nil).Once()
	s.repo.On("GetMostRecentBlock", s.ctx).Return(uint64(2), nil).Once()
	s.repo.On("GetMostRecentBlock", s.ctx).Return(uint64(0), stop).Once()

	validator := NewValidator(s.repo, epochDuration, inputBoxDeploymentBlock)

	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- validator.Start(s.ctx, ready)
	}()

	for {
		select {
		case <-s.ctx.Done():
			s.FailNow("timed out")
		case err := <-errChannel:
			s.Require().ErrorIs(err, stop, "unexpected failure")
			s.repo.AssertCalled(s.T(), "FinishEmptyEpochTransaction", mock.Anything, expectedEpoch)
			s.repo.AssertNumberOfCalls(s.T(), "FinishEmptyEpochTransaction", 1)
			return
		}
	}
}

func (s *ValidatorSuite) TestItFinishesAnEpochWithOutputs() {
	epochDuration := uint64(2)
	inputBoxDeploymentBlock := uint64(0)
	currentEpoch := Epoch{
		StartBlock: inputBoxDeploymentBlock,
		EndBlock:   inputBoxDeploymentBlock + epochDuration,
	}
	expectedEpoch := Epoch{
		StartBlock: currentEpoch.EndBlock + 1,
		EndBlock:   currentEpoch.EndBlock + epochDuration,
	}

	s.repo.Unset("GetCurrentEpoch")
	s.repo.On("GetCurrentEpoch", s.ctx).Return(currentEpoch, nil).Times(3)
	s.repo.On("GetCurrentEpoch", s.ctx).Return(expectedEpoch, nil)

	s.repo.Unset("GetMostRecentBlock")
	stop := fmt.Errorf("stop")
	s.repo.On("GetMostRecentBlock", s.ctx).Return(uint64(0), nil).Once()
	s.repo.On("GetMostRecentBlock", s.ctx).Return(uint64(1), nil).Once()
	s.repo.On("GetMostRecentBlock", s.ctx).Return(uint64(2), nil).Once()
	s.repo.On("GetMostRecentBlock", s.ctx).Return(uint64(0), stop)

	s.repo.Unset("GetAllOutputsFromProcessedInputs")
	s.repo.On(
		"GetAllOutputsFromProcessedInputs",
		mock.Anything,
		currentEpoch.StartBlock,
		currentEpoch.EndBlock,
		mock.Anything,
	).Return([]Output{{InputIndex: uint64(0), Index: uint64(0), Blob: []byte{}}}, nil)

	s.repo.Unset("GetMachineStateHash")
	genericData := common.HexToHash("0xdeadbeef")
	s.repo.On("GetMachineStateHash", mock.Anything, uint64(0)).Return(genericData, nil)

	validator := NewValidator(s.repo, epochDuration, inputBoxDeploymentBlock)

	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- validator.Start(s.ctx, ready)
	}()

	for {
		select {
		case <-s.ctx.Done():
			s.FailNow("timed out")
		case err := <-errChannel:
			s.Require().ErrorIs(err, stop, "unexpected failure")

			epochHash := crypto.Keccak256Hash(genericData.Bytes(), genericData.Bytes())
			expectedClaim := &Claim{
				InputRange: InputRange{First: uint64(0), Last: uint64(0)},
				EpochHash:  epochHash,
			}
			expectedProofs := []Proof{{OutputsEpochRootHash: genericData}}
			s.repo.AssertCalled(
				s.T(),
				"FinishEpochTransaction",
				mock.Anything,
				expectedEpoch,
				expectedClaim,
				expectedProofs,
			)
			s.repo.AssertNumberOfCalls(s.T(), "FinishEpochTransaction", 1)
			return
		}
	}
}

// ------------------------------------------------------------------------------------------------
// Auxiliary types and functions
// ------------------------------------------------------------------------------------------------

type MockRepository struct {
	mock.Mock
}

func newMockRepository() *MockRepository {
	repo := new(MockRepository)
	repo.On("GetMostRecentBlock", mock.Anything).Return(uint64(0), nil)
	repo.On("GetCurrentEpoch", mock.Anything).Return(Epoch{}, nil)
	repo.On(
		"GetMachineStateHash",
		mock.Anything,
		mock.Anything,
	).Return(common.HexToHash("0xdeadbeef"), nil)
	repo.On(
		"GetAllOutputsFromProcessedInputs",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return([]Output{}, nil)
	repo.On("InsertFirstEpochTransaction", mock.Anything, mock.Anything).Return(nil)
	repo.On("FinishEmptyEpochTransaction", mock.Anything, mock.Anything).Return(nil)
	repo.On(
		"FinishEpochTransaction",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil)
	return repo
}

func (m *MockRepository) Unset(methodName string) {
	for _, call := range m.ExpectedCalls {
		if call.Method == methodName {
			call.Unset()
		}
	}
}

func (m *MockRepository) GetMostRecentBlock(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRepository) GetCurrentEpoch(ctx context.Context) (Epoch, error) {
	args := m.Called(ctx)
	return args.Get(0).(Epoch), args.Error(1)
}

func (m *MockRepository) GetMachineStateHash(
	ctx context.Context,
	inputIndex uint64,
) (Hash, error) {
	args := m.Called(ctx, inputIndex)
	return args.Get(0).(Hash), args.Error(1)
}

func (m *MockRepository) GetAllOutputsFromProcessedInputs(
	ctx context.Context,
	startBlock uint64,
	endBlock uint64,
	timeout *time.Duration,
) ([]Output, error) {
	args := m.Called(ctx, startBlock, endBlock, timeout)
	return args.Get(0).([]Output), args.Error(1)
}

func (m *MockRepository) InsertFirstEpochTransaction(
	ctx context.Context,
	epoch Epoch,
) error {
	args := m.Called(ctx, epoch)
	return args.Error(0)
}

func (m *MockRepository) FinishEmptyEpochTransaction(
	ctx context.Context,
	nextEpoch Epoch,
) error {
	args := m.Called(ctx, nextEpoch)
	return args.Error(0)
}

func (m *MockRepository) FinishEpochTransaction(
	ctx context.Context,
	nextEpoch Epoch,
	claim *Claim,
	proofs []Proof,
) error {
	args := m.Called(ctx, nextEpoch, claim, proofs)
	return args.Error(0)
}

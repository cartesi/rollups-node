// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	crand "crypto/rand"
	"log/slog"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
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
	validator   *Service
	repo        *Mockrepo
	dummyEpochs []Epoch
)

func (s *ValidatorSuite) SetupSubTest() {
	repo = newMockrepo()
	validator = &Service{}
	s.Require().Nil(Create(&CreateInfo{
		Repository:     repo,
		MaxStartupTime: 5 * time.Second,
		CreateInfo: service.CreateInfo{
			Impl:     validator,
			LogLevel: service.LogLevel(slog.LevelInfo),
		},
	}, validator))
	dummyEpochs = []Epoch{
		{Index: 0, VirtualIndex: 0, FirstBlock: 0, LastBlock: 9},
		{Index: 1, VirtualIndex: 1, FirstBlock: 10, LastBlock: 19},
		{Index: 2, VirtualIndex: 2, FirstBlock: 20, LastBlock: 29},
		{Index: 3, VirtualIndex: 3, FirstBlock: 30, LastBlock: 39},
	}
}

func (s *ValidatorSuite) TearDownSubTest() {
	repo = nil
	validator = nil
}

func (s *ValidatorSuite) TestItFailsWhenClaimDoesNotMatchMachineOutputsHash() {
	s.Run("OneAppSingleEpoch", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app := Application{ID: 0, IApplicationAddress: randomAddress()}
		epochs := []*Epoch{&dummyEpochs[0]}
		epochs[0].ApplicationID = app.ID
		mismatchedHash := randomHash()
		input := Input{EpochApplicationID: app.ID, OutputsHash: &mismatchedHash}
		repo.On(
			"ListEpochs", mock.Anything, app.IApplicationAddress.String(), mock.Anything, mock.Anything,
		).Return(epochs, nil)
		repo.On(
			"GetLastInput",
			mock.Anything, app.IApplicationAddress.String(), epochs[0].Index,
		).Return(&input, nil)
		repo.On(
			"ListOutputs",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return([]*Output{}, nil)

		err := validator.validateApplication(ctx, &app)
		s.NotNil(err)
		s.ErrorContains(err, "claim does not match")

		repo.AssertExpectations(s.T())
	})

	// fails on the second epoch, do not process the third
	s.Run("OneAppThreeEpochs", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app := Application{IApplicationAddress: randomAddress()}
		epochs := []*Epoch{&dummyEpochs[0], &dummyEpochs[1], &dummyEpochs[2]}
		for idx := range epochs {
			epochs[idx].ApplicationID = app.ID
		}
		epoch0Claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)
		epochs[0].ClaimHash = &epoch0Claim
		input1 := Input{EpochApplicationID: app.ID, OutputsHash: &epoch0Claim}

		mismatchedHash := randomHash()
		input2 := Input{EpochApplicationID: app.ID, OutputsHash: &mismatchedHash}

		repo.On(
			"ListEpochs", mock.Anything, app.IApplicationAddress.String(), mock.Anything, mock.Anything,
		).Return(epochs, nil).Once()
		repo.On(
			"ListOutputs",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return([]*Output{}, nil)
		repo.On(
			"GetLastInput",
			mock.Anything, app.IApplicationAddress.String(), epochs[0].Index,
		).Return(&input1, nil)
		repo.On(
			"GetLastInput",
			mock.Anything, app.IApplicationAddress.String(), epochs[1].Index,
		).Return(&input2, nil)
		repo.On("GetEpochByVirtualIndex", mock.Anything, mock.Anything, uint64(0)).Return(epochs[0], nil)
		repo.On(
			"StoreClaimAndProofs",
			mock.Anything, mock.Anything, mock.Anything,
		).Return(nil).Once()

		err = validator.validateApplication(ctx, &app)
		s.NotNil(err)
		s.ErrorContains(err, "claim does not match")
		repo.AssertExpectations(s.T())
	})

	// validates first app, fails on the first epoch of the second
	s.Run("TwoAppsTwoEpochsEach", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		applications := []*Application{
			{IApplicationAddress: randomAddress()},
			{IApplicationAddress: randomAddress()},
		}
		epochsApp1 := []*Epoch{&dummyEpochs[0], &dummyEpochs[1]}
		epochsApp2 := []*Epoch{&dummyEpochs[2], &dummyEpochs[3]}
		for idx := range epochsApp1 {
			epochsApp1[idx].ApplicationID = applications[0].ID
		}
		for idx := range epochsApp2 {
			epochsApp2[idx].ApplicationID = applications[1].ID
		}
		epoch0Claim, _, err := merkle.CreateProofs(nil, MAX_OUTPUT_TREE_HEIGHT)
		s.Require().Nil(err)
		epochsApp1[0].ClaimHash = &epoch0Claim
		input1 := Input{EpochApplicationID: applications[0].ID, OutputsHash: &epoch0Claim}

		mismatchedHash := randomHash()
		input2 := Input{EpochApplicationID: applications[1].ID, OutputsHash: &mismatchedHash}

		repo.On("ListApplications",
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).Return(applications, nil)
		repo.On(
			"ListEpochs", mock.Anything, applications[0].IApplicationAddress.String(), mock.Anything, mock.Anything,
		).Return(epochsApp1, nil)
		repo.On(
			"ListOutputs",
			mock.Anything, applications[0].IApplicationAddress.String(), mock.Anything, mock.Anything,
		).Return([]*Output{}, nil)
		repo.On("GetEpochByVirtualIndex", mock.Anything, mock.Anything, uint64(0)).Return(epochsApp1[0], nil)
		repo.On(
			"GetLastInput",
			mock.Anything, applications[0].IApplicationAddress.String(), epochsApp1[0].Index,
		).Return(&input1, nil)
		repo.On(
			"GetLastInput",
			mock.Anything, applications[0].IApplicationAddress.String(), epochsApp1[1].Index,
		).Return(&input1, nil)
		repo.On(
			"StoreClaimAndProofs",
			mock.Anything, mock.Anything, mock.Anything,
		).Return(nil).Twice()
		repo.On(
			"ListEpochs", mock.Anything, applications[1].IApplicationAddress.String(), mock.Anything, mock.Anything,
		).Return(epochsApp2, nil)
		repo.On(
			"ListOutputs",
			mock.Anything, applications[1].IApplicationAddress.String(), mock.Anything, mock.Anything,
		).Return([]*Output{}, nil)
		repo.On("GetEpochByVirtualIndex", mock.Anything, mock.Anything, uint64(1)).Return(nil, nil)
		repo.On(
			"GetLastInput",
			mock.Anything, applications[1].IApplicationAddress.String(), epochsApp2[0].Index,
		).Return(&input2, nil)

		err = validator.Run(ctx)
		s.NotNil(err)
		s.ErrorContains(err, "claim does not match")
		repo.AssertExpectations(s.T())
	})
}

func randomAddress() common.Address {
	address := make([]byte, 20)
	_, err := crand.Read(address)
	if err != nil {
		panic(err)
	}
	return common.Address(address)
}

func randomHash() common.Hash {
	hash := make([]byte, 32)
	_, err := crand.Read(hash)
	if err != nil {
		panic(err)
	}
	return common.Hash(hash)
}

type Mockrepo struct {
	mock.Mock
}

func newMockrepo() *Mockrepo {
	return new(Mockrepo)
}

func (m *Mockrepo) ListApplications(
	ctx context.Context,
	f repository.ApplicationFilter,
	pagination repository.Pagination,
) ([]*Application, error) {
	args := m.Called(ctx, f, pagination)
	return args.Get(0).([]*Application), args.Error(1)
}

func (m *Mockrepo) ListOutputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.OutputFilter,
	p repository.Pagination,
) ([]*Output, error) {
	args := m.Called(ctx, nameOrAddress, f, p)
	return args.Get(0).([]*Output), args.Error(1)
}

func (m *Mockrepo) ListEpochs(
	ctx context.Context,
	nameOrAddress string,
	f repository.EpochFilter,
	p repository.Pagination,
) ([]*Epoch, error) {
	args := m.Called(ctx, nameOrAddress, f, p)
	return args.Get(0).([]*Epoch), args.Error(1)
}

func (m *Mockrepo) GetLastInput(ctx context.Context, appAddress string, epochIndex uint64) (*Input, error) {
	args := m.Called(ctx, appAddress, epochIndex)
	if input, ok := args.Get(0).(*Input); ok {
		return input, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *Mockrepo) GetEpochByVirtualIndex(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error) {
	args := m.Called(ctx, nameOrAddress, index)
	if epoch, ok := args.Get(0).(*Epoch); ok {
		return epoch, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *Mockrepo) StoreClaimAndProofs(ctx context.Context, epoch *Epoch, outputs []*Output) error {
	args := m.Called(ctx, epoch, outputs)
	return args.Error(0)
}

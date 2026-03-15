// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/events"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SealedEpochsSuite struct {
	suite.Suite
	ctx        context.Context
	cancel     context.CancelFunc
	repository *MockRepository
	inputBox   *MockInputBox
	dave       *MockDaveConsensus
	evmReader  *Service
}

func TestSealedEpochsSuite(t *testing.T) {
	suite.Run(t, new(SealedEpochsSuite))
}

func (s *SealedEpochsSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	config.SetDefaults()
}

func (s *SealedEpochsSuite) TearDownSuite() {
	s.cancel()
}

func (s *SealedEpochsSuite) SetupTest() {
	s.repository = newMockRepository()
	s.inputBox = newMockInputBox()
	s.dave = newMockDaveConsensus()

	s.evmReader = &Service{
		repository:     s.repository,
		publisher:      events.NopPublisher{},
		defaultBlock:   DefaultBlock_Latest,
		hasEnabledApps: true,
	}

	logLevel, err := config.GetLogLevel()
	s.Require().NoError(err)
	serviceArgs := &service.CreateInfo{Name: "evm-reader", Impl: s.evmReader, LogLevel: logLevel}
	err = service.Create(context.Background(), serviceArgs, &s.evmReader.Service)
	s.Require().NoError(err)
}

// TestProcessSealedEpochFindsInputAtOverlapBlock verifies that when an input
// is added at the same block where the previous epoch was sealed (the overlap
// block in PRT's block boundary design), the sealed epoch processing correctly
// finds it. Without the fix, the search would start from lastInputCheckBlock+1,
// skipping the overlap block entirely.
func (s *SealedEpochsSuite) TestProcessSealedEpochFindsInputAtOverlapBlock() {
	const (
		sealBlock0 uint64 = 100 // Epoch 0 sealed here
		sealBlock1 uint64 = 200 // Epoch 1 sealed here
	)

	tournamentAddr := common.HexToAddress("0xAAAA")

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
			DataAvailability:    DataAvailability_InputBox[:],
		},
		inputSource:   s.inputBox,
		daveConsensus: s.dave,
	}

	// Epoch 0 was already stored with LastBlock=100 and InputIndexUpperBound=3.
	// CreateEpochsAndInputs set LastInputCheckBlock=100 for this app.
	//
	// Now epoch 1 is sealed at block 200:
	//   FirstBlock = prevEpoch.LastBlock = 100 (PRT overlap)
	//   InputIndexLowerBound = 3, InputIndexUpperBound = 4
	//
	// Input index 3 was added at block 100 (same block as epoch 0 seal, later tx).
	// The search must start from epoch.FirstBlock=100 (not lastInputCheckBlock+1=101)
	// to find this input.

	sealedEvent := &idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber:          big.NewInt(1),
		InputIndexLowerBound: big.NewInt(3),
		InputIndexUpperBound: big.NewInt(4),
		Tournament:           tournamentAddr,
		Raw: types.Log{
			BlockNumber: sealBlock1,
			TxHash:      common.BigToHash(big.NewInt(999)),
		},
	}

	// Epoch 0 exists (the previous epoch) — needed to compute FirstBlock.
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(&Epoch{
			Index:                0,
			FirstBlock:           10,
			LastBlock:            sealBlock0,
			InputIndexLowerBound: 0,
			InputIndexUpperBound: 3,
		}, nil)
	s.repository.On("UpdateEpochClaimTransactionHash", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	// Epoch 1 does not exist yet (first time seeing it).
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(nil, nil)

	// On-chain: 3 inputs at block 99, 4 inputs from block 100 onward
	// (input 3 was added at block 100, same block as epoch 0 seal).
	s.inputBox.Unset("GetNumberOfInputs")
	s.inputBox.On("GetNumberOfInputs",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() < sealBlock0
		}),
		mock.Anything,
	).Return(big.NewInt(3), nil)
	s.inputBox.On("GetNumberOfInputs",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() >= sealBlock0
		}),
		mock.Anything,
	).Return(big.NewInt(4), nil)

	// Input 3 is at block 100 (the overlap block).
	overlapInput := makeInputEvent(app1Addr, 3, sealBlock0)
	s.inputBox.Unset("RetrieveInputs")
	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == sealBlock0
		}),
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{overlapInput}, nil)

	// No inputs at other blocks in the range.
	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start != sealBlock0
		}),
		mock.Anything,
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{}, nil)

	// CreateEpochsAndInputs captures what was stored.
	var storedInputs []*Input
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		epochInputMap := args.Get(2).(map[*Epoch][]*Input)
		for _, inputs := range epochInputMap {
			storedInputs = inputs
		}
	}).Return(nil)

	err := s.evmReader.processSealedEpochEvent(s.ctx, app, sealedEvent)
	s.Require().NoError(err, "processSealedEpochEvent should succeed when input is at the overlap block")
	s.Require().Len(storedInputs, 1, "should find exactly one input")
	s.Require().Equal(uint64(3), storedInputs[0].Index, "input should have index 3")
	s.Require().Equal(sealBlock0, storedInputs[0].BlockNumber, "input should be at the overlap block")
}

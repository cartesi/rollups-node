// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"context"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
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

	app := newDaveAppContracts(s.inputBox, s.dave)
	app.application.IInputBoxAddress = inputBoxAddr
	app.application.IInputBoxBlock = 10

	// Epoch 0 was already stored with LastBlock=100 and InputIndexUpperBound=3.
	// An earlier open-epoch scan can already have reached block 100, so its
	// input cursor must not determine where this sealed-epoch scan starts.
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

func (s *SealedEpochsSuite) TestCatchUpForeclosedSealedEpochsAdvancesCursor() {
	const (
		lastEpochCheckBlock uint64 = 50
		forecloseBlock      uint64 = 70
	)

	app := newDaveAppContracts(s.inputBox, s.dave)
	app.application.Name = "test-prt-app"
	app.application.ConsensusType = Consensus_PRT
	app.application.ForecloseBlock = forecloseBlock
	app.application.LastEpochCheckBlock = lastEpochCheckBlock
	app.application.LastInputCheckBlock = forecloseBlock
	app.application.LastOutputCheckBlock = lastEpochCheckBlock

	currentSealedEpoch := DaveCurrentSealedEpoch{
		EpochNumber:          big.NewInt(2),
		InputIndexLowerBound: big.NewInt(0),
		InputIndexUpperBound: big.NewInt(0),
		Tournament:           common.Address{},
	}
	s.dave.On("GetCurrentSealedEpoch", mock.Anything).
		Return(currentSealedEpoch, nil)

	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything,
		[]int64{app.application.ID},
		MonitoredEvent_EpochSealed,
		forecloseBlock,
	).Return(nil).Once()

	s.evmReader.scanDaveConsensusEpochsAndInputs(s.ctx, []appContracts{app}, forecloseBlock+10)

	s.repository.AssertExpectations(s.T())
	s.dave.AssertExpectations(s.T())
}

func (s *SealedEpochsSuite) TestTerminalDaveConsensusAppProcessesOpenEpochToForeclosure() {
	const forecloseBlock uint64 = 70

	app := newDaveAppContracts(s.inputBox, s.dave)
	app.application.Name = "test-prt-app"
	app.application.ConsensusType = Consensus_PRT
	app.application.Status = ApplicationStatus_MachineHalted
	app.application.ForecloseBlock = forecloseBlock
	app.application.LastEpochCheckBlock = forecloseBlock
	app.application.LastInputCheckBlock = forecloseBlock - 1

	s.repository.On("GetLastNonOpenEpoch",
		mock.Anything, app.application.IApplicationAddress.String()).
		Return(&Epoch{
			Index:                2,
			LastBlock:            50,
			InputIndexUpperBound: 0,
		}, nil).Once()
	s.repository.On("GetEpoch",
		mock.Anything, app.application.IApplicationAddress.Hex(), uint64(3)).
		Return(nil, nil).Once()
	s.repository.On("GetEventLastCheckBlock",
		mock.Anything, app.application.ID, MonitoredEvent_InputAdded).
		Return(forecloseBlock-1, nil).Once()
	s.repository.On("GetNumberOfInputs",
		mock.Anything, app.application.IApplicationAddress.String()).
		Return(uint64(0), nil).Once()
	s.inputBox.On("GetNumberOfInputs", mock.Anything, app.application.IApplicationAddress).
		Return(big.NewInt(0), nil)
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything,
		app.application.IApplicationAddress.String(),
		mock.MatchedBy(func(epochInputs map[*Epoch][]*Input) bool {
			if len(epochInputs) != 1 {
				return false
			}
			for epoch, inputs := range epochInputs {
				return epoch.Status == EpochStatus_Open &&
					epoch.Index == 3 &&
					epoch.LastBlock == forecloseBlock &&
					len(inputs) == 0
			}
			return false
		}),
		forecloseBlock,
	).Return(nil).Once()

	s.evmReader.scanDaveConsensusEpochsAndInputs(s.ctx, []appContracts{app}, forecloseBlock+10)

	s.repository.AssertExpectations(s.T())
	s.inputBox.AssertExpectations(s.T())
	s.dave.AssertNumberOfCalls(s.T(), "GetCurrentSealedEpoch", 0)
}

func (s *SealedEpochsSuite) TestDaveConsensusWithMissingInputBoxAdapterDoesNotPanic() {
	var logs bytes.Buffer
	s.evmReader.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "missing-input-box-adapter-prt-app",
			IApplicationAddress: app1Addr,
			IInputBoxAddress:    inputBoxAddr,
			ConsensusType:       Consensus_PRT,
			Status:              ApplicationStatus_OK,
		},
		daveConsensus: s.dave,
	}

	s.NotPanics(func() {
		s.evmReader.scanDaveConsensusEpochsAndInputs(
			s.ctx, []appContracts{app}, 100)
	})
	s.Contains(logs.String(), "InputBox adapter is missing")
	s.Equal(ApplicationStatus_OK, app.application.Status)
	s.dave.AssertNotCalled(s.T(), "GetCurrentSealedEpoch")
	s.repository.AssertNumberOfCalls(s.T(), "UpdateApplicationStatus", 0)
}

func (s *SealedEpochsSuite) TestSealedEpochAtGenesisDoesNotPublishCursor() {
	event := makeSealedEpochEvent(0, 0, 0, 0, common.HexToAddress("0xAAAA"))
	err := s.evmReader.processSealedEpochEvent(s.ctx, appContracts{}, event)
	s.Require().ErrorContains(err, "sealed epoch event has block number zero")
	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
}

func (s *SealedEpochsSuite) TestSealedEpochAtFirstBlockPublishesZeroCursor() {
	app := appContracts{application: &Application{IApplicationAddress: app1Addr, IInputBoxBlock: 1}}
	event := makeSealedEpochEvent(0, 0, 0, 1, common.HexToAddress("0xAAAA"))
	s.repository.On("GetEpoch", mock.Anything, app1Addr.Hex(), uint64(0)).Return(nil, nil).Once()
	s.repository.On("CreateEpochsAndInputs", mock.Anything, app1Addr.Hex(),
		mock.MatchedBy(func(epochs map[*Epoch][]*Input) bool {
			if len(epochs) != 1 {
				return false
			}
			for epoch, inputs := range epochs {
				return epoch.FirstBlock == 1 && epoch.LastBlock == 1 && len(inputs) == 0
			}
			return false
		}), uint64(0)).Return(nil).Once()
	s.Require().NoError(s.evmReader.processSealedEpochEvent(s.ctx, app, event))
	s.repository.AssertExpectations(s.T())
}

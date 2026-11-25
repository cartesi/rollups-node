// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
)

const daveTestApplicationName = "test-app"

// Keep scan boundaries, InputBox identity, and application status in each test.
func newDaveAppContracts(inputSource InputSourceAdapter, daveConsensus DaveConsensusAdapter) appContracts {
	return appContracts{
		application: &Application{
			ID: 1, Name: daveTestApplicationName,
			IApplicationAddress: app1Addr, IConsensusAddress: consensusAddr,
		},
		inputSource: inputSource, daveConsensus: daveConsensus,
	}
}

// makeSealedEpochResult constructs the node-owned GetCurrentSealedEpoch result.
func makeSealedEpochResult(
	epochNum int64,
	lowerBound, upperBound uint64,
	tournament common.Address,
) DaveCurrentSealedEpoch {
	return DaveCurrentSealedEpoch{
		EpochNumber:          big.NewInt(epochNum),
		InputIndexLowerBound: new(big.Int).SetUint64(lowerBound),
		InputIndexUpperBound: new(big.Int).SetUint64(upperBound),
		Tournament:           tournament,
	}
}

// makeSealedEpochEvent constructs an IDaveConsensusEpochSealed event for testing.
func makeSealedEpochEvent(
	epochNum int64,
	lowerBound, upperBound, blockNum uint64,
	tournament common.Address,
) *idaveconsensus.IDaveConsensusEpochSealed {
	return &idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber:          big.NewInt(epochNum),
		InputIndexLowerBound: new(big.Int).SetUint64(lowerBound),
		InputIndexUpperBound: new(big.Int).SetUint64(upperBound),
		Tournament:           tournament,
		Raw: types.Log{
			BlockNumber: blockNum,
			TxHash:      common.BigToHash(new(big.Int).SetUint64(blockNum)),
		},
	}
}

// --- Test 1: Full sealed pipeline with 2 epoch transitions (no inputs) ---
// Exercises: processApplicationSealedEpochs → FindTransitions → processEpochTransition
// → processSealedEpochEvent, verifying the complete DaveConsensus sealed epoch pipeline.
func (s *SealedEpochsSuite) TestSealedEpochsEndToEndTwoTransitions() {
	const (
		inputBoxBlock   uint64 = 10
		searchStart     uint64 = 50
		sealBlock0      uint64 = 100
		sealBlock1      uint64 = 150
		mostRecentBlock uint64 = 200
	)

	tournamentAddr0 := common.HexToAddress("0xA000")
	tournamentAddr1 := common.HexToAddress("0xA001")

	app := newDaveAppContracts(s.inputBox, s.dave)
	app.application.IInputBoxAddress = inputBoxAddr
	app.application.IInputBoxBlock = inputBoxBlock
	app.application.LastEpochCheckBlock = searchStart

	// Oracle: GetCurrentSealedEpoch returns epoch number based on block
	s.dave.On("GetCurrentSealedEpoch",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Int64() < int64(sealBlock0)
		}),
	).Return(makeSealedEpochResult(-1, 0, 0, common.Address{}), nil)

	s.dave.On("GetCurrentSealedEpoch",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			b := opts.BlockNumber.Uint64()
			return b >= sealBlock0 && b < sealBlock1
		}),
	).Return(makeSealedEpochResult(0, 0, 0, tournamentAddr0), nil)

	s.dave.On("GetCurrentSealedEpoch",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() >= sealBlock1
		}),
	).Return(makeSealedEpochResult(1, 0, 0, tournamentAddr1), nil)

	// RetrieveSealedEpochs at each transition block
	s.dave.On("RetrieveSealedEpochs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == sealBlock0 }),
	).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
		makeSealedEpochEvent(0, 0, 0, sealBlock0, tournamentAddr0),
	}, nil)

	s.dave.On("RetrieveSealedEpochs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == sealBlock1 }),
	).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
		makeSealedEpochEvent(1, 0, 0, sealBlock1, tournamentAddr1),
	}, nil)

	// Epoch 0: first lookup → nil (new epoch)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(nil, nil).Once()
	// Epoch 1: lookup of prev epoch (0) → returns epoch 0 data
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(&Epoch{
			Index: 0, FirstBlock: inputBoxBlock, LastBlock: sealBlock0,
			InputIndexLowerBound: 0, InputIndexUpperBound: 0,
		}, nil)
	// Epoch 1: self-lookup → nil (new epoch)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(nil, nil)

	s.repository.On("UpdateEpochClaimTransactionHash",
		mock.Anything, mock.Anything, mock.Anything,
	).Return(nil)

	var storedEpochs []*Epoch
	s.repository.On("AttributeSealedEpoch",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		storedEpochs = append(storedEpochs, args.Get(2).(*Epoch))
	}).Return(nil)

	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything, mock.Anything, MonitoredEvent_EpochSealed, mostRecentBlock,
	).Return(nil)

	err := s.evmReader.processApplicationSealedEpochs(s.ctx, app, mostRecentBlock)
	s.Require().NoError(err)

	// Two epochs stored
	s.Require().Len(storedEpochs, 2)
	// Sort by index for deterministic assertions
	if storedEpochs[0].Index > storedEpochs[1].Index {
		storedEpochs[0], storedEpochs[1] = storedEpochs[1], storedEpochs[0]
	}

	// Epoch 0
	s.Require().Equal(uint64(0), storedEpochs[0].Index)
	s.Require().Equal(inputBoxBlock, storedEpochs[0].FirstBlock)
	s.Require().Equal(sealBlock0, storedEpochs[0].LastBlock)
	s.Require().Equal(EpochStatus_Closed, storedEpochs[0].Status)
	s.Require().Equal(&tournamentAddr0, storedEpochs[0].TournamentAddress)

	// Epoch 1 (PRT overlap: FirstBlock = prevEpoch.LastBlock)
	s.Require().Equal(uint64(1), storedEpochs[1].Index)
	s.Require().Equal(sealBlock0, storedEpochs[1].FirstBlock)
	s.Require().Equal(sealBlock1, storedEpochs[1].LastBlock)
	s.Require().Equal(EpochStatus_Closed, storedEpochs[1].Status)
	s.Require().Equal(&tournamentAddr1, storedEpochs[1].TournamentAddress)

	s.repository.AssertNumberOfCalls(s.T(), "AttributeSealedEpoch", 2)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 1)
}

// --- Test 2: Open epoch happy path — new epoch with 1 input ---
// Exercises projectDaveOpenEpoch creating a new open epoch and fetching inputs.
func (s *SealedEpochsSuite) TestOpenEpochHappyPathCreatesNewEpoch() {
	const (
		sealBlock       uint64 = 150
		mostRecentBlock uint64 = 200
	)

	app := newDaveAppContracts(s.inputBox, s.dave)
	app.application.IInputBoxAddress = inputBoxAddr
	app.application.IInputBoxBlock = 10
	app.application.LastInputCheckBlock = sealBlock

	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(&Epoch{
			Index: 0, FirstBlock: 10, LastBlock: sealBlock,
			InputIndexLowerBound: 0, InputIndexUpperBound: 3,
		}, nil)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(nil, nil)
	s.repository.On("GetEventLastCheckBlock",
		mock.Anything, int64(1), MonitoredEvent_InputAdded,
	).Return(sealBlock, nil)
	s.repository.On("GetInputObservation", mock.Anything, app1Addr.Hex()).
		Return(&InputObservation{ThroughBlock: mostRecentBlock, NextInputIndex: 4}, nil)

	event := makeInputEvent(app1Addr, 3, sealBlock)
	observed := &Input{
		Index: 3, BlockNumber: sealBlock, RawData: event.Input,
		Status: InputCompletionStatus_None, TransactionHash: event.Raw.TxHash,
	}
	s.repository.On("ListObservedInputs", mock.Anything, app1Addr.Hex(), uint64(3), uint64(4)).
		Return([]*Input{observed}, nil)

	var storedEpoch *Epoch
	var storedInputs []*Input
	s.repository.On("ProjectDaveOpenInputs", mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mostRecentBlock,
	).Run(func(args mock.Arguments) {
		storedEpoch = args.Get(2).(*Epoch)
		storedInputs = args.Get(3).([]*Input)
	}).Return(nil)

	frontier, err := s.repository.GetInputObservation(s.ctx, app1Addr.Hex())
	s.Require().NoError(err)
	err = s.evmReader.projectDaveOpenEpoch(s.ctx, app, mostRecentBlock, frontier)
	s.Require().NoError(err)

	s.Require().NotNil(storedEpoch)
	s.Require().Equal(uint64(1), storedEpoch.Index)
	s.Require().Equal(sealBlock, storedEpoch.FirstBlock) // PRT overlap
	s.Require().Equal(mostRecentBlock, storedEpoch.LastBlock)
	s.Require().Equal(uint64(3), storedEpoch.InputIndexLowerBound)
	s.Require().Equal(uint64(4), storedEpoch.InputIndexUpperBound)
	s.Require().Equal(EpochStatus_Open, storedEpoch.Status)
	s.Require().Len(storedInputs, 1)
	s.Require().Equal(uint64(3), storedInputs[0].Index)
	s.Require().Equal(sealBlock, storedInputs[0].BlockNumber)
}

func (s *SealedEpochsSuite) TestOpenEpochExistingEpochAccumulatesInputs() {
	const mostRecentBlock uint64 = 300

	app := newDaveAppContracts(s.inputBox, s.dave)
	app.application.IInputBoxAddress = inputBoxAddr
	app.application.IInputBoxBlock = 10
	app.application.LastInputCheckBlock = 200

	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(&Epoch{
			Index: 0, FirstBlock: 10, LastBlock: 100,
			InputIndexLowerBound: 0, InputIndexUpperBound: 3,
		}, nil)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(&Epoch{
			Index: 1, FirstBlock: 100, LastBlock: 200,
			InputIndexLowerBound: 3, InputIndexUpperBound: 4,
			Status: EpochStatus_Open,
		}, nil)
	s.repository.On("GetEventLastCheckBlock",
		mock.Anything, int64(1), MonitoredEvent_InputAdded,
	).Return(uint64(200), nil)
	s.repository.On("GetInputObservation", mock.Anything, app1Addr.Hex()).
		Return(&InputObservation{ThroughBlock: mostRecentBlock, NextInputIndex: 5}, nil)

	observedEvent := makeInputEvent(app1Addr, 4, 250)
	observed := &Input{
		Index: 4, BlockNumber: 250, RawData: observedEvent.Input,
		Status: InputCompletionStatus_None, TransactionHash: observedEvent.Raw.TxHash,
	}
	// Input 3 was stored and verified by the previous projection, so only the
	// newly grown prefix [4, 5) is collected.
	s.repository.On("ListObservedInputs", mock.Anything, app1Addr.Hex(), uint64(4), uint64(5)).
		Return([]*Input{observed}, nil)

	var storedEpoch *Epoch
	var storedInputs []*Input
	s.repository.On("ProjectDaveOpenInputs", mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mostRecentBlock,
	).Run(func(args mock.Arguments) {
		storedEpoch = args.Get(2).(*Epoch)
		storedInputs = args.Get(3).([]*Input)
	}).Return(nil)

	frontier, err := s.repository.GetInputObservation(s.ctx, app1Addr.Hex())
	s.Require().NoError(err)
	err = s.evmReader.projectDaveOpenEpoch(s.ctx, app, mostRecentBlock, frontier)
	s.Require().NoError(err)

	s.Require().Equal(uint64(1), storedEpoch.Index)
	s.Require().Equal(uint64(100), storedEpoch.FirstBlock)         // Preserved
	s.Require().Equal(mostRecentBlock, storedEpoch.LastBlock)      // Updated
	s.Require().Equal(uint64(3), storedEpoch.InputIndexLowerBound) // Preserved
	s.Require().Equal(uint64(5), storedEpoch.InputIndexUpperBound) // 4 + 1 new
	s.Require().Equal(EpochStatus_Open, storedEpoch.Status)
	s.Require().Len(storedInputs, 1)
	s.Require().Equal(uint64(4), storedInputs[0].Index)
}

func (s *SealedEpochsSuite) TestOpenEpochScansBoundaryBlockAfterIntraTickCursorAdvance() {
	const (
		previousInputCursor uint64 = 100
		mostRecentBlock     uint64 = 200
	)

	app := newDaveAppContracts(s.inputBox, s.dave)
	app.application.IInputBoxAddress = inputBoxAddr
	app.application.IInputBoxBlock = 10
	app.application.LastInputCheckBlock = previousInputCursor

	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(&Epoch{
			Index:                0,
			FirstBlock:           10,
			LastBlock:            mostRecentBlock,
			InputIndexLowerBound: 0,
			InputIndexUpperBound: 4,
		}, nil)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(nil, nil)
	s.repository.On("GetEventLastCheckBlock",
		mock.Anything, int64(1), MonitoredEvent_InputAdded,
	).Return(mostRecentBlock-1, nil)
	s.repository.On("GetInputObservation", mock.Anything, app1Addr.Hex()).
		Return(&InputObservation{ThroughBlock: mostRecentBlock, NextInputIndex: 5}, nil)

	event := makeInputEvent(app1Addr, 4, mostRecentBlock)
	observed := &Input{
		Index: 4, BlockNumber: mostRecentBlock, RawData: event.Input,
		Status: InputCompletionStatus_None, TransactionHash: event.Raw.TxHash,
	}
	s.repository.On("ListObservedInputs", mock.Anything, app1Addr.Hex(), uint64(4), uint64(5)).
		Return([]*Input{observed}, nil)

	var storedEpoch *Epoch
	var storedInputs []*Input
	s.repository.On("ProjectDaveOpenInputs", mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mostRecentBlock,
	).Run(func(args mock.Arguments) {
		storedEpoch = args.Get(2).(*Epoch)
		storedInputs = args.Get(3).([]*Input)
	}).Return(nil)

	frontier, err := s.repository.GetInputObservation(s.ctx, app1Addr.Hex())
	s.Require().NoError(err)
	err = s.evmReader.projectDaveOpenEpoch(s.ctx, app, mostRecentBlock, frontier)
	s.Require().NoError(err)

	s.Require().NotNil(storedEpoch)
	s.Require().Equal(uint64(1), storedEpoch.Index)
	s.Require().Equal(mostRecentBlock, storedEpoch.FirstBlock)
	s.Require().Equal(mostRecentBlock, storedEpoch.LastBlock)
	s.Require().Equal(uint64(5), storedEpoch.InputIndexUpperBound)
	s.Require().Len(storedInputs, 1)
	s.Require().Equal(uint64(4), storedInputs[0].Index)
	s.Require().Equal(mostRecentBlock, storedInputs[0].BlockNumber)
}

// --- Test 4: Multiple EpochSealed events in one block ---
// When two epochs are sealed in the same block, FindTransitions sees a single
// transition, but RetrieveSealedEpochs returns both events and each is processed.
func (s *SealedEpochsSuite) TestMultipleSealedEpochsInOneBlock() {
	const (
		sealBlock       uint64 = 100
		mostRecentBlock uint64 = 200
	)

	tournamentAddr0 := common.HexToAddress("0xA000")
	tournamentAddr1 := common.HexToAddress("0xA001")

	app := newDaveAppContracts(s.inputBox, s.dave)
	app.application.IInputBoxAddress = inputBoxAddr
	app.application.IInputBoxBlock = 10
	app.application.LastEpochCheckBlock = 50

	// Oracle: before sealBlock → -1, from sealBlock → 1 (both sealed at same block)
	s.dave.On("GetCurrentSealedEpoch",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Int64() < int64(sealBlock)
		}),
	).Return(makeSealedEpochResult(-1, 0, 0, common.Address{}), nil)

	s.dave.On("GetCurrentSealedEpoch",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() >= sealBlock
		}),
	).Return(makeSealedEpochResult(1, 0, 0, tournamentAddr1), nil)

	// Both sealed events at the same block
	s.dave.On("RetrieveSealedEpochs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == sealBlock }),
	).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
		makeSealedEpochEvent(0, 0, 0, sealBlock, tournamentAddr0),
		makeSealedEpochEvent(1, 0, 0, sealBlock, tournamentAddr1),
	}, nil)

	// Epoch 0: self-lookup → nil (new)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(nil, nil).Once()
	// Epoch 1: prev epoch lookup → epoch 0 data
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(&Epoch{
			Index: 0, FirstBlock: 10, LastBlock: sealBlock,
			InputIndexLowerBound: 0, InputIndexUpperBound: 0,
		}, nil)
	// Epoch 1: self-lookup → nil (new)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(nil, nil)

	s.repository.On("UpdateEpochClaimTransactionHash",
		mock.Anything, mock.Anything, mock.Anything,
	).Return(nil)

	var storedEpochs []*Epoch
	s.repository.On("AttributeSealedEpoch",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		storedEpochs = append(storedEpochs, args.Get(2).(*Epoch))
	}).Return(nil)

	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything, mock.Anything, MonitoredEvent_EpochSealed, mostRecentBlock,
	).Return(nil)

	err := s.evmReader.processApplicationSealedEpochs(s.ctx, app, mostRecentBlock)
	s.Require().NoError(err)

	s.Require().Len(storedEpochs, 2)
	if storedEpochs[0].Index > storedEpochs[1].Index {
		storedEpochs[0], storedEpochs[1] = storedEpochs[1], storedEpochs[0]
	}

	s.Require().Equal(uint64(0), storedEpochs[0].Index)
	s.Require().Equal(uint64(10), storedEpochs[0].FirstBlock)
	s.Require().Equal(sealBlock, storedEpochs[0].LastBlock)

	s.Require().Equal(uint64(1), storedEpochs[1].Index)
	s.Require().Equal(sealBlock, storedEpochs[1].FirstBlock) // PRT overlap
	s.Require().Equal(sealBlock, storedEpochs[1].LastBlock)  // Same block

	s.repository.AssertNumberOfCalls(s.T(), "AttributeSealedEpoch", 2)
}

// The deployment floor does not certify a successful scan or change its cursor.
func (s *SealedEpochsSuite) TestInitialSealedEpochSearchBlockDoesNotSetCheckpoint() {
	app := newDaveAppContracts(nil, s.dave)

	// DaveConsensus deployed at block 50
	s.dave.On("GetDeploymentBlockNumber", mock.Anything).
		Return(big.NewInt(50), nil)

	block, err := s.evmReader.initialSealedEpochSearchBlock(s.ctx, app, 200)
	s.Require().NoError(err)
	s.Equal(uint64(50), block)
	s.Zero(app.application.LastEpochCheckBlock)
	s.Empty(s.repository.Calls)
}

func (s *SealedEpochsSuite) TestInitializeSealedEpochSyncSkipsBeforeConsensusDeployment() {
	app := newDaveAppContracts(nil, s.dave)

	s.dave.On("GetDeploymentBlockNumber",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber != nil && opts.BlockNumber.Uint64() == 90
		}),
	).Return(new(big.Int), bind.ErrNoCode).Once()

	err := s.evmReader.processApplicationSealedEpochs(s.ctx, app, 90)
	s.Require().ErrorIs(err, errContractNotDeployedAtBlock)
	s.Require().ErrorIs(err, bind.ErrNoCode)
	s.Require().Zero(app.application.LastEpochCheckBlock)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
}

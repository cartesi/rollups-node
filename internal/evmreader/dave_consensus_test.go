// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
)

// makeSealedEpochResult constructs the anonymous struct returned by GetCurrentSealedEpoch.
func makeSealedEpochResult(
	epochNum int64,
	lowerBound, upperBound uint64,
	tournament common.Address,
) struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
} {
	return struct {
		EpochNumber          *big.Int
		InputIndexLowerBound *big.Int
		InputIndexUpperBound *big.Int
		Tournament           common.Address
	}{
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

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      inputBoxBlock,
			LastEpochCheckBlock: searchStart,
		},
		inputSource:   s.inputBox,
		daveConsensus: s.dave,
	}

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

	// No previous sealed epochs in DB
	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(nil, nil)

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
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		epochInputMap := args.Get(2).(map[*Epoch][]*Input)
		for epoch := range epochInputMap {
			storedEpochs = append(storedEpochs, epoch)
		}
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

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 2)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 1)
}

// --- Test 2: Open epoch happy path — new epoch with 1 input ---
// Exercises processApplicationOpenEpoch creating a new open epoch and fetching inputs.
func (s *SealedEpochsSuite) TestOpenEpochHappyPathCreatesNewEpoch() {
	const (
		sealBlock       uint64 = 100
		mostRecentBlock uint64 = 200
	)

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
		},
		inputSource:   s.inputBox,
		daveConsensus: s.dave,
	}

	// Last sealed epoch
	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(&Epoch{
			Index: 0, FirstBlock: 10, LastBlock: sealBlock,
			InputIndexLowerBound: 0, InputIndexUpperBound: 3,
		}, nil)

	// Open epoch (index 1) doesn't exist yet
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(nil, nil)

	// Last input check block
	s.repository.On("GetEventLastCheckBlock",
		mock.Anything, int64(1), MonitoredEvent_InputAdded,
	).Return(sealBlock, nil)

	// DB has 3 inputs
	s.repository.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(uint64(3), nil)

	// On-chain: 3 inputs before block 150, 4 from block 150
	s.inputBox.On("GetNumberOfInputs",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() < 150
		}),
		mock.Anything,
	).Return(big.NewInt(3), nil)
	s.inputBox.On("GetNumberOfInputs",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() >= 150
		}),
		mock.Anything,
	).Return(big.NewInt(4), nil)

	// Input 3 at block 150
	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 150 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{makeInputEvent(app1Addr, 3, 150)}, nil)

	var storedEpoch *Epoch
	var storedInputs []*Input
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		epochInputMap := args.Get(2).(map[*Epoch][]*Input)
		for epoch, inputs := range epochInputMap {
			storedEpoch = epoch
			storedInputs = inputs
		}
	}).Return(nil)

	err := s.evmReader.processApplicationOpenEpoch(s.ctx, app, mostRecentBlock)
	s.Require().NoError(err)

	s.Require().NotNil(storedEpoch)
	s.Require().Equal(uint64(1), storedEpoch.Index)
	s.Require().Equal(sealBlock, storedEpoch.FirstBlock) // PRT overlap
	s.Require().Equal(mostRecentBlock, storedEpoch.LastBlock)
	s.Require().Equal(uint64(3), storedEpoch.InputIndexLowerBound)
	s.Require().Equal(uint64(4), storedEpoch.InputIndexUpperBound) // 3 + 1 new
	s.Require().Equal(EpochStatus_Open, storedEpoch.Status)

	s.Require().Len(storedInputs, 1)
	s.Require().Equal(uint64(3), storedInputs[0].Index)
	s.Require().Equal(uint64(150), storedInputs[0].BlockNumber)
}

// --- Test 3: Open epoch with existing epoch accumulates inputs ---
// When processApplicationOpenEpoch finds an existing open epoch in the DB,
// it must reuse it and add new inputs (incrementing InputIndexUpperBound).
func (s *SealedEpochsSuite) TestOpenEpochExistingEpochAccumulatesInputs() {
	const mostRecentBlock uint64 = 300

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
		},
		inputSource:   s.inputBox,
		daveConsensus: s.dave,
	}

	// Last sealed epoch
	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(&Epoch{
			Index: 0, FirstBlock: 10, LastBlock: 100,
			InputIndexLowerBound: 0, InputIndexUpperBound: 3,
		}, nil)

	// Open epoch already exists from a previous tick
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(&Epoch{
			Index: 1, FirstBlock: 100, LastBlock: 200,
			InputIndexLowerBound: 3, InputIndexUpperBound: 4,
			Status: EpochStatus_Open,
		}, nil)

	// Last input check at block 200
	s.repository.On("GetEventLastCheckBlock",
		mock.Anything, int64(1), MonitoredEvent_InputAdded,
	).Return(uint64(200), nil)

	// DB has 4 inputs
	s.repository.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(uint64(4), nil)

	// On-chain: 4 inputs before block 250, 5 from block 250
	s.inputBox.On("GetNumberOfInputs",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() < 250
		}),
		mock.Anything,
	).Return(big.NewInt(4), nil)
	s.inputBox.On("GetNumberOfInputs",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() >= 250
		}),
		mock.Anything,
	).Return(big.NewInt(5), nil)

	// Input 4 at block 250
	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 250 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{makeInputEvent(app1Addr, 4, 250)}, nil)

	var storedEpoch *Epoch
	var storedInputs []*Input
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		epochInputMap := args.Get(2).(map[*Epoch][]*Input)
		for epoch, inputs := range epochInputMap {
			storedEpoch = epoch
			storedInputs = inputs
		}
	}).Return(nil)

	err := s.evmReader.processApplicationOpenEpoch(s.ctx, app, mostRecentBlock)
	s.Require().NoError(err)

	// Existing epoch reused with accumulated inputs
	s.Require().Equal(uint64(1), storedEpoch.Index)
	s.Require().Equal(uint64(100), storedEpoch.FirstBlock)         // Preserved
	s.Require().Equal(mostRecentBlock, storedEpoch.LastBlock)       // Updated
	s.Require().Equal(uint64(3), storedEpoch.InputIndexLowerBound) // Preserved
	s.Require().Equal(uint64(5), storedEpoch.InputIndexUpperBound) // 4 + 1 new
	s.Require().Equal(EpochStatus_Open, storedEpoch.Status)

	s.Require().Len(storedInputs, 1)
	s.Require().Equal(uint64(4), storedInputs[0].Index)
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

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
			LastEpochCheckBlock: 50,
		},
		inputSource:   s.inputBox,
		daveConsensus: s.dave,
	}

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

	// No previous sealed epochs
	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(nil, nil)

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
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Run(func(args mock.Arguments) {
		epochInputMap := args.Get(2).(map[*Epoch][]*Input)
		for epoch := range epochInputMap {
			storedEpochs = append(storedEpochs, epoch)
		}
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

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 2)
}

// --- Test 5: initializeNewApplicationSealedEpochSync sets checkpoint ---
// On first run, the deployment block is fetched from DaveConsensus and the
// LastEpochCheckBlock is set to deploymentBlock - 1.
func (s *SealedEpochsSuite) TestInitializeSealedEpochSyncSetsCheckpoint() {
	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
		},
		daveConsensus: s.dave,
	}

	// DaveConsensus deployed at block 50
	s.dave.On("GetDeploymentBlockNumber", mock.Anything).
		Return(big.NewInt(50), nil)

	s.repository.On("UpdateEventLastCheckBlock",
		mock.Anything, []int64{int64(1)}, MonitoredEvent_EpochSealed, uint64(49),
	).Return(nil)

	err := s.evmReader.initializeNewApplicationSealedEpochSync(s.ctx, &app, 200)
	s.Require().NoError(err)

	// LastEpochCheckBlock set to deploymentBlock - 1
	s.Require().Equal(uint64(49), app.application.LastEpochCheckBlock)
	s.repository.AssertExpectations(s.T())
}

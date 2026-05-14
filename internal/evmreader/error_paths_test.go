// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Priority 1: CreateEpochsAndInputs error → checkpoint not advanced ---
// When CreateEpochsAndInputs fails for an app with inputs, the checkpoint must
// NOT be advanced. Apps with inputs are excluded from the no-input checkpoint
// update path, so no UpdateEventLastCheckBlock call should occur.
func (s *EvmReaderSuite) TestCreateEpochsAndInputsErrorDoesNotAdvanceCheckpoint() {
	require := require.New(s.T())

	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")

	app := &Application{
		ID:                  1,
		Name:                "test-app",
		IApplicationAddress: addr,
		IInputBoxAddress:    inputBoxAddr,
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}

	// Input source: 0 inputs before block 105, 1 from block 105
	inputSrc := &MockInputBox{}
	inputSrc.On("GetNumberOfInputs", blockRange(0, 105), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	inputSrc.On("GetNumberOfInputs", blockFrom(105), mock.Anything).
		Return(new(big.Int).SetUint64(1), nil)
	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 105 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{makeInputEvent(addr, 0, 105)}, nil)

	apps := []appContracts{
		{application: app, inputSource: inputSrc},
	}

	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(uint64(0), nil)
	repo.On("GetEpoch", mock.Anything, mock.Anything, uint64(10)).
		Return(nil, nil)
	repo.On("CreateEpochsAndInputs",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(errors.New("database connection lost"))

	s.evmReader.repository = repo

	err := s.evmReader.readAndStoreInputs(s.ctx, 100, 110, apps)
	require.NoError(err) // per-app failure doesn't abort

	// CreateEpochsAndInputs was attempted
	repo.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 1)
	// Checkpoint must NOT be advanced
	repo.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
}

// --- Priority 2: Sealed epoch input count mismatch → error, no epoch stored ---
// When fetchInputs returns fewer inputs than the sealed event
// declares, processSealedEpochEvent must return an error and NOT store the epoch.
func (s *SealedEpochsSuite) TestSealedEpochInputCountMismatchReturnsError() {
	const sealBlock uint64 = 200
	tournamentAddr := common.HexToAddress("0xAAAA")

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

	// Sealed event expects 2 inputs (indices 3 to 5), but only 1 exists on-chain.
	event := &idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber:          big.NewInt(1),
		InputIndexLowerBound: big.NewInt(3),
		InputIndexUpperBound: big.NewInt(5),
		Tournament:           tournamentAddr,
		Raw:                  types.Log{BlockNumber: sealBlock},
	}

	// Previous epoch (index 0)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(&Epoch{
			Index: 0, FirstBlock: 10, LastBlock: 100,
			InputIndexLowerBound: 0, InputIndexUpperBound: 3,
		}, nil)
	s.repository.On("UpdateEpochClaimTransactionHash",
		mock.Anything, mock.Anything, mock.Anything).Return(nil)
	// Epoch 1 doesn't exist yet
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(nil, nil)

	// On-chain: only 4 inputs total (1 new instead of expected 2)
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
	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == 150
		}),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{
		makeInputEvent(app1Addr, 3, 150),
	}, nil)

	err := s.evmReader.processSealedEpochEvent(s.ctx, app, event)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "input count mismatch")
	s.Require().ErrorContains(err, "expected 2")
	s.Require().ErrorContains(err, "got 1")

	// Epoch must NOT be stored
	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
}

// --- Priority 3: Existing epoch FirstBlock mismatch → error ---
// When a sealed epoch already exists in the DB with a different FirstBlock
// than what the previous epoch's LastBlock implies, processSealedEpochEvent
// must return a data mismatch error.
func (s *SealedEpochsSuite) TestSealedEpochFirstBlockMismatchReturnsError() {
	tournamentAddr := common.HexToAddress("0xBBBB")

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
			IInputBoxAddress:    inputBoxAddr,
		},
		inputSource:   s.inputBox,
		daveConsensus: s.dave,
	}

	// Epoch 1 sealed. Previous epoch (0) has LastBlock=100 → firstBlock should be 100.
	event := &idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber:          big.NewInt(1),
		InputIndexLowerBound: big.NewInt(3),
		InputIndexUpperBound: big.NewInt(3), // no inputs
		Tournament:           tournamentAddr,
		Raw:                  types.Log{BlockNumber: 200},
	}

	// Previous epoch
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(&Epoch{
			Index: 0, FirstBlock: 10, LastBlock: 100,
			InputIndexLowerBound: 0, InputIndexUpperBound: 3,
		}, nil)
	s.repository.On("UpdateEpochClaimTransactionHash",
		mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Existing epoch 1 has WRONG FirstBlock (50 instead of 100)
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(&Epoch{
			Index:                1,
			FirstBlock:           50, // should be 100 (prevEpoch.LastBlock)
			InputIndexLowerBound: 3,
		}, nil)

	err := s.evmReader.processSealedEpochEvent(s.ctx, app, event)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "data mismatch")

	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
}

// --- Priority 4: GetLastNonOpenEpoch nil → app set corrupted ---
// When processApplicationOpenEpoch finds no non-open epoch, the application
// must be set corrupted. This is an invariant of the DaveConsensus protocol.
func (s *SealedEpochsSuite) TestOpenEpochWithNoNonOpenEpochSetsCorrupted() {
	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
		},
		inputSource: s.inputBox,
	}

	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(nil, nil)
	s.repository.On("UpdateApplicationStatus",
		mock.Anything, int64(1), ApplicationStatus_Corrupted, mock.Anything,
	).Return(nil).Once()

	err := s.evmReader.processApplicationOpenEpoch(s.ctx, app, 200)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "no non open epochs found")

	s.repository.AssertExpectations(s.T())
	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
}

// --- Priority 5: Epoch 0 with IInputBoxBlock=0 → error ---
// When processing sealed epoch 0 for an app without a configured InputBox
// block number, processSealedEpochEvent must return an error immediately.
func (s *SealedEpochsSuite) TestSealedEpoch0WithNoInputBoxBlockReturnsError() {
	tournamentAddr := common.HexToAddress("0xCCCC")

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      0, // misconfigured
		},
		inputSource:   s.inputBox,
		daveConsensus: s.dave,
	}

	event := &idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber:          big.NewInt(0),
		InputIndexLowerBound: big.NewInt(0),
		InputIndexUpperBound: big.NewInt(0),
		Tournament:           tournamentAddr,
		Raw:                  types.Log{BlockNumber: 100},
	}

	err := s.evmReader.processSealedEpochEvent(s.ctx, app, event)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "no InputBox block number defined")

	// No DB operations should occur
	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
	s.repository.AssertNumberOfCalls(s.T(), "GetEpoch", 0)
}

// --- Priority 6: UpdateOutputsExecution error → checkpoint not advanced ---
// When UpdateOutputsExecution fails, no UpdateEventLastCheckBlock for
// OutputExecuted should be called. The checkpoint update is embedded in
// UpdateOutputsExecution, so failure means no checkpoint advance.
func (s *EvmReaderSuite) TestUpdateOutputsExecutionErrorDoesNotAdvanceCheckpoint() {
	appContract := &MockApplicationContract{}
	// On-chain: 0 executed outputs before block 50, 1 from block 50
	appContract.On("GetNumberOfExecutedOutputs", blockRange(0, 50)).
		Return(new(big.Int).SetUint64(0), nil)
	appContract.On("GetNumberOfExecutedOutputs", blockFrom(50)).
		Return(new(big.Int).SetUint64(1), nil)
	appContract.On("RetrieveOutputExecutionEvents",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == 50
		}),
	).Return([]*iapplication.IApplicationOutputExecuted{outputExecution0}, nil)

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
		},
		applicationContract: appContract,
	}

	repo := newMockRepository()
	repo.On("GetNumberOfExecutedOutputs", mock.Anything, mock.Anything).
		Return(uint64(0), nil)
	repo.On("GetOutput", mock.Anything, mock.Anything, uint64(0)).
		Return(output0, nil)
	repo.On("UpdateOutputsExecution",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(errors.New("database connection lost"))

	s.evmReader.repository = repo

	s.evmReader.readAndUpdateOutputs(s.ctx, app, 40, 60)

	// UpdateOutputsExecution was attempted once (and failed)
	repo.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 1)
	// Checkpoint must NOT be advanced
	repo.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
}

// --- Priority 7: Block regression → no DB writes, warn logged ---
// When mostRecentBlockNumber < lastProcessedBlock (chain reorg or node
// misconfiguration), scanIConsensusInputs must not write to the database.
func (s *EvmReaderSuite) TestBlockRegressionDoesNotWriteToDb() {
	app := &Application{
		Name:                "test-app",
		IApplicationAddress: app1Addr,
		IInputBoxAddress:    inputBoxAddr,
		DataAvailability:    DataAvailability_InputBox[:],
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}

	apps := []appContracts{
		{application: app},
	}

	repo := newMockRepository()
	s.evmReader.repository = repo

	// mostRecentBlockNumber (90) < lastProcessedBlock (100) → block regression
	s.evmReader.scanIConsensusInputs(s.ctx, apps, 90)

	// No DB writes should happen
	repo.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
	repo.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	repo.AssertNumberOfCalls(s.T(), "GetEpoch", 0)
	repo.AssertNumberOfCalls(s.T(), "GetNumberOfInputs", 0)
}

// --- Block regression in sealed epoch path → no DB writes ---
// When mostRecentBlockNumber < LastEpochCheckBlock, processApplicationSealedEpochs
// must skip processing and not write to the database.
func (s *SealedEpochsSuite) TestSealedEpochBlockRegressionDoesNotWriteToDb() {
	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
			LastEpochCheckBlock: 200,
		},
		daveConsensus: s.dave,
	}

	// mostRecentBlockNumber (150) < LastEpochCheckBlock (200) → regression
	err := s.evmReader.processApplicationSealedEpochs(s.ctx, app, 150)
	s.Require().NoError(err)

	// No DB writes should occur
	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	s.repository.AssertNumberOfCalls(s.T(), "GetLastNonOpenEpoch", 0)
}

// --- Block regression in output execution path → no DB writes ---
// When mostRecentBlockNumber < LastOutputCheckBlock, checkForOutputExecution
// must skip processing and not write to the database.
func (s *EvmReaderSuite) TestOutputBlockRegressionDoesNotWriteToDb() {
	app := appContracts{
		application: &Application{
			ID:                   1,
			Name:                 "test-app",
			IApplicationAddress:  app1Addr,
			LastOutputCheckBlock: 100,
		},
		applicationContract: &MockApplicationContract{},
	}

	repo := newMockRepository()
	s.evmReader.repository = repo

	// mostRecentBlockNumber (80) < LastOutputCheckBlock (100)
	s.evmReader.checkForOutputExecution(s.ctx, []appContracts{app}, 80)

	// No DB writes should occur
	repo.AssertNumberOfCalls(s.T(), "UpdateOutputsExecution", 0)
	repo.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	repo.AssertNumberOfCalls(s.T(), "GetNumberOfExecutedOutputs", 0)
}

func (s *EvmReaderSuite) TestOutputExecutionSyncSkipsBeforeApplicationDeployment() {
	appContract := &MockApplicationContract{}
	appContract.On("GetDeploymentBlockNumber",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber != nil && opts.BlockNumber.Uint64() == 90
		}),
	).Return(new(big.Int), bind.ErrNoCode).Once()

	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
		},
		applicationContract: appContract,
	}

	repo := newMockRepository()
	s.evmReader.repository = repo

	s.evmReader.checkForOutputExecution(s.ctx, []appContracts{app}, 90)

	repo.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	repo.AssertNumberOfCalls(s.T(), "GetNumberOfPendingExecutableOutputs", 0)
	appContract.AssertExpectations(s.T())
}

// --- Input count mismatch in IConsensus path → app skipped, no checkpoint advance ---
// When the on-chain counter delta at endBlock disagrees with the number of inputs
// returned by RetrieveInputs (e.g., missing event), the app must be skipped
// (not stored) and its checkpoint must NOT advance.
func (s *EvmReaderSuite) TestIConsensusInputCountMismatchSkipsApp() {
	addr := common.HexToAddress("0x6666666666666666666666666666666666666666")

	inputSrc := &MockInputBox{}
	app := &Application{
		ID:                  1,
		Name:                "test-app",
		IApplicationAddress: addr,
		IInputBoxAddress:    inputBoxAddr,
		DataAvailability:    DataAvailability_InputBox[:],
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}

	// On-chain counter says 2 new inputs, but RetrieveInputs only returns 1
	inputSrc.On("GetNumberOfInputs", blockRange(0, 105), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	inputSrc.On("GetNumberOfInputs", blockFrom(105), mock.Anything).
		Return(new(big.Int).SetUint64(2), nil)

	// RetrieveInputs at 105 only returns 1 event (missing second input)
	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 105 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{
		makeInputEvent(addr, 0, 105),
	}, nil)

	apps := []appContracts{
		{application: app, inputSource: inputSrc},
	}

	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(uint64(0), nil)
	s.evmReader.repository = repo

	err := s.evmReader.readAndStoreInputs(s.ctx, 100, 110, apps)
	s.Require().NoError(err) // per-app failure doesn't abort

	// App was skipped: counter says 2 new, but only 1 fetched → no DB writes
	repo.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
	repo.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
}

// The IConsensus count check must validate against the end-block counter
// observed by the same transition walk that fetched the logs. Under normal
// RPC semantics, a second eth_call pinned to the same block number should
// return the same value; using the already-observed value avoids a redundant
// call and keeps validation tied to the fetch walk's chain view.
func (s *EvmReaderSuite) TestIConsensusInputCountValidationUsesObservedEndCount() {
	addr := common.HexToAddress("0x7777777777777777777777777777777777777777")

	inputSrc := &MockInputBox{}
	app := &Application{
		ID:                  1,
		Name:                "test-app",
		IApplicationAddress: addr,
		IInputBoxAddress:    inputBoxAddr,
		DataAvailability:    DataAvailability_InputBox[:],
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}

	inputSrc.On("GetNumberOfInputs", blockRange(0, 105), addr).
		Return(new(big.Int).SetUint64(0), nil)
	inputSrc.On("GetNumberOfInputs", mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockNumber.Uint64() == 105
	}), addr).Return(new(big.Int).SetUint64(1), nil).Once()

	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 105 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{
		makeInputEvent(addr, 0, 105),
	}, nil).Once()

	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, addr.String()).
		Return(uint64(0), nil).Once()
	repo.On("GetEpoch", mock.Anything, addr.String(), uint64(10)).
		Return(nil, nil).Once()
	repo.On("CreateEpochsAndInputs",
		mock.Anything,
		addr.String(),
		mock.Anything,
		uint64(105),
	).Run(func(arguments mock.Arguments) {
		epochInputMap, ok := arguments.Get(2).(map[*Epoch][]*Input)
		s.Require().True(ok)
		s.Require().Len(epochInputMap, 1)
		for _, inputs := range epochInputMap {
			s.Require().Len(inputs, 1)
			s.Require().Equal(uint64(0), inputs[0].Index)
		}
	}).Return(nil).Once()
	s.evmReader.repository = repo

	err := s.evmReader.readAndStoreInputs(s.ctx, 100, 105, []appContracts{{
		application: app,
		inputSource: inputSrc,
	}})
	s.Require().NoError(err)

	repo.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 1)
	repo.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
	repo.AssertExpectations(s.T())
	inputSrc.AssertExpectations(s.T())
}

// --- EpochLength=0 sets app corrupted ---
// When an application with EpochLength=0 reaches the epoch indexing logic,
// it must be set corrupted to prevent division-by-zero in calculateEpochIndex.
func (s *EvmReaderSuite) TestEpochLengthZeroSetsAppCorrupted() {
	addr := common.HexToAddress("0x5555555555555555555555555555555555555555")

	inputSrc := &MockInputBox{}
	// On-chain: constant 0 inputs (no transitions, fetchInputs succeeds)
	inputSrc.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)

	apps := []appContracts{{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: addr,
			IInputBoxAddress:    inputBoxAddr,
			DataAvailability:    DataAvailability_InputBox[:],
			EpochLength:         0, // will trigger corrupted
			LastInputCheckBlock: 100,
		},
		inputSource: inputSrc,
	}}

	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(uint64(0), nil)
	repo.On("UpdateApplicationStatus",
		mock.Anything, int64(1), ApplicationStatus_Corrupted, mock.Anything,
	).Return(nil)
	repo.On("UpdateEventLastCheckBlock",
		mock.Anything, mock.Anything, MonitoredEvent_InputAdded, mock.Anything,
	).Return(nil)
	s.evmReader.repository = repo

	err := s.evmReader.readAndStoreInputs(s.ctx, 100, 110, apps)
	s.Require().NoError(err)

	// App must be set inoperable
	repo.AssertNumberOfCalls(s.T(), "UpdateApplicationStatus", 1)
	// No epochs or inputs should be stored
	repo.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
}

// --- Sealed epoch CreateEpochsAndInputs failure prevents checkpoint advance ---
// When CreateEpochsAndInputs fails inside processSealedEpochEvent, the error
// must propagate through processEpochTransition and FindTransitions, causing
// processApplicationSealedEpochs to return before calling UpdateEventLastCheckBlock.
func (s *SealedEpochsSuite) TestSealedEpochDBFailurePreventsCheckpointAdvance() {
	const (
		sealBlock       uint64 = 100
		mostRecentBlock uint64 = 200
	)
	tournamentAddr := common.HexToAddress("0xEEEE")

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

	// Oracle: before sealBlock → -1, from sealBlock → 0
	s.dave.On("GetCurrentSealedEpoch",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Int64() < int64(sealBlock)
		}),
	).Return(makeSealedEpochResult(-1, 0, 0, common.Address{}), nil)
	s.dave.On("GetCurrentSealedEpoch",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() >= sealBlock
		}),
	).Return(makeSealedEpochResult(0, 0, 0, tournamentAddr), nil)

	// RetrieveSealedEpochs at transition block
	s.dave.On("RetrieveSealedEpochs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == sealBlock }),
	).Return([]*idaveconsensus.IDaveConsensusEpochSealed{
		makeSealedEpochEvent(0, 0, 0, sealBlock, tournamentAddr),
	}, nil)

	// No previous sealed epochs
	s.repository.On("GetLastNonOpenEpoch", mock.Anything, mock.Anything).
		Return(nil, nil)

	// Epoch 0 doesn't exist
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(nil, nil)

	// CreateEpochsAndInputs FAILS
	s.repository.On("CreateEpochsAndInputs",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(errors.New("database connection lost"))

	err := s.evmReader.processApplicationSealedEpochs(s.ctx, app, mostRecentBlock)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "database connection lost")

	// CreateEpochsAndInputs was attempted
	s.repository.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 1)
	// Checkpoint must NOT be advanced (error prevented reaching UpdateEventLastCheckBlock)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 0)
}

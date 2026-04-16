// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
)

// --- #19: Duplicate/out-of-order input events within a block ---
// When RetrieveInputs returns duplicate events (same index) at a single block,
// insertSorted must deduplicate them. Out-of-order events must be sorted by index.
func (s *EvmReaderSuite) TestFetchApplicationInputsDuplicateAndOutOfOrderEvents() {
	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")

	inputSrc := &MockInputBox{}
	app := appContracts{
		application: &Application{
			Name:                "test-app",
			IApplicationAddress: addr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
		},
		inputSource: inputSrc,
	}

	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, mock.Anything).
		Return(uint64(0), nil)
	s.evmReader.repository = repo

	// On-chain: 0 inputs before block 100, 3 from block 100
	inputSrc.On("GetNumberOfInputs", blockRange(0, 100), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	inputSrc.On("GetNumberOfInputs", blockFrom(100), mock.Anything).
		Return(new(big.Int).SetUint64(3), nil)

	// RetrieveInputs at block 100: 4 events including duplicate index 1,
	// delivered out-of-order: [2, 0, 1, 1]
	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 100 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{
		makeInputEvent(addr, 2, 100),
		makeInputEvent(addr, 0, 100),
		makeInputEvent(addr, 1, 100),
		makeInputEvent(addr, 1, 100), // duplicate
	}, nil)

	prevValue := new(big.Int).SetUint64(0) // repo returns 0 inputs
	inputs, _, err := s.evmReader.fetchInputs(s.ctx, app, 10, 200, prevValue, 0, math.MaxUint64)
	s.Require().NoError(err)

	// 3 unique inputs, sorted by index
	s.Require().Len(inputs, 3)
	s.Require().Equal(uint64(0), inputs[0].Index)
	s.Require().Equal(uint64(1), inputs[1].Index)
	s.Require().Equal(uint64(2), inputs[2].Index)
}

func (s *EvmReaderSuite) TestFetchInputsSameTransactionHashPreservesLogIndex() {
	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")

	inputSrc := &MockInputBox{}
	app := appContracts{
		application: &Application{
			Name:                "test-app",
			IApplicationAddress: addr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
		},
		inputSource: inputSrc,
	}

	txHash := common.HexToHash("0xfeed")
	input0 := makeInputEvent(addr, 0, 100)
	input0.Raw.TxHash = txHash
	input0.Raw.Index = 7
	input1 := makeInputEvent(addr, 1, 100)
	input1.Raw.TxHash = txHash
	input1.Raw.Index = 8

	inputSrc.On("GetNumberOfInputs", blockRange(0, 100), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	inputSrc.On("GetNumberOfInputs", blockFrom(100), mock.Anything).
		Return(new(big.Int).SetUint64(2), nil)
	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 100 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{input0, input1}, nil)

	inputs, _, err := s.evmReader.fetchInputs(
		s.ctx, app, 10, 200, new(big.Int).SetUint64(0), 0, math.MaxUint64)
	s.Require().NoError(err)
	s.Require().Len(inputs, 2)
	s.Equal(txHash, inputs[0].TransactionHash)
	s.Equal(uint64(7), inputs[0].LogIndex)
	s.Equal(txHash, inputs[1].TransactionHash)
	s.Equal(uint64(8), inputs[1].LogIndex)
}

// sameTransactionScanApp builds the app/mocks for a scan of block 100 that
// finds two InputAdded events sharing one transaction hash (log indexes 7, 8).
func (s *EvmReaderSuite) sameTransactionScanApp() (appContracts, common.Hash) {
	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")

	inputSrc := &MockInputBox{}
	app := appContracts{
		application: &Application{
			Name:                "test-app",
			IApplicationAddress: addr,
			IInputBoxAddress:    inputBoxAddr,
			DataAvailability:    DataAvailability_InputBox[:],
			Enabled:             true,
			Status:              ApplicationStatus_OK,
			IInputBoxBlock:      10,
			EpochLength:         10,
			LastInputCheckBlock: 99,
		},
		inputSource: inputSrc,
	}

	txHash := common.HexToHash("0xfeed")
	event0 := makeInputEvent(addr, 0, 100)
	event0.Raw.TxHash = txHash
	event0.Raw.Index = 7
	event1 := makeInputEvent(addr, 1, 100)
	event1.Raw.TxHash = txHash
	event1.Raw.Index = 8

	inputSrc.On("GetNumberOfInputs",
		mock.MatchedBy(func(opts *bind.CallOpts) bool {
			return opts.BlockNumber.Uint64() == 100
		}),
		addr,
	).Return(new(big.Int).SetUint64(2), nil)
	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == 100 && opts.End != nil && *opts.End == 100
		}),
		[]common.Address{addr}, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{event0, event1}, nil).Once()

	return app, txHash
}

// The same-transaction pair must survive the full scan path — fetch, sort,
// epoch indexing — and reach CreateEpochsAndInputs with the shared
// transaction hash and distinct log indexes.
func (s *EvmReaderSuite) TestSameTransactionInputsReachRepository() {
	app, txHash := s.sameTransactionScanApp()
	addr := app.application.IApplicationAddress

	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, addr.String()).Return(uint64(0), nil)
	repo.On("GetEpoch", mock.Anything, addr.String(), uint64(9)).Return(nil, nil)
	repo.On("CreateEpochsAndInputs",
		mock.Anything, addr.String(), mock.Anything, uint64(100),
	).Run(func(arguments mock.Arguments) {
		epochInputMap, ok := arguments.Get(2).(map[*Epoch][]*Input)
		s.Require().True(ok)
		s.Require().Len(epochInputMap, 1)
		for _, inputs := range epochInputMap {
			s.Require().Len(inputs, 2)
			s.Require().Equal(uint64(0), inputs[0].Index)
			s.Require().Equal(txHash, inputs[0].TransactionHash)
			s.Require().Equal(uint64(7), inputs[0].LogIndex)
			s.Require().Equal(uint64(1), inputs[1].Index)
			s.Require().Equal(txHash, inputs[1].TransactionHash)
			s.Require().Equal(uint64(8), inputs[1].LogIndex)
		}
	}).Return(nil).Once()
	s.evmReader.repository = repo

	s.evmReader.scanIConsensusInputs(s.ctx, []appContracts{app}, 100)

	repo.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 1)
}

// A CreateEpochsAndInputs failure tagged ErrInputLogIdentityConflict means the
// stored inputs diverged from rescanned chain data; the scan must escalate the
// application to CORRUPTED instead of retrying the same insert forever.
func (s *EvmReaderSuite) TestInputLogIdentityConflictMarksApplicationCorrupted() {
	app, _ := s.sameTransactionScanApp()
	addr := app.application.IApplicationAddress

	repo := newMockRepository()
	repo.On("GetNumberOfInputs", mock.Anything, addr.String()).Return(uint64(0), nil)
	repo.On("GetEpoch", mock.Anything, addr.String(), uint64(9)).Return(nil, nil)
	repo.On("CreateEpochsAndInputs",
		mock.Anything, addr.String(), mock.Anything, uint64(100),
	).Return(fmt.Errorf("insert failed: %w", repository.ErrInputLogIdentityConflict)).Once()
	repo.On("UpdateApplicationStatus",
		mock.Anything, app.application.ID, ApplicationStatus_Corrupted, mock.Anything,
	).Return(nil).Once()
	s.evmReader.repository = repo

	s.evmReader.scanIConsensusInputs(s.ctx, []appContracts{app}, 100)

	repo.AssertNumberOfCalls(s.T(), "UpdateApplicationStatus", 1)
	s.Equal(ApplicationStatus_Corrupted, app.application.Status)
}

// --- Bounds filtering: inputs outside [lowerBound, upperBound) are excluded ---
// When RetrieveInputs returns events with indices outside the requested bounds,
// fetchInputs must exclude them. This exercises the bounds filtering added in the
// unified fetchInputs (C2 consolidation).
func (s *EvmReaderSuite) TestFetchInputsBoundsFiltering() {
	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")

	inputSrc := &MockInputBox{}
	app := appContracts{
		application: &Application{
			Name:                "test-app",
			IApplicationAddress: addr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
		},
		inputSource: inputSrc,
	}

	// On-chain: 0 inputs before block 100, 5 from block 100
	inputSrc.On("GetNumberOfInputs", blockRange(0, 100), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	inputSrc.On("GetNumberOfInputs", blockFrom(100), mock.Anything).
		Return(new(big.Int).SetUint64(5), nil)

	// RetrieveInputs returns indices 0..4, but we only want [2, 4)
	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 100 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{
		makeInputEvent(addr, 0, 100), // below lowerBound → excluded
		makeInputEvent(addr, 1, 100), // below lowerBound → excluded
		makeInputEvent(addr, 2, 100), // in bounds → included
		makeInputEvent(addr, 3, 100), // in bounds → included
		makeInputEvent(addr, 4, 100), // >= upperBound → excluded
	}, nil)

	prevValue := new(big.Int).SetUint64(0)
	inputs, _, err := s.evmReader.fetchInputs(s.ctx, app, 10, 200, prevValue, 2, 4)
	s.Require().NoError(err)

	// Only indices 2 and 3 should be included
	s.Require().Len(inputs, 2)
	s.Require().Equal(uint64(2), inputs[0].Index)
	s.Require().Equal(uint64(3), inputs[1].Index)
}

// --- Bounds filtering: upperBound == lowerBound yields zero inputs ---
// When lowerBound == upperBound, the half-open range [lb, ub) is empty,
// so no inputs should be returned even if the chain has matching events.
func (s *EvmReaderSuite) TestFetchInputsEmptyBoundsRange() {
	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")

	inputSrc := &MockInputBox{}
	app := appContracts{
		application: &Application{
			Name:                "test-app",
			IApplicationAddress: addr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
		},
		inputSource: inputSrc,
	}

	// On-chain: 0 inputs before block 100, 2 from block 100
	inputSrc.On("GetNumberOfInputs", blockRange(0, 100), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	inputSrc.On("GetNumberOfInputs", blockFrom(100), mock.Anything).
		Return(new(big.Int).SetUint64(2), nil)

	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 100 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{
		makeInputEvent(addr, 0, 100),
		makeInputEvent(addr, 1, 100),
	}, nil)

	// lowerBound == upperBound == 5 → empty range
	prevValue := new(big.Int).SetUint64(0)
	inputs, _, err := s.evmReader.fetchInputs(s.ctx, app, 10, 200, prevValue, 5, 5)
	s.Require().NoError(err)
	s.Require().Empty(inputs)
}

// --- Bounds filtering: boundary-exact inclusion/exclusion ---
// Verifies the half-open [lowerBound, upperBound) semantics precisely:
// lowerBound is inclusive, upperBound is exclusive.
func (s *EvmReaderSuite) TestFetchInputsBoundaryExactness() {
	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")

	inputSrc := &MockInputBox{}
	app := appContracts{
		application: &Application{
			Name:                "test-app",
			IApplicationAddress: addr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
		},
		inputSource: inputSrc,
	}

	// On-chain: 0 inputs before block 100, 3 from block 100
	inputSrc.On("GetNumberOfInputs", blockRange(0, 100), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	inputSrc.On("GetNumberOfInputs", blockFrom(100), mock.Anything).
		Return(new(big.Int).SetUint64(3), nil)

	inputSrc.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 100 }),
		mock.Anything, mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{
		makeInputEvent(addr, 0, 100),
		makeInputEvent(addr, 1, 100),
		makeInputEvent(addr, 2, 100),
	}, nil)

	// Range [1, 2): only index 1 should be included
	prevValue := new(big.Int).SetUint64(0)
	inputs, _, err := s.evmReader.fetchInputs(s.ctx, app, 10, 200, prevValue, 1, 2)
	s.Require().NoError(err)
	s.Require().Len(inputs, 1)
	s.Require().Equal(uint64(1), inputs[0].Index)
}

// --- #18: Adversarial EpochSealed data (LowerBound > UpperBound) ---
// When a sealed event has InputIndexLowerBound > InputIndexUpperBound,
// the code skips input fetching and stores the epoch without crashing.
func (s *SealedEpochsSuite) TestSealedEpochLowerBoundGreaterThanUpperBound() {
	tournamentAddr := common.HexToAddress("0xDDDD")

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

	// Adversarial sealed event: LowerBound=5, UpperBound=3
	event := &idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber:          big.NewInt(1),
		InputIndexLowerBound: big.NewInt(5),
		InputIndexUpperBound: big.NewInt(3), // < LowerBound — adversarial
		Tournament:           tournamentAddr,
		Raw:                  types.Log{BlockNumber: 200},
	}

	// Previous epoch (index 0) with UpperBound=5 matching the adversarial LowerBound
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(0)).
		Return(&Epoch{
			Index: 0, FirstBlock: 10, LastBlock: 100,
			InputIndexLowerBound: 0, InputIndexUpperBound: 5,
		}, nil)
	s.repository.On("UpdateEpochClaimTransactionHash",
		mock.Anything, mock.Anything, mock.Anything,
	).Return(nil)

	// Epoch 1 doesn't exist yet
	s.repository.On("GetEpoch", mock.Anything, mock.Anything, uint64(1)).
		Return(nil, nil)

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

	err := s.evmReader.processSealedEpochEvent(s.ctx, app, event)
	s.Require().NoError(err)

	// Epoch stored with inverted bounds, no inputs fetched
	s.Require().NotNil(storedEpoch)
	s.Require().Equal(uint64(1), storedEpoch.Index)
	s.Require().Equal(uint64(5), storedEpoch.InputIndexLowerBound)
	s.Require().Equal(uint64(3), storedEpoch.InputIndexUpperBound)
	s.Require().Equal(EpochStatus_Closed, storedEpoch.Status)
	s.Require().Empty(storedInputs)

	// No input fetching occurred
	s.inputBox.AssertNotCalled(s.T(), "GetNumberOfInputs")
	s.inputBox.AssertNotCalled(s.T(), "RetrieveInputs")
}

// --- #10: RetrieveInputs failure at a specific block ---
// When RetrieveInputs fails during fetchInputs, the error must
// propagate through FindTransitions back to the caller.
func (s *SealedEpochsSuite) TestFetchSealedEpochInputsRetrieveFailure() {
	app := appContracts{
		application: &Application{
			ID:                  1,
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IInputBoxAddress:    inputBoxAddr,
			IInputBoxBlock:      10,
		},
		inputSource: s.inputBox,
	}

	epoch := &Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            200,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 2,
	}

	// On-chain: 0 inputs before block 100, 2 from block 100
	s.inputBox.On("GetNumberOfInputs", blockRange(0, 100), mock.Anything).
		Return(new(big.Int).SetUint64(0), nil)
	s.inputBox.On("GetNumberOfInputs", blockFrom(100), mock.Anything).
		Return(new(big.Int).SetUint64(2), nil)

	// RetrieveInputs fails at the transition block
	s.inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool { return opts.Start == 100 }),
		mock.Anything, mock.Anything,
	).Return(([]iinputbox.IInputBoxInputAdded)(nil), errors.New("RPC timeout"))

	prevValue := new(big.Int).SetUint64(epoch.InputIndexLowerBound)
	_, _, err := s.evmReader.fetchInputs(s.ctx, app,
		epoch.FirstBlock, epoch.LastBlock,
		prevValue,
		epoch.InputIndexLowerBound, epoch.InputIndexUpperBound)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "RPC timeout")
	s.Require().ErrorContains(err, "failed to walk input transitions")
}

// --- Adapter cache invalidation on config change ---
// When an application's consensus address changes between block headers,
// the adapter cache must be invalidated and adapters recreated.
func (s *EvmReaderSuite) TestAdapterCacheInvalidationOnConfigChange() {
	// Fire 3 headers (block numbers below 999 so output check skips)
	s.client.EnqueueNewHead(100).Once()
	s.client.EnqueueNewHead(101).Once()
	called := newCallNotification(s.client.EnqueueNewHead(102))

	s.evmReader.inputReaderEnabled = false
	s.evmReader.defaultBlock = DefaultBlock_Latest

	addr := common.HexToAddress("0x4444444444444444444444444444444444444444")
	consensusAddr1 := common.HexToAddress("0xAAA1")
	consensusAddr2 := common.HexToAddress("0xAAA2")

	repo := newMockRepository()
	// Header 1: app with consensus=addr1
	repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{{
			ID: 1, Name: "app",
			IApplicationAddress:     addr,
			IConsensusAddress:       consensusAddr1,
			IInputBoxAddress:        inputBoxAddr,
			LastOutputCheckBlock:    999, // > header block → skip output check
			LastForecloseCheckBlock: 999,
		}}, uint64(1), nil).Once()
	// Header 2: consensus address changed → cache invalidation
	repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{{
			ID: 1, Name: "app",
			IApplicationAddress:     addr,
			IConsensusAddress:       consensusAddr2,
			IInputBoxAddress:        inputBoxAddr,
			LastOutputCheckBlock:    999,
			LastForecloseCheckBlock: 999,
		}}, uint64(1), nil).Once()
	// Header 3: same config as header 2 → cache hit
	repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{{
			ID: 1, Name: "app",
			IApplicationAddress:     addr,
			IConsensusAddress:       consensusAddr2,
			IInputBoxAddress:        inputBoxAddr,
			LastOutputCheckBlock:    999,
			LastForecloseCheckBlock: 999,
		}}, uint64(1), nil).Once()
	// Catch-all for sentinel header
	repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{}, uint64(0), nil)
	repo.On("UpdateApplicationLastForecloseCheckBlock",
		mock.Anything, int64(1), mock.Anything,
	).Return(nil).Maybe()
	s.evmReader.repository = repo

	factory := newMockAdapterFactory()
	factory.On("CreateAdapters", mock.Anything).
		Return(newMockApplicationContract(), newMockInputBox(), nil, nil)
	s.evmReader.adapterFactory = factory
	s.evmReader.resolver = newApplicationAdapterResolver(s.evmReader.Logger, factory)

	done := make(chan struct{})
	go func() {
		err := s.supervisor.Serve()
		s.Require().NoError(err)
		close(done)
	}()

	s.Require().True(waitNotification(called), "evmreader did not read headers")
	s.supervisor.Stop(false)
	s.Require().True(waitNotification(done), "evmreader did not stop after supervisor was stopped")

	// CreateAdapters called twice:
	// Header 1: cache miss → create
	// Header 2: consensus changed → invalidate + recreate
	// Header 3: cache hit → skip
	factory.AssertNumberOfCalls(s.T(), "CreateAdapters", 2)
}

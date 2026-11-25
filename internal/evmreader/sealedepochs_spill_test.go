// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

// Since Dave alpha.7 a seal is capped at 2^24 pending inputs and the surplus
// is left for the next epoch. An input of epoch B can therefore be published
// in a block before epoch A's seal block, which is the start of the sealed
// epoch's block search. These tests describe the desired behavior and fail on
// the pre-fix code.

import (
	"context"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestSealedPipelineAtProductionCap reproduces the ingestion stall. The input
// of the next epoch is already in the InputBox before the previous seal, so
// the block window [previous seal, current seal] contains no InputAdded log
// for it. The reader must still store the epoch and its input.
func TestSealedPipelineAtProductionCap(t *testing.T) {
	const (
		inputBoxBlock     uint64 = 10
		spillInputBlock   uint64 = 90
		previousSealBlock uint64 = 100
		sealBlock         uint64 = 120
		productionCap     uint64 = 1 << 24 // 16_777_216
	)
	tournament := common.HexToAddress("0xAAAA")

	repo := newMockRepository()
	inputBox := newMockInputBox()
	reader := &Service{repository: repo}
	reader.Logger = testLogger(t)

	app := newDaveAppContracts(inputBox, nil)
	app.application.IInputBoxBlock = inputBoxBlock
	app.application.IInputBoxAddress = inputBoxAddr
	app.application.LastInputCheckBlock = previousSealBlock

	// Epoch A was sealed at block 100 and owns the first 2^24 inputs. Its
	// seal is the next epoch's search floor.
	previous := &Epoch{
		Index:                0,
		FirstBlock:           inputBoxBlock,
		LastBlock:            previousSealBlock,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: productionCap,
		Status:               EpochStatus_Closed,
	}
	repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(0)).
		Return(previous, nil).Once()
	repo.On("UpdateEpochClaimTransactionHash", t.Context(), app1Addr.Hex(), previous).
		Return(nil).Once()
	repo.On("GetEpoch", t.Context(), app1Addr.Hex(), uint64(1)).
		Return(nil, nil).Once()

	// The spill input is in the InputBox before block 100: the counter is
	// already 2^24+1 when the search starts, so the only transition is at the
	// start block, which has no matching log.
	inputBox.On("GetNumberOfInputs", blockRange(0, previousSealBlock+1), app1Addr).
		Return(new(big.Int).SetUint64(productionCap+1), nil)
	inputBox.On("GetNumberOfInputs", blockRange(previousSealBlock+1, sealBlock+1), app1Addr).
		Return(new(big.Int).SetUint64(productionCap+1), nil)
	inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == previousSealBlock && opts.End != nil && *opts.End == previousSealBlock
		}),
		[]common.Address{app1Addr},
		mock.Anything,
	).Return([]iinputbox.IInputBoxInputAdded{}, nil).Once()

	var storedInputs []*Input
	repo.On("CreateEpochsAndInputs", t.Context(), app1Addr.Hex(),
		mock.MatchedBy(func(epochs map[*Epoch][]*Input) bool {
			for epoch, inputs := range epochs {
				return len(epochs) == 1 && epoch.Index == 1 && len(inputs) == 1
			}
			return false
		}),
		sealBlock-1,
	).Run(func(args mock.Arguments) {
		for _, inputs := range args.Get(2).(map[*Epoch][]*Input) {
			storedInputs = inputs
		}
	}).Return(nil).Maybe()

	event := makeSealedEpochEvent(1, productionCap, productionCap+1, sealBlock, tournament)
	err := reader.processSealedEpochEvent(t.Context(), app, event)
	require.NoError(t, err, "the epoch must be ingested even though its input predates the search window")
	require.Len(t, storedInputs, 1)
	require.Equal(t, productionCap, storedInputs[0].Index)
	require.Equal(t, spillInputBlock, storedInputs[0].BlockNumber)

	repo.AssertExpectations(t)
	inputBox.AssertExpectations(t)
}

// TestSealedEpochDoesNotAttributeObservedSurplus reproduces the over-execution.
// The OPEN scan stores three inputs; the seal covers only the first two. The
// third must not stay attached to the sealed epoch, otherwise the advancer
// executes it under that epoch and the epoch cannot finalize.
func TestSealedEpochDoesNotAttributeObservedSurplus(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	ctx := context.Background()
	release, err := db.LockTestPostgres(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(release)
	require.NoError(t, db.SetupTestPostgres(endpoint))

	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })

	const (
		inputBoxBlock uint64 = 10
		inputBlock    uint64 = 90
		sealBlock     uint64 = 100
	)
	tournament := common.HexToAddress("0xAAAA")

	app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Build()
	app.IInputBoxBlock = inputBoxBlock
	id, err := repo.CreateApplication(ctx, app, false)
	require.NoError(t, err)
	app.ID = id
	address := app.IApplicationAddress.Hex()

	// The OPEN scan already stored indices 0, 1 and 2 before the seal.
	openEpoch := &Epoch{
		Index:                0,
		FirstBlock:           inputBoxBlock,
		LastBlock:            inputBlock,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 3,
		Status:               EpochStatus_Open,
	}
	observed := make([]*Input, 0, 3)
	events := make([]iinputbox.IInputBoxInputAdded, 0, 3)
	for index := uint64(0); index < 3; index++ {
		event := makeInputEvent(app.IApplicationAddress, index, inputBlock)
		events = append(events, event)
		observed = append(observed, &Input{
			Index:           index,
			Status:          InputCompletionStatus_None,
			RawData:         event.Input,
			BlockNumber:     inputBlock,
			TransactionHash: event.Raw.TxHash,
			LogIndex:        uint64(index),
		})
	}
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, address,
		map[*Epoch][]*Input{openEpoch: observed}, inputBlock))

	inputBox := newMockInputBox()
	inputBox.On("GetNumberOfInputs", blockRange(0, inputBlock), app.IApplicationAddress).
		Return(new(big.Int), nil)
	inputBox.On("GetNumberOfInputs", blockFrom(inputBlock), app.IApplicationAddress).
		Return(new(big.Int).SetUint64(3), nil)
	inputBox.On("RetrieveInputs",
		mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts.Start == inputBlock && opts.End != nil && *opts.End == inputBlock
		}),
		[]common.Address{app.IApplicationAddress},
		mock.Anything,
	).Return(events, nil).Once()

	reader := &Service{repository: repo}
	reader.Logger = testLogger(t)
	appContracts := appContracts{application: app, inputSource: inputBox}
	event := makeSealedEpochEvent(0, 0, 2, sealBlock, tournament)
	require.NoError(t, reader.processSealedEpochEvent(ctx, appContracts, event))

	sealed, err := repo.GetEpoch(ctx, address, 0)
	require.NoError(t, err)
	require.Equal(t, EpochStatus_Closed, sealed.Status)
	require.Equal(t, uint64(2), sealed.InputIndexUpperBound)

	// The advancer's query for this epoch must see only the attributed rows.
	epochIndex := uint64(0)
	status := InputCompletionStatus_None
	inputs, total, err := repo.ListInputs(ctx, address, repository.InputFilter{
		EpochIndex: &epochIndex,
		Status:     &status,
	}, repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Equal(t, uint64(2), total, "the sealed epoch owns two inputs, not the observed surplus")
	require.Len(t, inputs, 2)
	for i, input := range inputs {
		require.Equal(t, uint64(i), input.Index)
	}

	// The surplus remains observed and visible to index/count lookups.
	count, err := repo.GetNumberOfInputs(ctx, address)
	require.NoError(t, err)
	require.Equal(t, uint64(3), count, "the observed surplus must remain visible to the input counter")

	inputBox.AssertExpectations(t)
}

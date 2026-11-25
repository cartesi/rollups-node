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
	"github.com/cartesi/rollups-node/test/tooling/db"
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
	reader := &Service{repository: repo}
	reader.Logger = testLogger(t)

	app := newDaveAppContracts(nil, nil)
	app.application.IInputBoxBlock = inputBoxBlock
	app.application.IInputBoxAddress = inputBoxAddr
	app.application.LastInputCheckBlock = previousSealBlock

	// Epoch A was sealed at block 100 and owns the first 2^24 inputs.
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

	// Observation already recorded the spill input at block 90, before the
	// previous seal block. Attribution must not search by block window.
	spill := &Input{
		Index:           productionCap,
		BlockNumber:     spillInputBlock,
		RawData:         []byte{0xde, 0xad},
		Status:          InputCompletionStatus_None,
		TransactionHash: common.BigToHash(big.NewInt(1)),
		LogIndex:        0,
	}
	repo.On("ListObservedInputs", t.Context(), app1Addr.Hex(), productionCap, productionCap+1).
		Return([]*Input{spill}, nil).Once()

	var storedInputs []*Input
	repo.On("AttributeSealedEpoch", t.Context(), app1Addr.Hex(),
		mock.MatchedBy(func(epoch *Epoch) bool {
			return epoch.Index == 1 &&
				epoch.InputIndexLowerBound == productionCap &&
				epoch.InputIndexUpperBound == productionCap+1 &&
				epoch.LastBlock == sealBlock
		}),
		mock.Anything,
	).Run(func(args mock.Arguments) {
		storedInputs = args.Get(3).([]*Input)
	}).Return(nil).Once()

	event := makeSealedEpochEvent(1, productionCap, productionCap+1, sealBlock, tournament)
	err := reader.processSealedEpochEvent(t.Context(), app, event)
	require.NoError(t, err, "the epoch must be ingested even though its input predates the search window")
	require.Len(t, storedInputs, 1)
	require.Equal(t, productionCap, storedInputs[0].Index)
	require.Equal(t, spillInputBlock, storedInputs[0].BlockNumber)

	repo.AssertExpectations(t)
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
	for index := uint64(0); index < 3; index++ {
		event := makeInputEvent(app.IApplicationAddress, index, inputBlock)
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

	reader := &Service{repository: repo}
	reader.Logger = testLogger(t)
	appContracts := appContracts{application: app}
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

	// The surplus remains observed for a later epoch.
	observedCount := countObservedInputs(ctx, t, repo, address)
	require.Equal(t, uint64(1), observedCount, "the observed surplus must remain unassigned")

}

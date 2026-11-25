// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres_test

import (
	"context"
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func setupDaveInputRepository(t *testing.T) (context.Context, repository.Repository, *model.Application) {
	t.Helper()
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))

	ctx := context.Background()
	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })

	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Build()
	id, err := repo.CreateApplication(ctx, app, false)
	require.NoError(t, err)
	app.ID = id
	return ctx, repo, app
}

// countObservedInputs exercises the Postgres-only counter without putting a
// test-only method on the shared repository interface.
func countObservedInputs(ctx context.Context, t *testing.T,
	repo repository.Repository, address string) uint64 {
	t.Helper()
	counter, ok := repo.(interface {
		CountObservedInputs(context.Context, string) (uint64, error)
	})
	require.True(t, ok, "repository must be a Postgres repository")
	count, err := counter.CountObservedInputs(ctx, address)
	require.NoError(t, err)
	return count
}

func daveInput(index, block uint64) *model.Input {
	rawData := make([]byte, 8)
	binary.BigEndian.PutUint64(rawData, index)
	return &model.Input{
		Index:           index,
		BlockNumber:     block,
		RawData:         rawData,
		Status:          model.InputCompletionStatus_None,
		TransactionHash: common.BigToHash(new(big.Int).SetUint64(block*1000 + index)),
		LogIndex:        index,
	}
}

func TestDaveInputObservationSurvivesAttributionAndProjection(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	const (
		observationBlock uint64 = 90
		sealBlock        uint64 = 100
		head             uint64 = 120
	)
	observed := []*model.Input{
		daveInput(0, observationBlock),
		daveInput(1, observationBlock),
		daveInput(2, observationBlock),
	}
	require.NoError(t, repo.ObserveDaveInputs(ctx, address,
		observationBlock, 3, observed))

	frontier, err := repo.GetInputObservation(ctx, address)
	require.NoError(t, err)
	require.Equal(t, &model.InputObservation{
		ThroughBlock:   observationBlock,
		NextInputIndex: 3,
	}, frontier)

	count := countObservedInputs(ctx, t, repo, address)
	require.Equal(t, uint64(3), count)

	firstTwo, err := repo.ListObservedInputs(ctx, address, 0, 2)
	require.NoError(t, err)
	require.Len(t, firstTwo, 2)
	require.Equal(t, uint64(0), firstTwo[0].Index)
	require.Equal(t, uint64(1), firstTwo[1].Index)
	require.Equal(t, model.InputCompletionStatus_None, firstTwo[0].Status)

	// The seal attributes [0, 2) and leaves the observed surplus for the next
	// epoch. It must not publish the input cursor.
	sealed := &model.Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            sealBlock,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 2,
		Status:               model.EpochStatus_Closed,
	}
	require.NoError(t, repo.AttributeSealedEpoch(ctx, address, sealed, firstTwo))

	epochIndex := uint64(0)
	status := model.InputCompletionStatus_None
	attributed, total, err := repo.ListInputs(ctx, address, repository.InputFilter{
		EpochIndex: &epochIndex,
		Status:     &status,
	}, repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Equal(t, uint64(2), total)
	require.Len(t, attributed, 2)

	count = countObservedInputs(ctx, t, repo, address)
	require.Equal(t, uint64(1), count, "the surplus must stay observed")

	storedApp, err := repo.GetApplication(ctx, address)
	require.NoError(t, err)
	require.Zero(t, storedApp.LastInputCheckBlock,
		"attribution must not publish the input cursor")

	// Replaying the same seal is idempotent.
	require.NoError(t, repo.AttributeSealedEpoch(ctx, address, sealed, firstTwo))
	count = countObservedInputs(ctx, t, repo, address)
	require.Equal(t, uint64(1), count)

	// The OPEN epoch projects only [2, 3) and publishes the cursor.
	surplus, err := repo.ListObservedInputs(ctx, address, 2, 3)
	require.NoError(t, err)
	require.Len(t, surplus, 1)
	open := &model.Epoch{
		Index:                1,
		FirstBlock:           sealBlock,
		LastBlock:            head,
		InputIndexLowerBound: 2,
		InputIndexUpperBound: 3,
		Status:               model.EpochStatus_Open,
	}
	require.NoError(t, repo.ProjectDaveOpenInputs(ctx, address, open, surplus, head))

	count = countObservedInputs(ctx, t, repo, address)
	require.Zero(t, count, "projection consumes the observed surplus")

	storedApp, err = repo.GetApplication(ctx, address)
	require.NoError(t, err)
	require.Equal(t, head, storedApp.LastInputCheckBlock)

	frontier, err = repo.GetInputObservation(ctx, address)
	require.NoError(t, err)
	require.Equal(t, &model.InputObservation{
		ThroughBlock:   observationBlock,
		NextInputIndex: 3,
	}, frontier, "attribution and projection must not rewind observation")

	storedOpen, err := repo.GetEpoch(ctx, address, 1)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_Open, storedOpen.Status)
	require.Equal(t, uint64(3), storedOpen.InputIndexUpperBound)
}

func TestAttributeSealedEpochDetachesUnsealedSurplus(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	// A pre-seal OPEN projection stored all three pending inputs. The seal only
	// covers the first two.
	open := &model.Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            90,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 3,
		Status:               model.EpochStatus_Open,
	}
	inputs := []*model.Input{
		daveInput(0, 90),
		daveInput(1, 90),
		daveInput(2, 90),
	}
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, address,
		map[*model.Epoch][]*model.Input{open: inputs}, 90))

	sealed := *open
	sealed.LastBlock = 100
	sealed.InputIndexUpperBound = 2
	sealed.Status = model.EpochStatus_Closed
	require.NoError(t, repo.AttributeSealedEpoch(ctx, address, &sealed, inputs[:2]))

	attributed, total, err := repo.ListInputs(ctx, address, repository.InputFilter{
		EpochIndex: &open.Index,
	}, repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Equal(t, uint64(2), total)
	require.Len(t, attributed, 2)

	surplus, err := repo.ListObservedInputs(ctx, address, 2, 3)
	require.NoError(t, err)
	require.Len(t, surplus, 1)
	require.Equal(t, uint64(2), surplus[0].Index)
	require.Equal(t, model.InputCompletionStatus_None, surplus[0].Status)
}

func TestAttributeSealedEpochRejectsExecutedSurplus(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	open := &model.Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            90,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 3,
		Status:               model.EpochStatus_Open,
	}
	inputs := []*model.Input{
		daveInput(0, 90),
		daveInput(1, 90),
		daveInput(2, 90),
	}
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, address,
		map[*model.Epoch][]*model.Input{open: inputs}, 90))

	// Mark the surplus input as executed. CreateEpochsAndInputs only stores
	// pending inputs, so complete it directly.
	endpoint, err := db.GetTestDatabaseEndpoint()
	require.NoError(t, err)
	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(context.Background())) })
	machineHash := common.HexToHash("0x01")
	_, err = conn.Exec(ctx, `
		UPDATE input
		SET status = 'ACCEPTED', machine_hash = $1, tx_buffer_data_block = $2
		WHERE epoch_application_id = $3 AND epoch_index = $4 AND index = $5`,
		machineHash.Bytes(), machineHash.Bytes(), app.ID, open.Index, 2)
	require.NoError(t, err)

	sealed := *open
	sealed.LastBlock = 100
	sealed.InputIndexUpperBound = 2
	sealed.Status = model.EpochStatus_Closed
	err = repo.AttributeSealedEpoch(ctx, address, &sealed, inputs[:2])
	require.ErrorIs(t, err, repository.ErrEpochInputRangeConflict)

	// The failed attribution must not move the executed input.
	stored, _, err := repo.ListInputs(ctx, address, repository.InputFilter{
		EpochIndex: &open.Index,
	}, repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Len(t, stored, 3)
	require.Equal(t, model.InputCompletionStatus_Accepted, stored[2].Status)
}

func TestHistoricalInputLookupsIncludeObservedSurplus(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	observed := []*model.Input{
		daveInput(0, 90),
		daveInput(1, 90),
		daveInput(2, 90),
	}
	require.NoError(t, repo.ObserveDaveInputs(ctx, address, 90, 3, observed))

	// A by-index lookup must find an input that has no epoch yet.
	got, err := repo.GetInput(ctx, address, 2)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Unattributed)
	require.Equal(t, uint64(2), got.Index)
	require.Equal(t, model.InputCompletionStatus_None, got.Status)
	require.True(t, got.CreatedAt.IsZero())

	// ListInputs unions attributed and observed rows without exposing a fake
	// epoch on the observed one.
	all, total, err := repo.ListInputs(ctx, address, repository.InputFilter{},
		repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Equal(t, uint64(3), total)
	require.Len(t, all, 3)
	require.True(t, all[0].Unattributed)
	require.True(t, all[2].Unattributed)

	descending, total, err := repo.ListInputs(ctx, address, repository.InputFilter{},
		repository.Pagination{Limit: 1}, true)
	require.NoError(t, err)
	require.Equal(t, uint64(3), total)
	require.Len(t, descending, 1)
	require.Equal(t, uint64(2), descending[0].Index)
	require.True(t, descending[0].Unattributed)

	// An epoch filter cannot match observed rows.
	_, total, err = repo.ListInputs(ctx, address, repository.InputFilter{
		EpochIndex: new(uint64),
	}, repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Zero(t, total)

	// Status filters treat an observed row as NONE.
	none := model.InputCompletionStatus_None
	_, total, err = repo.ListInputs(ctx, address, repository.InputFilter{Status: &none},
		repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Equal(t, uint64(3), total)

	accepted := model.InputCompletionStatus_Accepted
	_, total, err = repo.ListInputs(ctx, address, repository.InputFilter{Status: &accepted},
		repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Zero(t, total)

	// Once the prefix is attributed, the surplus remains queryable next to it.
	sealed := &model.Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            100,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 2,
		Status:               model.EpochStatus_Closed,
	}
	require.NoError(t, repo.AttributeSealedEpoch(ctx, address, sealed, observed[:2]))

	all, total, err = repo.ListInputs(ctx, address, repository.InputFilter{},
		repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Equal(t, uint64(3), total)
	require.Len(t, all, 3)
	require.False(t, all[0].Unattributed)
	require.False(t, all[1].Unattributed)
	require.True(t, all[2].Unattributed, "the observed surplus stays queryable")

	got, err = repo.GetInput(ctx, address, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.False(t, got.Unattributed)
	require.Equal(t, uint64(0), got.EpochIndex)
}

func TestProjectDaveOpenInputsPreservesCompletedInput(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	open := &model.Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            90,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 1,
		Status:               model.EpochStatus_Open,
	}
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, address,
		map[*model.Epoch][]*model.Input{open: {daveInput(0, 90)}}, 90))

	// A completed input has non-NULL hashes. Re-projecting the OPEN epoch must
	// not try to reinsert it as a fresh NONE row.
	endpoint, err := db.GetTestDatabaseEndpoint()
	require.NoError(t, err)
	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(context.Background())) })
	machineHash := common.HexToHash("0x01")
	_, err = conn.Exec(ctx, `
		UPDATE input
		SET status = 'INVALID_OUTPUTS_ROOT', machine_hash = $1, tx_buffer_data_block = $2
		WHERE epoch_application_id = $3 AND epoch_index = $4 AND index = $5`,
		machineHash.Bytes(), machineHash.Bytes(), app.ID, open.Index, 0)
	require.NoError(t, err)

	completed, err := repo.GetInput(ctx, address, 0)
	require.NoError(t, err)
	require.Equal(t, model.InputCompletionStatus_InvalidOutputsRoot, completed.Status)

	require.NoError(t, repo.ObserveDaveInputs(ctx, address, 95, 2,
		[]*model.Input{daveInput(1, 95)}))
	newInput, err := repo.GetInput(ctx, address, 1)
	require.NoError(t, err)
	require.True(t, newInput.Unattributed)

	projection := *open
	projection.LastBlock = 100
	projection.InputIndexUpperBound = 2
	// Only the newly grown prefix [1, 2) is sent. The completed input from the
	// previous projection must be preserved without being re-proposed.
	require.NoError(t, repo.ProjectDaveOpenInputs(ctx, address, &projection,
		[]*model.Input{newInput}, 100))

	stored, err := repo.GetInput(ctx, address, 0)
	require.NoError(t, err)
	require.Equal(t, model.InputCompletionStatus_InvalidOutputsRoot, stored.Status)
	require.Equal(t, &machineHash, stored.MachineHash)

	pending, err := repo.GetInput(ctx, address, 1)
	require.NoError(t, err)
	require.False(t, pending.Unattributed)
	require.Equal(t, model.InputCompletionStatus_None, pending.Status)
}

func TestDaveInputObservationIsMonotonic(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	require.NoError(t, repo.ObserveDaveInputs(ctx, address, 100, 2,
		[]*model.Input{daveInput(0, 90), daveInput(1, 95)}))

	for _, test := range []struct {
		name           string
		throughBlock   uint64
		nextInputIndex uint64
	}{
		{name: "block rewinds", throughBlock: 99, nextInputIndex: 2},
		{name: "index rewinds on a later block", throughBlock: 101, nextInputIndex: 1},
		{name: "same block changes index", throughBlock: 100, nextInputIndex: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := repo.ObserveDaveInputs(ctx, address, test.throughBlock,
				test.nextInputIndex, nil)
			require.ErrorIs(t, err, repository.ErrInputObservationConflict)
		})
	}

	frontier, err := repo.GetInputObservation(ctx, address)
	require.NoError(t, err)
	require.Equal(t, uint64(100), frontier.ThroughBlock)
	require.Equal(t, uint64(2), frontier.NextInputIndex)
}

func TestDaveInputObservationRejectsIdentityChange(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	input := daveInput(0, 90)
	require.NoError(t, repo.ObserveDaveInputs(ctx, address, 90, 1,
		[]*model.Input{input}))

	// Same index at the same observed prefix, but a different L1 log.
	changed := *input
	changed.TransactionHash = common.HexToHash("0x1234")
	err := repo.ObserveDaveInputs(ctx, address, 90, 1,
		[]*model.Input{&changed})
	require.ErrorIs(t, err, repository.ErrInputLogIdentityConflict)
}

func TestProjectDaveOpenInputsRejectsNarrowing(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	open := &model.Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            90,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 2,
		Status:               model.EpochStatus_Open,
	}
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, address,
		map[*model.Epoch][]*model.Input{open: {daveInput(0, 90), daveInput(1, 90)}}, 90))

	projection := *open
	projection.LastBlock = 100
	projection.InputIndexUpperBound = 1
	err := repo.ProjectDaveOpenInputs(ctx, address, &projection, nil, 100)
	require.ErrorIs(t, err, repository.ErrEpochInputRangeConflict)

	stored, err := repo.GetEpoch(ctx, address, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(2), stored.InputIndexUpperBound)
	require.Equal(t, uint64(90), stored.LastBlock)
}

func TestProjectDaveOpenInputsRejectsSealedEpoch(t *testing.T) {
	ctx, repo, app := setupDaveInputRepository(t)
	address := app.IApplicationAddress.Hex()

	sealed := &model.Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            100,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 0,
		Status:               model.EpochStatus_Closed,
	}
	require.NoError(t, repo.AttributeSealedEpoch(ctx, address, sealed, nil))

	projection := *sealed
	projection.Status = model.EpochStatus_Open
	err := repo.ProjectDaveOpenInputs(ctx, address, &projection, nil, 100)
	require.ErrorIs(t, err, repository.ErrNoUpdate)

	stored, err := repo.GetEpoch(ctx, address, 0)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_Closed, stored.Status)
}

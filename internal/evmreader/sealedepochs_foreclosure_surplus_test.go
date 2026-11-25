// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"io"
	"log/slog"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/stretchr/testify/require"
)

// TestForeclosureSurplusAboveProjectionCapStaysObserved pins the foreclosure
// contract for observed-but-unsealed inputs. Once the OPEN epoch has been
// projected up to the 2^24 cap, further observed inputs stay in observed_input.
// No later EpochSealed can cover them after foreclosure, so they must neither
// block the drain gate nor be executed. They remain queryable as unattributed
// history.
func TestForeclosureSurplusAboveProjectionCapStaysObserved(t *testing.T) {
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
		inputBoxBlock  uint64 = 10
		forecloseBlock uint64 = 120
		surplus        uint64 = 5
	)
	app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Build()
	app.IInputBoxBlock = inputBoxBlock
	id, err := repo.CreateApplication(ctx, app, false)
	require.NoError(t, err)
	app.ID = id
	address := app.IApplicationAddress.Hex()

	// Epoch 0 is sealed; epoch 1 is OPEN and already projected to the protocol
	// cap. Its rows were projected and consumed by earlier ticks.
	sealed := &Epoch{
		Index:                0,
		FirstBlock:           inputBoxBlock,
		LastBlock:            50,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 0,
		Status:               EpochStatus_Closed,
	}
	cappedOpen := &Epoch{
		Index:                1,
		FirstBlock:           50,
		LastBlock:            100,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: MaxAdvanceStatesPerEpoch,
		Status:               EpochStatus_Open,
	}
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, address,
		map[*Epoch][]*Input{sealed: nil, cappedOpen: nil}, 100))

	// The backlog above the cap is observed at or before foreclosure. It cannot
	// fit in the current epoch and no further seal can arrive after teardown.
	backlog := make([]*Input, surplus)
	for i := range backlog {
		index := MaxAdvanceStatesPerEpoch + uint64(i)
		event := makeInputEvent(app.IApplicationAddress, index, 110)
		backlog[i] = &Input{
			Index:           index,
			BlockNumber:     110,
			RawData:         event.Input,
			Status:          InputCompletionStatus_None,
			TransactionHash: event.Raw.TxHash,
			LogIndex:        uint64(i),
		}
	}
	require.NoError(t, repo.ObserveDaveInputs(ctx, address, 110,
		MaxAdvanceStatesPerEpoch+surplus, backlog))

	observation, err := repo.GetInputObservation(ctx, address)
	require.NoError(t, err)
	reader := &Service{repository: repo}
	reader.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, reader.projectDaveOpenEpoch(ctx, appContracts{application: app},
		forecloseBlock, observation))

	// The projection publishes the input cursor at foreclosure without growing
	// past the cap; the capped epoch does not own the surplus.
	stored, err := repo.GetEpoch(ctx, address, 1)
	require.NoError(t, err)
	require.Equal(t, MaxAdvanceStatesPerEpoch, stored.InputIndexUpperBound)
	require.Equal(t, forecloseBlock, stored.LastBlock)
	require.Equal(t, EpochStatus_Open, stored.Status)

	app, err = repo.GetApplication(ctx, address)
	require.NoError(t, err)
	require.Equal(t, forecloseBlock, app.LastInputCheckBlock)

	require.NoError(t, repo.UpdateEventLastCheckBlock(ctx, []int64{app.ID},
		MonitoredEvent_EpochSealed, forecloseBlock))
	app, err = repo.GetApplication(ctx, address)
	require.NoError(t, err)
	require.True(t, app.ForeclosureScanCaughtUp(),
		"both Dave scans have reached foreclosure")

	undrained, err := repo.HasUndrainedEpochsBeforeBlock(ctx, app.ID, forecloseBlock)
	require.NoError(t, err)
	require.False(t, undrained,
		"unsealed surplus must not keep the machine alive after foreclosure")

	// The surplus stays in observed_input and is visible to historical lookups
	// as an unattributed input, not as part of the capped epoch.
	observed, err := repo.ListObservedInputs(ctx, address,
		MaxAdvanceStatesPerEpoch, MaxAdvanceStatesPerEpoch+surplus)
	require.NoError(t, err)
	require.Len(t, observed, int(surplus))
	for i, input := range observed {
		require.Equal(t, MaxAdvanceStatesPerEpoch+uint64(i), input.Index)
	}

	got, err := repo.GetInput(ctx, address, MaxAdvanceStatesPerEpoch)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Unattributed)
	require.Equal(t, uint64(110), got.BlockNumber)

	epochIndex := uint64(1)
	attributed, total, err := repo.ListInputs(ctx, address, repository.InputFilter{
		EpochIndex: &epochIndex,
	}, repository.Pagination{Limit: 10}, false)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, attributed)
}

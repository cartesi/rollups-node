// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type failingProjectionRepository struct {
	repository.Repository
}

func (r *failingProjectionRepository) ProjectDaveOpenInputs(
	context.Context, string, *Epoch, []*Input, uint64,
) error {
	return errors.New("projection interrupted")
}

// countObservedInputs exercises the Postgres-only counter without putting a
// test-only method on the shared repository interface.
func countObservedInputs(
	ctx context.Context,
	t *testing.T,
	repo repository.Repository,
	address string,
) uint64 {
	t.Helper()
	counter, ok := repo.(interface {
		CountObservedInputs(context.Context, string) (uint64, error)
	})
	require.True(t, ok, "repository must be a Postgres repository")
	count, err := counter.CountObservedInputs(ctx, address)
	require.NoError(t, err)
	return count
}

// Observation persists the input and its frontier before the OPEN projection
// publishes the input cursor. A failed projection therefore leaves the cursor
// retryable without re-reading L1, and foreclosure readiness stays false until
// the projection succeeds.
func TestDaveInputCheckpointAfterFailedProjection(t *testing.T) {
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
		inputBoxBlock     uint64 = 10
		previousSealBlock uint64 = 50
		forecloseBlock    uint64 = 100
	)
	app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Build()
	app.IInputBoxBlock = inputBoxBlock
	id, err := repo.CreateApplication(ctx, app, false)
	require.NoError(t, err)
	app.ID = id
	address := app.IApplicationAddress.Hex()

	previousEpoch := &Epoch{
		Index:                0,
		FirstBlock:           inputBoxBlock,
		LastBlock:            previousSealBlock,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 0,
		Status:               EpochStatus_Closed,
	}
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, address,
		map[*Epoch][]*Input{previousEpoch: nil}, forecloseBlock-2))
	app, err = repo.GetApplication(ctx, address)
	require.NoError(t, err)
	require.Equal(t, forecloseBlock-2, app.LastInputCheckBlock)

	inputBox := newMockInputBox()
	inputBox.On("GetNumberOfInputs", blockRange(0, forecloseBlock), app.IApplicationAddress).
		Return(new(big.Int), nil)
	inputBox.On("GetNumberOfInputs", blockFrom(forecloseBlock), app.IApplicationAddress).
		Return(new(big.Int).SetUint64(1), nil)
	inputEvents := []iinputbox.IInputBoxInputAdded{
		makeInputEvent(app.IApplicationAddress, 0, forecloseBlock),
	}
	inputBlock := mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == forecloseBlock && opts.End != nil && *opts.End == forecloseBlock
	})
	inputBox.On("RetrieveInputs", inputBlock, []common.Address{app.IApplicationAddress}, mock.Anything).
		Return(inputEvents, nil)

	contracts := appContracts{application: app, inputSource: inputBox}
	reader := &Service{repository: repo}
	reader.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	observation, err := reader.observeDaveInputs(ctx, contracts, forecloseBlock)
	require.NoError(t, err)
	require.Equal(t, forecloseBlock, observation.ThroughBlock)
	require.Equal(t, uint64(1), observation.NextInputIndex)

	// The observation is durable, but the input cursor is still at the last
	// complete projection.
	app, err = repo.GetApplication(ctx, address)
	require.NoError(t, err)
	assert.Equal(t, forecloseBlock-2, app.LastInputCheckBlock)
	observed := countObservedInputs(ctx, t, repo, address)
	assert.Equal(t, uint64(1), observed)

	require.NoError(t, repo.UpdateEventLastCheckBlock(ctx, []int64{app.ID},
		MonitoredEvent_EpochSealed, forecloseBlock))
	require.NoError(t, repo.UpdateApplicationForeclosure(ctx, app.ID, forecloseBlock,
		repotest.UniqueHash(), forecloseBlock))
	app, err = repo.GetApplication(ctx, address)
	require.NoError(t, err)
	contracts.application = app
	assert.False(t, app.ForeclosureScanCaughtUp(),
		"drain must wait for the observation to be projected")

	failing := &failingProjectionRepository{Repository: repo}
	failingReader := &Service{repository: failing}
	failingReader.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	err = failingReader.projectDaveOpenEpoch(ctx, contracts, forecloseBlock, observation)
	require.ErrorContains(t, err, "projection interrupted")

	app, err = repo.GetApplication(ctx, address)
	require.NoError(t, err)
	assert.Equal(t, forecloseBlock-2, app.LastInputCheckBlock,
		"a failed projection must not publish the input cursor")
	assert.False(t, app.ForeclosureScanCaughtUp())
	openEpoch, err := repo.GetEpoch(ctx, address, 1)
	require.NoError(t, err)
	require.Nil(t, openEpoch)
	observed = countObservedInputs(ctx, t, repo, address)
	assert.Equal(t, uint64(1), observed, "the observed input survives the failed projection")

	// Retry publishes the same observed input without re-reading L1.
	inputBox.Unset("RetrieveInputs")
	observation, err = repo.GetInputObservation(ctx, address)
	require.NoError(t, err)
	require.NoError(t, reader.projectDaveOpenEpoch(ctx, contracts, forecloseBlock, observation))

	input, err := repo.GetInput(ctx, address, 0)
	require.NoError(t, err)
	require.NotNil(t, input)
	assert.Equal(t, uint64(1), input.EpochIndex)
	assert.Equal(t, forecloseBlock, input.BlockNumber)

	openEpoch, err = repo.GetEpoch(ctx, address, 1)
	require.NoError(t, err)
	require.NotNil(t, openEpoch)
	assert.Equal(t, previousSealBlock, openEpoch.FirstBlock)
	assert.Equal(t, forecloseBlock, openEpoch.LastBlock)
	assert.Equal(t, uint64(0), openEpoch.InputIndexLowerBound)
	assert.Equal(t, uint64(1), openEpoch.InputIndexUpperBound)
	assert.Equal(t, EpochStatus_Open, openEpoch.Status)

	observed = countObservedInputs(ctx, t, repo, address)
	assert.Zero(t, observed)

	app, err = repo.GetApplication(ctx, address)
	require.NoError(t, err)
	assert.Equal(t, forecloseBlock, app.LastInputCheckBlock)
	assert.True(t, app.ForeclosureScanCaughtUp())
	undrained, err := repo.HasUndrainedEpochsBeforeBlock(ctx, app.ID, forecloseBlock)
	require.NoError(t, err)
	assert.True(t, undrained, "the stored input must remain visible to the machine drain")
	inputBox.AssertExpectations(t)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres_test

import (
	"context"
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

func TestPostgresReplayVerificationLevels(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))

	ctx := context.Background()
	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(ctx)) })

	app := repotest.NewApplicationBuilder().Create(ctx, t, repo)
	epoch := repotest.NewEpochBuilder(app.ID).
		WithStatus(model.EpochStatus_Closed).
		WithInputBounds(0, 1).
		Build()
	inputs := []*model.Input{
		repotest.NewInputBuilder().WithIndex(0).Build(),
		repotest.NewInputBuilder().WithIndex(1).Build(),
	}
	require.NoError(t, repo.CreateEpochsAndInputs(
		ctx,
		app.IApplicationAddress.String(),
		map[*model.Epoch][]*model.Input{epoch: inputs},
		10,
	))
	require.NoError(t, repo.StoreAdvanceResult(ctx, app.ID, &model.AdvanceResult{
		EpochIndex: 0,
		InputIndex: 0,
		Status:     model.InputCompletionStatus_Accepted,
		Outputs:    [][]byte{[]byte("output")},
		Reports:    [][]byte{[]byte("report")},
		OutputsProof: model.OutputsProof{
			MachineHash: repotest.UniqueHash(),
			OutputsHash: repotest.UniqueHash(),
		},
	}))
	require.NoError(t, repo.StoreAdvanceResult(ctx, app.ID, &model.AdvanceResult{
		EpochIndex: 0,
		InputIndex: 1,
		Status:     model.InputCompletionStatus_Rejected,
		OutputsProof: model.OutputsProof{
			MachineHash: repotest.UniqueHash(),
			OutputsHash: repotest.UniqueHash(),
		},
	}))

	canonical, err := repo.ReplaySummary(
		ctx, app.IApplicationAddress, repository.ReplayVerificationCanonical,
	)
	require.NoError(t, err)
	require.Equal(t, app.ID, canonical.ApplicationID)
	require.Equal(t, uint64(2), canonical.ProcessedInputs)
	require.Equal(t, model.Consensus_Authority, canonical.Consensus)

	canonicalPage, err := repo.ReplayPage(ctx, repository.ReplayPageRequest{
		ApplicationID:    canonical.ApplicationID,
		FromInput:        0,
		ToInputExclusive: canonical.ProcessedInputs,
		Limit:            canonical.ProcessedInputs,
		Verification:     repository.ReplayVerificationCanonical,
	})
	require.NoError(t, err)
	require.Len(t, canonicalPage, 2)
	for _, record := range canonicalPage {
		require.Empty(t, record.Outputs)
		require.Empty(t, record.Reports)
		require.Empty(t, record.StateHashes)
	}

	_, err = repo.ReplaySummary(
		ctx, app.IApplicationAddress, repository.ReplayVerificationFull,
	)
	require.NoError(t, err)
	fullPage, err := repo.ReplayPage(ctx, repository.ReplayPageRequest{
		ApplicationID:    canonical.ApplicationID,
		FromInput:        0,
		ToInputExclusive: canonical.ProcessedInputs,
		Limit:            canonical.ProcessedInputs,
		Verification:     repository.ReplayVerificationFull,
	})
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("output")}, fullPage[0].Outputs)
	require.Equal(t, [][]byte{[]byte("report")}, fullPage[0].Reports)

	// Poison only full evidence. Canonical reconstruction remains independent
	// of child evidence, while a full page exposes it to replay comparison.
	_, err = conn.Exec(ctx, `
		INSERT INTO output (input_epoch_application_id, input_index, index, raw_data)
		VALUES ($1, 1, 1, $2)`, app.ID, []byte("illegal-output"))
	require.NoError(t, err)
	_, err = repo.ReplaySummary(
		ctx, app.IApplicationAddress, repository.ReplayVerificationCanonical,
	)
	require.NoError(t, err)
	fullPage, err = repo.ReplayPage(ctx, repository.ReplayPageRequest{
		ApplicationID:    canonical.ApplicationID,
		FromInput:        0,
		ToInputExclusive: canonical.ProcessedInputs,
		Limit:            canonical.ProcessedInputs,
		Verification:     repository.ReplayVerificationFull,
	})
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("illegal-output")}, fullPage[1].Outputs)

	// State hashes are invalid for every Authority input, including completed
	// inputs whose other Full evidence is otherwise valid.
	illegalStateHash := repotest.UniqueHash()
	_, err = conn.Exec(ctx, `INSERT INTO state_hashes (
		input_epoch_application_id, epoch_index, input_index, index, machine_hash, repetitions
	) VALUES ($1, 0, 1, 0, $2, 1)`, app.ID, illegalStateHash.Bytes())
	require.NoError(t, err)
	_, err = repo.ReplaySummary(
		ctx, app.IApplicationAddress, repository.ReplayVerificationCanonical,
	)
	require.NoError(t, err)
	_, err = repo.ReplaySummary(
		ctx, app.IApplicationAddress, repository.ReplayVerificationFull,
	)
	violation := requireReplayViolationKind(
		t, err, repository.ReplayStructureUnexpectedStateHash,
	)
	require.Equal(t, uint64(1), violation.InputIndex)
	require.Equal(t, uint64(0), violation.EvidenceIndex)
	_, err = conn.Exec(ctx, `DELETE FROM state_hashes WHERE input_epoch_application_id = $1`, app.ID)
	require.NoError(t, err)

	// The persisted application counter and completed input rows are one
	// invariant and must agree in the same repository snapshot.
	_, err = conn.Exec(ctx, `UPDATE application SET processed_inputs = 1 WHERE id = $1`, app.ID)
	require.NoError(t, err)
	_, err = repo.ReplaySummary(
		ctx, app.IApplicationAddress, repository.ReplayVerificationCanonical,
	)
	violation = requireReplayViolationKind(
		t, err, repository.ReplayStructureProcessedInputCount,
	)
	require.Equal(t, uint64(1), violation.ApplicationProcessedInputs)
	require.Equal(t, uint64(2), violation.CompletedInputCount)
}

func TestPostgresReplayRejectsCompletedInputGap(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))

	ctx := context.Background()
	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(ctx)) })

	app := repotest.NewApplicationBuilder().Create(ctx, t, repo)
	epoch := repotest.NewEpochBuilder(app.ID).
		WithStatus(model.EpochStatus_Closed).
		WithInputBounds(0, 1).
		Build()
	inputs := []*model.Input{
		repotest.NewInputBuilder().WithIndex(0).Build(),
		repotest.NewInputBuilder().WithIndex(1).Build(),
	}
	require.NoError(t, repo.CreateEpochsAndInputs(
		ctx,
		app.IApplicationAddress.String(),
		map[*model.Epoch][]*model.Input{epoch: inputs},
		10,
	))

	// Create two completed rows at indexes 0 and 2. Changing the index while
	// the input is still pending is allowed by the completion immutability
	// trigger and models a malformed completed prefix without disabling it.
	_, err = conn.Exec(ctx, `UPDATE input SET index = 2
		WHERE epoch_application_id = $1 AND index = 1`, app.ID)
	require.NoError(t, err)
	machineHash := repotest.UniqueHash()
	outputsHash := repotest.UniqueHash()
	_, err = conn.Exec(ctx, `UPDATE input
		SET status = 'ACCEPTED', machine_hash = $2, outputs_hash = $3
		WHERE epoch_application_id = $1`, app.ID, machineHash.Bytes(), outputsHash.Bytes())
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `UPDATE application SET processed_inputs = 2 WHERE id = $1`, app.ID)
	require.NoError(t, err)

	_, err = repo.ReplaySummary(
		ctx, app.IApplicationAddress, repository.ReplayVerificationCanonical,
	)
	violation := requireReplayViolationKind(
		t, err, repository.ReplayStructureCompletedInputSequence,
	)
	require.Equal(t, uint64(2), violation.InputIndex)
	require.Equal(t, uint64(1), violation.ExpectedIndex)
}

func TestPostgresReplayRejectsInvalidStateHashOrdering(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))

	ctx := context.Background()
	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(ctx)) })

	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(ctx, t, repo)
	epoch := repotest.NewEpochBuilder(app.ID).
		WithStatus(model.EpochStatus_Closed).
		WithInputBounds(0, 1).
		Build()
	inputs := []*model.Input{
		repotest.NewInputBuilder().WithIndex(0).Build(),
		repotest.NewInputBuilder().WithIndex(1).Build(),
	}
	require.NoError(t, repo.CreateEpochsAndInputs(
		ctx,
		app.IApplicationAddress.String(),
		map[*model.Epoch][]*model.Input{epoch: inputs},
		10,
	))
	for inputIndex := range uint64(2) {
		require.NoError(t, repo.StoreAdvanceResult(ctx, app.ID, &model.AdvanceResult{
			EpochIndex:         0,
			InputIndex:         inputIndex,
			Status:             model.InputCompletionStatus_Accepted,
			IsDaveConsensus:    true,
			PaddingRepetitions: 1 << 24,
			OutputsProof: model.OutputsProof{
				MachineHash: repotest.UniqueHash(),
				OutputsHash: repotest.UniqueHash(),
			},
		}))
	}
	_, err = repo.ReplaySummary(ctx, app.IApplicationAddress, repository.ReplayVerificationFull)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, `UPDATE state_hashes SET index = 2 WHERE index = 1`)
	require.NoError(t, err)
	_, err = repo.ReplaySummary(ctx, app.IApplicationAddress, repository.ReplayVerificationFull)
	_ = requireReplayViolationKind(t, err, repository.ReplayStructureStateHashIndexSequence)
	_, err = conn.Exec(ctx, `UPDATE state_hashes SET index = 1 WHERE index = 2`)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, `UPDATE state_hashes
		SET input_index = CASE index WHEN 0 THEN 1 ELSE 0 END`)
	require.NoError(t, err)
	_, err = repo.ReplaySummary(ctx, app.IApplicationAddress, repository.ReplayVerificationFull)
	_ = requireReplayViolationKind(t, err, repository.ReplayStructureStateHashInputOrder)
}

func requireReplayViolationKind(
	t *testing.T,
	err error,
	want repository.ReplayStructureViolationKind,
) *repository.ReplayStructureViolationError {
	t.Helper()
	require.ErrorIs(t, err, repository.ErrReplayInvalidStructure)
	var violation *repository.ReplayStructureViolationError
	require.ErrorAs(t, err, &violation)
	require.Equal(t, want, violation.Kind)
	return violation
}

func TestPostgresReplayRejectsInvalidRequests(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))

	ctx := context.Background()
	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	app := repotest.NewApplicationBuilder().Create(ctx, t, repo)

	_, err = repo.ReplaySummary(ctx, app.IApplicationAddress, repository.ReplayVerificationLevel(255))
	require.ErrorContains(t, err, "unsupported replay verification level")
	_, err = repo.ReplaySummary(
		ctx,
		common.HexToAddress("0xdead"),
		repository.ReplayVerificationCanonical,
	)
	require.ErrorIs(t, err, repository.ErrNotFound)
	_, err = repo.ReplayPage(ctx, repository.ReplayPageRequest{
		ApplicationID:    app.ID,
		FromInput:        1,
		ToInputExclusive: 0,
		Limit:            1,
		Verification:     repository.ReplayVerificationCanonical,
	})
	require.ErrorContains(t, err, "replay input range is invalid")
}

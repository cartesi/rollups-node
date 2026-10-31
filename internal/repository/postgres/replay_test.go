// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestStateHashInsertShape(t *testing.T) {
	t.Parallel()

	t.Run("positive padding appends final row", func(t *testing.T) {
		rows, err := stateHashInsertShape(
			1, model.InputHashCollectionCapacity-1,
		)
		require.NoError(t, err)
		require.Equal(t, uint64(2), rows)
	})

	t.Run("empty collection uses one positive padding row", func(t *testing.T) {
		rows, err := stateHashInsertShape(0, model.InputHashCollectionCapacity)
		require.NoError(t, err)
		require.Equal(t, uint64(1), rows)
	})

	t.Run("full span has no padding row", func(t *testing.T) {
		rows, err := stateHashInsertShape(model.InputHashCollectionCapacity, 0)
		require.NoError(t, err)
		require.Equal(t, model.InputHashCollectionCapacity, rows)
	})

	t.Run("rejects incomplete span", func(t *testing.T) {
		_, err := stateHashInsertShape(1, 7)
		require.ErrorContains(t, err, "does not cover input hash collection capacity")
	})

}

func TestStateHashCopySource(t *testing.T) {
	t.Parallel()

	hashes := [][32]byte{common.HexToHash("0x11"), common.HexToHash("0x12")}
	finalEntry := common.HexToHash("0x13")
	for _, withPadding := range []bool{false, true} {
		name := "without padding"
		padding := uint64(0)
		expected := hashes
		if withPadding {
			name = "with padding"
			padding = model.InputHashCollectionCapacity - uint64(len(hashes))
			expected = append(expected, finalEntry)
		}
		t.Run(name, func(t *testing.T) {
			source := &stateHashCopySource{
				appID:              7,
				epochIndex:         8,
				inputIndex:         9,
				nextIndex:          10,
				hashes:             hashes,
				finalEntry:         finalEntry,
				paddingRepetitions: padding,
				rowCount:           uint64(len(expected)),
			}
			for index, expectedHash := range expected {
				require.True(t, source.Next())
				values, err := source.Values()
				require.NoError(t, err)
				require.Equal(t, int64(7), values[0])
				require.Equal(t, uint64(8), values[1])
				require.Equal(t, uint64(9), values[2])
				require.Equal(t, uint64(10+index), values[3])
				require.Equal(t, expectedHash[:], values[4])
				if index < len(hashes) {
					require.Equal(t, int64(1), values[5])
				} else {
					require.Equal(t, padding, values[5])
				}
			}
			require.False(t, source.Next())
			require.NoError(t, source.Err())
		})
	}
}

func TestPostgresStateHashesRequirePositiveRepetitions(t *testing.T) {
	repo := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch := seedEpochPublication(t, repo)
	_, err := repo.db.Exec(t.Context(), `
		INSERT INTO state_hashes
			(input_epoch_application_id, epoch_index, input_index, index, machine_hash, repetitions)
		VALUES ($1, $2, 0, 0, $3, 0)`, app.ID, epoch.Index, repotest.UniqueHash().Bytes())
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23514", pgErr.Code)
	require.Equal(t, "state_hashes_repetitions_check", pgErr.ConstraintName)
}

func TestInsertStateHashesQualifiesPublicSchema(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	ctx := context.Background()

	// Remove a shadow schema left by an interrupted prior run before schema
	// migrations try to drop the uint64/hash domains it may reference.
	bootstrap, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	_, err = bootstrap.Exec(ctx, `DROP SCHEMA IF EXISTS state_hash_copy_shadow CASCADE`)
	require.NoError(t, err)
	require.NoError(t, bootstrap.Close(ctx))
	require.NoError(t, db.SetupTestPostgres(endpoint))

	repositoryInterface, err := NewPostgresRepository(ctx, endpoint, 1, 0)
	require.NoError(t, err)
	repo := repositoryInterface.(*PostgresRepository)
	t.Cleanup(func() {
		_, _ = repo.db.Exec(ctx, `DROP SCHEMA IF EXISTS state_hash_copy_shadow CASCADE`)
		repo.Close()
	})

	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(
		ctx, t, repo,
	)
	epoch := repotest.NewEpochBuilder(app.ID).
		WithStatus(model.EpochStatus_Closed).
		WithInputBounds(0, 0).
		Build()
	input := repotest.NewInputBuilder().WithIndex(0).Build()
	require.NoError(t, repo.CreateEpochsAndInputs(
		ctx,
		app.IApplicationAddress.String(),
		map[*model.Epoch][]*model.Input{epoch: {input}},
		1,
	))

	_, err = repo.db.Exec(ctx, `
		CREATE SCHEMA state_hash_copy_shadow;
		CREATE TABLE state_hash_copy_shadow.state_hashes
			(LIKE public.state_hashes INCLUDING ALL)`)
	require.NoError(t, err)
	tx, err := repo.db.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck
	_, err = tx.Exec(ctx, `SET LOCAL search_path TO state_hash_copy_shadow, public`)
	require.NoError(t, err)
	machineHash := repotest.UniqueHash()
	require.NoError(t, insertStateHashes(
		ctx,
		tx,
		app.ID,
		0,
		0,
		nil,
		machineHash,
		model.InputHashCollectionCapacity,
	))
	require.NoError(t, tx.Commit(ctx))

	var publicCount, shadowCount uint64
	require.NoError(t, repo.db.QueryRow(ctx, `SELECT count(*) FROM public.state_hashes`).Scan(&publicCount))
	require.NoError(t, repo.db.QueryRow(
		ctx,
		`SELECT count(*) FROM state_hash_copy_shadow.state_hashes`,
	).Scan(&shadowCount))
	require.Equal(t, uint64(1), publicCount)
	require.Zero(t, shadowCount)
}

func TestReplayRecordForChild(t *testing.T) {
	t.Parallel()

	record := new(model.ReplayRecord)
	got, err := replayRecordForChild(
		map[uint64]*model.ReplayRecord{4: record},
		repository.ReplayEvidenceOutput,
		4,
	)
	require.NoError(t, err)
	require.Same(t, record, got)

	for _, childKind := range []repository.ReplayEvidenceKind{
		repository.ReplayEvidenceOutput,
		repository.ReplayEvidenceReport,
		repository.ReplayEvidenceStateHash,
	} {
		t.Run(childKind.String(), func(t *testing.T) {
			const inputWithoutCompletedRecord = uint64(7)
			_, err := replayRecordForChild(nil, childKind, inputWithoutCompletedRecord)

			require.ErrorIs(t, err, repository.ErrReplayInconsistentEvidence)
			var detail *repository.ReplayInconsistentEvidenceError
			require.ErrorAs(t, err, &detail)
			require.Equal(t, childKind, detail.Kind)
			require.Equal(t, inputWithoutCompletedRecord, detail.InputIndex)
			require.Contains(t, err.Error(), childKind.String())
			require.Contains(t, err.Error(), "input 7")
			require.NotContains(t, err.Error(), "payload-must-stay-private")
		})
	}
}

func TestReplayInconsistencyErrorIdentity(t *testing.T) {
	t.Parallel()
	err := &repository.ReplayInconsistentEvidenceError{
		Kind:       repository.ReplayEvidenceReport,
		InputIndex: 11,
	}
	require.True(t, errors.Is(err, repository.ErrReplayInconsistentEvidence))
}

func TestReplayStructureViolationErrorIdentityAndPayloadHygiene(t *testing.T) {
	t.Parallel()
	epochIndex := uint64(3)
	for _, kind := range []repository.ReplayStructureViolationKind{
		repository.ReplayStructureCompletedInputSequence,
		repository.ReplayStructureStateHashIndexSequence,
		repository.ReplayStructureStateHashInputOrder,
		repository.ReplayStructureProcessedInputCount,
		repository.ReplayStructureUnexpectedStateHash,
	} {
		t.Run(kind.String(), func(t *testing.T) {
			err := &repository.ReplayStructureViolationError{
				Kind:               kind,
				EpochIndex:         &epochIndex,
				InputIndex:         4,
				EvidenceIndex:      7,
				ExpectedIndex:      6,
				PreviousInputIndex: 5,
			}
			require.ErrorIs(t, err, repository.ErrReplayInvalidStructure)
			require.Contains(t, err.Error(), "kind="+kind.String())
			require.NotContains(t, err.Error(), "payload-must-stay-private")
		})
	}
}

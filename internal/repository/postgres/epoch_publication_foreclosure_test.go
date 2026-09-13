// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func newEpochPublicationRepository(t *testing.T) repository.Repository {
	t.Helper()
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skip(err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))
	repo, err := NewPostgresRepository(t.Context(), endpoint, 1, 0)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	return repo
}

func seedEpochPublication(t *testing.T, repo repository.Repository) (*Application, *Epoch) {
	t.Helper()
	app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(t.Context(), t, repo)
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(EpochStatus_Closed).
		WithBlocks(0, 9).WithInputBounds(0, 1).Build()
	input := repotest.NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
	require.NoError(t, repo.CreateEpochsAndInputs(t.Context(), app.Name, map[*Epoch][]*Input{epoch: {input}}, 10))
	require.NoError(t, repo.StoreAdvanceResult(t.Context(), app.ID, &AdvanceResult{
		EpochIndex: epoch.Index, InputIndex: input.Index, Status: InputCompletionStatus_Accepted,
		StateProof: *repotest.DummyStateProof(), Outputs: [][]byte{[]byte("output")},
	}))
	return app, epoch
}

func TestEpochPublicationLosesToForeclosureWithoutWrites(t *testing.T) {
	for _, claimPublication := range []bool{false, true} {
		name := "closed state proof"
		if claimPublication {
			name = "inputs processed claim"
		}
		t.Run(name, func(t *testing.T) {
			repo := newEpochPublicationRepository(t)
			app, epoch := seedEpochPublication(t, repo)
			if claimPublication {
				require.NoError(t, repo.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof()))
			}
			// The publisher holds this snapshot before PRT wins the terminal transition.
			stale, err := repo.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.NoError(t, repo.UpdateApplicationForeclosure(t.Context(), app.ID, 100, repotest.UniqueHash(), 100))
			require.NoError(t, repo.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, epoch.Index))
			before, err := repo.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			outputsBefore, _, err := repo.ListOutputs(t.Context(), app.Name, repository.OutputFilter{}, repository.Pagination{}, false)
			require.NoError(t, err)
			require.Len(t, outputsBefore, 1)

			if claimPublication {
				stale.Commitment = new(repotest.UniqueHash())
				stale.CommitmentProof = make([]common.Hash, Log2EpochComputationHashLeafCount)
				output := *outputsBefore[0]
				output.Hash = new(repotest.UniqueHash())
				output.OutputHashesSiblings = []common.Hash{repotest.UniqueHash()}
				err = repo.StoreClaimAndProofs(t.Context(), stale, []*Output{&output})
			} else {
				proof := repotest.DummyStateProof()
				proof.MachineHash = repotest.UniqueHash()
				proof.TxBufferDataBlock = repotest.UniqueHash()
				err = repo.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof)
			}
			require.ErrorIs(t, err, repository.ErrEpochForeclosed)
			after, err := repo.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, before, after, "a rejected publication must preserve status, proof fields, and timestamps")
			outputsAfter, _, err := repo.ListOutputs(t.Context(), app.Name, repository.OutputFilter{}, repository.Pagination{}, false)
			require.NoError(t, err)
			require.Equal(t, outputsBefore, outputsAfter, "claim rejection must occur before any output-proof write")
			require.ErrorIs(t, repo.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, &StateProof{}),
				repository.ErrInvalidStateProof, "foreclosure must not bypass proof completeness checks")
		})
	}
}

func TestEpochPublicationDoesNotMisclassifyOtherFailures(t *testing.T) {
	repo := newEpochPublicationRepository(t)
	app, epoch := seedEpochPublication(t, repo)
	foreclosedApp, foreclosedEpoch := seedEpochPublication(t, repo)
	require.NoError(t, repo.UpdateApplicationForeclosure(t.Context(), foreclosedApp.ID, 100, repotest.UniqueHash(), 100))
	require.NoError(t, repo.UpdateEpochWithForeclosedClaim(t.Context(), foreclosedApp.ID, foreclosedEpoch.Index))
	for _, test := range []struct {
		name      string
		appID     int64
		appName   string
		epoch     uint64
		proofOnly bool
	}{
		{name: "claim closed epoch", appID: app.ID, epoch: epoch.Index},
		{name: "claim missing epoch", appID: foreclosedApp.ID, epoch: 99},
		{name: "claim missing application", appID: -1, epoch: epoch.Index},
		{name: "proof missing epoch", appName: foreclosedApp.Name, epoch: 99, proofOnly: true},
		{name: "proof missing application", appName: "missing", epoch: epoch.Index, proofOnly: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.proofOnly {
				err = repo.UpdateEpochInputsProcessed(t.Context(), test.appName, test.epoch, repotest.DummyStateProof())
			} else {
				err = repo.StoreClaimAndProofs(t.Context(), &Epoch{ApplicationID: test.appID, Index: test.epoch}, nil)
			}
			require.ErrorIs(t, err, repository.ErrNoUpdate)
			require.NotErrorIs(t, err, repository.ErrEpochForeclosed)
		})
	}

	// Normal publication still works in the presence of another foreclosed app.
	require.NoError(t, repo.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof()))
	err := repo.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof())
	require.ErrorIs(t, err, repository.ErrNoUpdate, "an already published proof is not foreclosure")
	computed, err := repo.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	computed.Commitment, computed.CommitmentProof = new(repotest.UniqueHash()), make([]common.Hash, Log2EpochComputationHashLeafCount)
	require.NoError(t, repo.StoreClaimAndProofs(t.Context(), computed, nil))
	stored, err := repo.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, EpochStatus_ClaimComputed, stored.Status)
	require.Equal(t, computed.Commitment, stored.Commitment)
	require.Equal(t, computed.CommitmentProof, stored.CommitmentProof)

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, err := range []error{
		repo.UpdateEpochInputsProcessed(canceled, app.Name, epoch.Index, repotest.DummyStateProof()),
		repo.StoreClaimAndProofs(canceled, computed, nil),
	} {
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, repository.ErrEpochForeclosed)
	}
}

type failedPublicationStatusRow struct{ err error }

func (r failedPublicationStatusRow) Scan(...any) error { return r.err }

func TestEpochPublicationStatusReadFailure(t *testing.T) {
	failure := errors.New("status read failed")
	err := classifyEpochPublicationMiss(failedPublicationStatusRow{err: failure})
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, repository.ErrEpochForeclosed)
	require.ErrorIs(t, classifyEpochPublicationMiss(failedPublicationStatusRow{err: pgx.ErrNoRows}), repository.ErrNoUpdate)
}

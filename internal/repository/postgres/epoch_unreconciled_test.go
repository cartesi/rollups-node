// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"fmt"
	"slices"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

func TestUnreconciledIndexMatchesNonTerminalEpochStatuses(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	var predicate string
	err := r.db.QueryRow(t.Context(), `
		SELECT pg_get_expr(indpred, indrelid) FROM pg_index
		WHERE indexrelid = 'epoch_unreconciled_idx'::regclass`).Scan(&predicate)
	require.NoError(t, err)
	require.NotEmpty(t, predicate)
	for _, status := range EpochStatusAllValues {
		// Evaluate the actual migrated predicate, not a copied status list.
		query := fmt.Sprintf(`SELECT %s FROM (SELECT $1::"EpochStatus" AS status) AS epoch`, predicate)
		var indexed bool
		require.NoError(t, r.db.QueryRow(t.Context(), query, status.String()).Scan(&indexed))
		require.Equal(t, slices.Contains(NonTerminalEpochStatuses(), status), indexed, status)
	}
}

func TestForeclosureQueriesUseNonTerminalEpochStatuses(t *testing.T) {
	r := newEpochPublicationRepository(t)
	for _, action := range []struct {
		name      string
		consensus Consensus
		bulk      bool
	}{
		{name: "single Authority", consensus: Consensus_Authority},
		{name: "single PRT", consensus: Consensus_PRT},
		{name: "bulk Authority", consensus: Consensus_Authority, bulk: true},
		{name: "bulk Quorum", consensus: Consensus_Quorum, bulk: true},
		{name: "bulk excludes PRT", consensus: Consensus_PRT, bulk: true},
	} {
		for _, status := range EpochStatusAllValues {
			t.Run(action.name+"/"+status.String(), func(t *testing.T) {
				app, epoch := seedForeclosureStatus(t, r, action.consensus, status)
				nonTerminal := slices.Contains(NonTerminalEpochStatuses(), status)
				pending, err := r.HasUnreconciledClaimsBeforeBlock(t.Context(), app.ID, app.ForecloseBlock)
				require.NoError(t, err)
				require.Equal(t, nonTerminal, pending)
				canForeclose := nonTerminal && (!action.bulk || action.consensus != Consensus_PRT)
				if action.bulk {
					count, err := r.ForecloseUnacceptedEpochsAtOrAfterBlock(t.Context(), app.ID, app.ForecloseBlock)
					require.NoError(t, err)
					if canForeclose {
						require.Equal(t, int64(1), count)
					} else {
						require.Zero(t, count)
					}
				} else {
					err := r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, epoch.Index)
					if canForeclose {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, repository.ErrNoUpdate)
					}
				}
				stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
				require.NoError(t, err)
				wantStatus := status
				if canForeclose {
					wantStatus = EpochStatus_ClaimForeclosed
				}
				require.Equal(t, wantStatus, stored.Status)
				pending, err = r.HasUnreconciledClaimsBeforeBlock(t.Context(), app.ID, app.ForecloseBlock)
				require.NoError(t, err)
				require.Equal(t, nonTerminal && !canForeclose, pending)
			})
		}
	}
}

func seedForeclosureStatus(t *testing.T, r repository.Repository, consensus Consensus, status EpochStatus) (*Application, *Epoch) {
	t.Helper()
	app := repotest.NewApplicationBuilder().WithConsensus(consensus).Create(t.Context(), t, r)
	initial := EpochStatus_Closed
	if status == EpochStatus_Open {
		initial = status
	}
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(initial).WithBlocks(0, 9).WithInputBounds(0, 0).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*Epoch][]*Input{epoch: {}}, 10))
	if status == EpochStatus_ClaimStaged {
		repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epoch, EpochStatus_ClaimSubmitted)
		require.NoError(t, r.UpdateEpochToStaged(t.Context(), app.ID, epoch.Index, 4))
	} else if status != initial && status != EpochStatus_ClaimForeclosed {
		repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epoch, status)
	}
	app.ForecloseBlock = 5
	require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, app.ForecloseBlock, repotest.UniqueHash(), 10))
	if status == EpochStatus_ClaimForeclosed {
		require.NoError(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, epoch.Index))
	}
	return app, epoch
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	jet "github.com/go-jet/jet/v2/postgres"
	"github.com/stretchr/testify/require"
)

type claimPageSelector func(context.Context, int64, uint64) (
	map[int64]*model.Epoch, map[int64]*model.Application, error,
)

func claimPageMaps(
	query func(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error),
) claimPageSelector {
	return func(ctx context.Context, afterID int64, limit uint64) (
		map[int64]*model.Epoch, map[int64]*model.Application, error,
	) {
		page, err := query(ctx, repository.ClaimCursor{AfterApplicationID: afterID}, max(limit, 2))
		epochs := make(map[int64]*model.Epoch, len(page.Work))
		apps := make(map[int64]*model.Application, len(page.Work))
		for _, work := range page.Work {
			if _, exists := epochs[work.Application.ID]; !exists {
				epochs[work.Application.ID], apps[work.Application.ID] = work.Epoch, work.Application
			}
		}
		return epochs, apps, err
	}
}

func seedClaimPageEpochs(
	t *testing.T, r *PostgresRepository, app *model.Application, statuses ...model.EpochStatus,
) []*model.Epoch {
	t.Helper()
	epochs := make([]*model.Epoch, len(statuses))
	inputs := make(map[*model.Epoch][]*model.Input, len(statuses))
	for i := range statuses {
		index := uint64(i)
		epochs[i] = repotest.NewEpochBuilder(app.ID).WithIndex(index).WithStatus(model.EpochStatus_Closed).
			WithBlocks(index*10, index*10+9).WithInputBounds(index, index).Build()
		inputs[epochs[i]] = []*model.Input{repotest.NewInputBuilder().WithIndex(index).WithBlockNumber(index*10 + 5).Build()}
	}
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, inputs, uint64(len(statuses))*10))
	for i, status := range statuses {
		if status == model.EpochStatus_ClaimStaged {
			repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epochs[i], model.EpochStatus_ClaimComputed)
			require.NoError(t, r.UpdateEpochWithSubmittedClaim(t.Context(), app.ID, uint64(i), repotest.UniqueHash()))
			require.NoError(t, r.UpdateEpochToStaged(t.Context(), app.ID, uint64(i), 100+uint64(i)))
		} else {
			repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epochs[i], status)
		}
	}
	return epochs
}

func consumeClaimPage(cursor *repository.ClaimCursor, page repository.ClaimPage) {
	cursor.AfterApplicationID = page.ApplicationID
	for _, work := range page.Work {
		cursor.Advance(work, page)
	}
}

func TestClaimApplicationRotationAndDeepWindows(t *testing.T) {
	for _, test := range []struct{ apps, depth int }{{0, 0}, {1, 1000}, {16, 1}, {17, 300}, {40, 1}} {
		t.Run(fmt.Sprintf("apps-%d-depth-%d", test.apps, test.depth), func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			expected := make(map[int64]map[uint64]bool)
			for appIndex := range test.apps {
				app := repotest.NewApplicationBuilder().WithName(fmt.Sprintf("rotation-%d", appIndex)).Create(t.Context(), t, r)
				statuses := make([]model.EpochStatus, test.depth)
				for index := range statuses {
					statuses[index] = model.EpochStatus_ClaimComputed
				}
				epochs := seedClaimPageEpochs(t, r, app, statuses...)
				expected[app.ID] = make(map[uint64]bool, len(epochs))
				for _, epoch := range epochs {
					expected[app.ID][epoch.Index] = false
				}
			}
			cursor := repository.ClaimCursor{}
			// Every app receives one window per rotation. The oldest row is
			// retried each time, while at least 15 later positions advance.
			turns := max(test.apps, 1) * (1 + (test.depth+14)/15)
			var previous int64
			for turn := range turns {
				page, err := r.SelectClaimsToSubmitPage(t.Context(), cursor, 16)
				require.NoError(t, err)
				require.LessOrEqual(t, len(page.Work), 16)
				if test.apps == 0 {
					require.Empty(t, page.Work)
					continue
				}
				require.NotEmpty(t, page.Work)
				if test.apps > 1 {
					require.NotEqual(t, previous, page.ApplicationID, "turn %d revisited an app too early", turn)
				}
				previous = page.ApplicationID
				require.Zero(t, page.Work[0].Epoch.Index, "each app turn retries its own oldest")
				for _, work := range page.Work {
					expected[work.Application.ID][work.Epoch.Index] = true
				}
				consumeClaimPage(&cursor, page)
			}
			for appID, epochs := range expected {
				for index, visited := range epochs {
					require.True(t, visited, "app %d epoch %d was never inspected", appID, index)
				}
			}
		})
	}
}

func TestClaimPageSourcesAreIndependentAndContainCompleteProofs(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	seedClaimPageEpochs(t, r, app, model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted,
		model.EpochStatus_ClaimStaged, model.EpochStatus_ClaimComputed)
	for _, query := range []func(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error){
		r.SelectClaimsToSubmitPage, r.SelectClaimsToStagePage, r.SelectClaimsToAcceptPage,
	} {
		page, err := query(t.Context(), repository.ClaimCursor{}, 16)
		require.NoError(t, err)
		require.NotEmpty(t, page.Work)
		for _, work := range page.Work {
			stored, err := r.GetEpoch(t.Context(), app.Name, work.Epoch.Index)
			require.NoError(t, err)
			actual, err := work.Epoch.StateProof()
			require.NoError(t, err)
			expected, err := stored.StateProof()
			require.NoError(t, err)
			require.Equal(t, expected, actual)
			require.Equal(t, stored.ClaimTransactionHash, work.Epoch.ClaimTransactionHash)
			require.Equal(t, stored.StagedAtBlock, work.Epoch.StagedAtBlock)
		}
	}
}

func TestClaimPageInsertionDeletionAndDisabledApplications(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	first := repotest.NewApplicationBuilder().WithName("first").Create(t.Context(), t, r)
	second := repotest.NewApplicationBuilder().WithName("second").Create(t.Context(), t, r)
	seedClaimPageEpochs(t, r, first, model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimComputed)
	seedClaimPageEpochs(t, r, second, model.EpochStatus_ClaimComputed)
	cursor := repository.ClaimCursor{}
	page, err := r.SelectClaimsToSubmitPage(t.Context(), cursor, 16)
	require.NoError(t, err)
	require.Equal(t, first.ID, page.ApplicationID)
	consumeClaimPage(&cursor, page)
	third := repotest.NewApplicationBuilder().WithName("newcomer").Create(t.Context(), t, r)
	seedClaimPageEpochs(t, r, third, model.EpochStatus_ClaimComputed)
	require.NoError(t, r.UpdateApplicationEnabled(t.Context(), first.ID, false))
	page, err = r.SelectClaimsToSubmitPage(t.Context(), cursor, 16)
	require.NoError(t, err)
	require.Equal(t, second.ID, page.ApplicationID)
	consumeClaimPage(&cursor, page)
	// This selector fixture deletes a row directly; application-removal gates
	// have their own production-path fixtures.
	statement := table.Application.DELETE().WHERE(table.Application.ID.EQ(jet.Int64(second.ID)))
	query, args := statement.Sql()
	_, err = r.db.Exec(t.Context(), query, args...)
	require.NoError(t, err)
	page, err = r.SelectClaimsToSubmitPage(t.Context(), cursor, 16)
	require.NoError(t, err)
	require.Equal(t, third.ID, page.ApplicationID)
	consumeClaimPage(&cursor, page)
	page, err = r.SelectClaimsToSubmitPage(t.Context(), cursor, 16)
	require.NoError(t, err)
	require.Equal(t, third.ID, page.ApplicationID, "disabled and deleted apps do not occupy future turns")
}

func TestClaimPageRejectsInvalidBounds(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	for _, test := range []struct {
		after int64
		limit uint64
	}{{-1, 16}, {0, 0}, {0, 1}, {0, math.MaxUint64}} {
		_, err := r.SelectClaimsToSubmitPage(t.Context(), repository.ClaimCursor{AfterApplicationID: test.after}, test.limit)
		require.Error(t, err)
	}
}

func TestClaimPageUsesSparseApplicationLocalEpochIndices(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	seeded := make([]*model.Epoch, 0, 4)
	inputs := make(map[*model.Epoch][]*model.Input)
	for offset, index := range []uint64{2, 700, 90000, math.MaxUint64} {
		epoch := repotest.NewEpochBuilder(app.ID).WithIndex(index).WithStatus(model.EpochStatus_Closed).
			WithBlocks(uint64(offset*10), uint64(offset*10+9)).WithInputBounds(0, 0).Build()
		seeded = append(seeded, epoch)
		inputs[epoch] = nil
	}
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, inputs, 40))
	for _, epoch := range seeded {
		repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epoch, model.EpochStatus_ClaimComputed)
	}
	var cursor repository.ClaimCursor
	page, err := r.SelectClaimsToSubmitPage(t.Context(), cursor, 3)
	require.NoError(t, err)
	require.Equal(t, []uint64{2, 700, 90000}, []uint64{page.Work[0].Epoch.Index, page.Work[1].Epoch.Index, page.Work[2].Epoch.Index})
	consumeClaimPage(&cursor, page)
	page, err = r.SelectClaimsToSubmitPage(t.Context(), cursor, 3)
	require.NoError(t, err)
	require.Len(t, page.Work, 2)
	require.Equal(t, uint64(2), page.Work[0].Epoch.Index)
	require.Equal(t, uint64(math.MaxUint64), page.Work[1].Epoch.Index)
}

func TestClaimPagesSkipIdleApplicationsForEverySource(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	for index := range 40 {
		app := repotest.NewApplicationBuilder().WithName(fmt.Sprintf("idle-%d", index)).Create(t.Context(), t, r)
		seedClaimPageEpochs(t, r, app, model.EpochStatus_ClaimAccepted)
	}
	prt := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
	seedClaimPageEpochs(t, r, prt, model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted,
		model.EpochStatus_ClaimStaged)
	for _, test := range []struct {
		status model.EpochStatus
		query  func(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error)
	}{
		{model.EpochStatus_ClaimComputed, r.SelectClaimsToSubmitPage},
		{model.EpochStatus_ClaimSubmitted, r.SelectClaimsToStagePage},
		{model.EpochStatus_ClaimStaged, r.SelectClaimsToAcceptPage},
	} {
		t.Run(test.status.String(), func(t *testing.T) {
			otherStatus := model.EpochStatus_ClaimComputed
			if test.status == otherStatus {
				otherStatus = model.EpochStatus_ClaimStaged
			}
			other := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
			seedClaimPageEpochs(t, r, other, otherStatus)
			first := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_Authority).Create(t.Context(), t, r)
			second := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_Quorum).Create(t.Context(), t, r)
			seedClaimPageEpochs(t, r, first, test.status)
			seedClaimPageEpochs(t, r, second, test.status)
			cursor := repository.ClaimCursor{}
			for _, expected := range []int64{first.ID, second.ID, first.ID} {
				page, err := test.query(t.Context(), cursor, 16)
				require.NoError(t, err)
				require.Equal(t, expected, page.ApplicationID, "accepted history, another source, and PRT consume no turn")
				require.Len(t, page.Work, 1)
				consumeClaimPage(&cursor, page)
			}
			require.NoError(t, r.UpdateApplicationEnabled(t.Context(), first.ID, false))
			require.NoError(t, r.UpdateApplicationEnabled(t.Context(), second.ID, false))
			require.NoError(t, r.UpdateApplicationEnabled(t.Context(), other.ID, false))
			page, err := test.query(t.Context(), cursor, 16)
			require.NoError(t, err)
			require.Empty(t, page.Work)
		})
	}
}

func TestClaimPagesKeepUnhealthyApplicationsObservable(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	for _, status := range model.ApplicationStatusAllValues {
		t.Run(status.String(), func(t *testing.T) {
			app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
			seedClaimPageEpochs(t, r, app, model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted,
				model.EpochStatus_ClaimStaged)
			require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, status, new("selector observation fixture")))
			for _, query := range []func(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error){
				r.SelectClaimsToSubmitPage, r.SelectClaimsToStagePage, r.SelectClaimsToAcceptPage,
			} {
				page, err := query(t.Context(), repository.ClaimCursor{}, 16)
				require.NoError(t, err)
				require.Equal(t, app.ID, page.ApplicationID)
				require.Len(t, page.Work, 1)
				require.Equal(t, status, page.Work[0].Application.Status)
			}
			require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
		})
	}
}

func TestClaimPublisherPagesSkipStoppedApplicationsBeforePagination(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	statuses := []model.EpochStatus{
		model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged,
	}
	stopped := make([]*model.Application, 0, 41)
	for index := range 40 {
		app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
		seedClaimPageEpochs(t, r, app, statuses...)
		status := model.ApplicationStatusAllValues[1+index%(len(model.ApplicationStatusAllValues)-1)]
		require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, status, new("publisher selection fixture")))
		stopped = append(stopped, app)
	}
	foreclosed := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	seedClaimPageEpochs(t, r, foreclosed, statuses...)
	require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), foreclosed.ID, 100, repotest.UniqueHash(), 100))
	stopped = append(stopped, foreclosed)
	first := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	second := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_Quorum).Create(t.Context(), t, r)
	seedClaimPageEpochs(t, r, first, statuses...)
	seedClaimPageEpochs(t, r, second, statuses...)
	for _, query := range []func(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error){
		r.SelectClaimsToSubmitPage, r.SelectClaimsToStagePage, r.SelectClaimsToAcceptPage,
	} {
		cursor := repository.ClaimCursor{PublishableOnly: true}
		for _, expected := range []int64{first.ID, second.ID, first.ID} {
			page, err := query(t.Context(), cursor, 2)
			require.NoError(t, err)
			require.Equal(t, expected, page.ApplicationID, "stopped applications must consume no publisher turn")
			require.Len(t, page.Work, 1)
			consumeClaimPage(&cursor, page)
		}
		// The same source without the publisher flag must still observe every
		// stopped application, including the foreclosed one, in ID order.
		cursor = repository.ClaimCursor{}
		for _, app := range append(stopped, first, second) {
			page, err := query(t.Context(), cursor, 2)
			require.NoError(t, err)
			require.Equal(t, app.ID, page.ApplicationID)
			require.Len(t, page.Work, 1)
			consumeClaimPage(&cursor, page)
		}
	}
}

func TestClaimPublisherPageMembershipChangesAcrossWrap(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	first := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	second := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	for _, app := range []*model.Application{first, second} {
		seedClaimPageEpochs(t, r, app, model.EpochStatus_ClaimComputed)
	}
	cursor := repository.ClaimCursor{PublishableOnly: true}
	page, err := r.SelectClaimsToSubmitPage(t.Context(), cursor, 2)
	require.NoError(t, err)
	require.Equal(t, first.ID, page.ApplicationID)
	consumeClaimPage(&cursor, page)
	require.NoError(t, r.UpdateApplicationStatus(t.Context(), second.ID, model.ApplicationStatus_Failed, new("stopped")))
	third := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	seedClaimPageEpochs(t, r, third, model.EpochStatus_ClaimComputed)
	require.NoError(t, r.UpdateApplicationEnabled(t.Context(), first.ID, false))
	page, err = r.SelectClaimsToSubmitPage(t.Context(), cursor, 2)
	require.NoError(t, err)
	require.Equal(t, third.ID, page.ApplicationID, "an inserted app is reached past the newly stopped app")
	consumeClaimPage(&cursor, page)
	require.NoError(t, r.UpdateApplicationStatus(t.Context(), second.ID, model.ApplicationStatus_OK, nil))
	page, err = r.SelectClaimsToSubmitPage(t.Context(), cursor, 2)
	require.NoError(t, err)
	require.Equal(t, second.ID, page.ApplicationID, "a repaired app rejoins after wrapping; disabled apps stay excluded")
}

func TestClaimPublisherPagesSelectOnlyTerminalPrefix(t *testing.T) {
	for _, kind := range []model.ApplicationStatus{model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		for _, status := range []model.EpochStatus{model.EpochStatus_ClaimComputed,
			model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged} {
			t.Run(kind.String()+"/"+status.String(), func(t *testing.T) {
				r := newEpochPublicationRepository(t).(*PostgresRepository)
				app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
				seedClaimPageEpochs(t, r, app, status, status, status, status, status)
				// Source production/rollback has separate real-writer tests.
				// Keep suffix rows here to check both SQL pagination gates.
				var input *uint64
				if kind.IsExecutionTerminal() {
					input = new(uint64(99))
				}
				_, err := r.db.Exec(t.Context(), `UPDATE application SET status=$2, reason='prefix pagination fixture',
                    problem_epoch_index=2,
                    problem_kind=$2, problem_input_index=$3 WHERE id=$1`, app.ID, kind, input)
				require.NoError(t, err)
				query := r.SelectClaimsToSubmitPage
				if status == model.EpochStatus_ClaimSubmitted {
					query = r.SelectClaimsToStagePage
				}
				if status == model.EpochStatus_ClaimStaged {
					query = r.SelectClaimsToAcceptPage
				}
				cursor := repository.ClaimCursor{PublishableOnly: true}
				for range 3 {
					page, err := query(t.Context(), cursor, 2)
					require.NoError(t, err)
					require.Equal(t, app.ID, page.ApplicationID)
					require.Len(t, page.Work, 2)
					require.Equal(t, uint64(0), page.Work[0].Epoch.Index)
					require.Equal(t, uint64(1), page.Work[1].Epoch.Index)
					consumeClaimPage(&cursor, page)
				}
				page, err := query(t.Context(), repository.ClaimCursor{}, 16)
				require.NoError(t, err)
				require.Len(t, page.Work, 5, "observation keeps every stored claim")
				_, err = r.db.Exec(t.Context(), `UPDATE application SET problem_epoch_index=0 WHERE id=$1`, app.ID)
				require.NoError(t, err)
				page, err = query(t.Context(), repository.ClaimCursor{PublishableOnly: true}, 16)
				require.NoError(t, err)
				require.Empty(t, page.Work, "an application with no eligible prefix consumes no turn")
				require.Zero(t, page.ApplicationID)
			})
		}
	}
}

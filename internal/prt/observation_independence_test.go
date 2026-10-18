// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type slowObservationRepository struct {
	*deliveryRepoFake
	delay time.Duration
}

func (r *slowObservationRepository) ListEpochs(
	ctx context.Context, _ string, _ repository.EpochFilter, _ repository.Pagination, _ bool,
) ([]*model.Epoch, uint64, error) {
	timer := time.NewTimer(r.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case <-timer.C:
		return nil, 0, nil
	}
}

func TestObservationHasNoPublicationTimeShare(t *testing.T) {
	f, client := tickFixture(t)
	f.app.Status = model.ApplicationStatus_Failed
	f.repo.publishedRoots = nil
	f.repo.storedTournaments = nil
	f.d.repository = &slowObservationRepository{deliveryRepoFake: f.repo, delay: 40 * time.Millisecond}
	tickHead(client, 110)
	_, err := f.d.Tick(t.Context())
	require.NoError(t, err)
	require.Empty(t, f.backend.headerReads)
	require.Empty(t, f.repo.windows, "an empty observation must not invent tournament coverage")
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
}

func TestObservationStillHonorsServiceCancellation(t *testing.T) {
	f, client := tickFixture(t)
	f.app.Status = model.ApplicationStatus_Failed
	f.repo.publishedRoots = nil
	f.repo.storedTournaments = nil
	f.d.repository = &slowObservationRepository{deliveryRepoFake: f.repo, delay: time.Second}
	tickHead(client, 110)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err := f.d.Tick(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, f.repo.windows)
	require.Empty(t, f.backend.published)
}

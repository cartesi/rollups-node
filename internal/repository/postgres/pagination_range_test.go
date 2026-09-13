// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"math"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

func TestApplicationAndEpochPaginationIntegerBounds(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	epoch := repotest.NewEpochBuilder(app.ID).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*Epoch][]*Input{epoch: {}}, epoch.LastBlock))
	for _, method := range []struct {
		name string
		list func(repository.Pagination) (int, uint64, error)
	}{
		{"applications", func(p repository.Pagination) (int, uint64, error) {
			rows, total, err := r.ListApplications(t.Context(), repository.ApplicationFilter{}, p, false)
			return len(rows), total, err
		}},
		{"epochs", func(p repository.Pagination) (int, uint64, error) {
			rows, total, err := r.ListEpochs(t.Context(), app.Name, repository.EpochFilter{}, p, false)
			return len(rows), total, err
		}},
	} {
		t.Run(method.name, func(t *testing.T) {
			for _, p := range []repository.Pagination{{Limit: math.MaxInt64}, {Offset: math.MaxInt64}} {
				count, total, err := method.list(p)
				require.NoError(t, err)
				require.Equal(t, uint64(1), total)
				if p.Offset == 0 {
					require.Equal(t, 1, count)
				} else {
					require.Zero(t, count)
				}
			}
			for _, p := range []repository.Pagination{{Limit: math.MaxInt64 + 1}, {Offset: math.MaxInt64 + 1}} {
				count, total, err := method.list(p)
				require.ErrorContains(t, err, "pagination exceeds PostgreSQL integer range")
				require.Zero(t, count)
				require.Zero(t, total)
			}
		})
	}
}

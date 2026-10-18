// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/repository"
)

// StoreTournamentEvents installs PostgreSQL package fixtures through a writer
// compiled exclusively into that package's tests. Production exposes only the
// checked StoreTournamentInterval method.
func StoreTournamentEvents(
	ctx context.Context, repo repository.Repository, appID int64, batches []*repository.TournamentEventBatch, lastBlock uint64,
) error {
	writer, ok := repo.(interface {
		StoreTournamentEvents(context.Context, int64, []*repository.TournamentEventBatch, uint64) error
	})
	if !ok {
		return errors.New("repository does not support tournament event fixtures")
	}
	return writer.StoreTournamentEvents(ctx, appID, batches, lastBlock)
}

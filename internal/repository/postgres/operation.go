// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"time"
)

// Bound each delivery database operation, including pool acquisition and its
// transaction. A shorter caller deadline or service cancellation still wins.
// Observation passes and blockchain calls have independent lifetimes.
const databaseOperationTimeout = 2 * time.Minute

func beginDatabaseOperation(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, databaseOperationTimeout)
}

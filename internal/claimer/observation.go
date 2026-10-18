// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
)

// A canonical observation can predate facts saved by another service or an
// earlier process. Absence in that view cannot contradict the newer facts.
func requireObservedBlock(observed, required uint64) error {
	if observed < required {
		return fmt.Errorf("%w: observation block %d is below required block %d", txsender.ErrInconclusive, observed, required)
	}
	return nil
}

func requireObservedStaging(epoch *model.Epoch, observed uint64) error {
	if epoch != nil && epoch.StagedAtBlock != nil {
		return requireObservedBlock(observed, *epoch.StagedAtBlock)
	}
	return nil
}

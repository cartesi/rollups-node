// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import "github.com/cartesi/rollups-node/internal/model"

type computedClaimWork struct {
	app       *model.Application
	prevEpoch *model.Epoch
	epoch     *model.Epoch
}

type submittedClaimWork struct {
	app       *model.Application
	prevEpoch *model.Epoch
	epoch     *model.Epoch
}

type stagedClaimWork struct {
	app       *model.Application
	prevEpoch *model.Epoch
	epoch     *model.Epoch
}

type submitInFlightWork struct {
	app   *model.Application
	epoch *model.Epoch
}

type acceptInFlightWork struct {
	app   *model.Application
	epoch *model.Epoch
}

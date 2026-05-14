// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import . "github.com/cartesi/rollups-node/internal/model"

type blockScanPlan struct {
	iConsensusInputTargets []appContracts
	daveEpochTargets       []appContracts
	outputTargets          []appContracts
	postForeclosureTargets []appContracts
}

func buildBlockScanPlan(apps []appContracts) blockScanPlan {
	var plan blockScanPlan
	for _, app := range apps {
		application := app.application
		if application == nil {
			continue
		}

		if application.IsForeclosed() {
			plan.outputTargets = append(plan.outputTargets, app)
			plan.postForeclosureTargets = append(plan.postForeclosureTargets, app)

			if application.IsDaveConsensus() {
				if application.LastEpochCheckBlock < application.ForecloseBlock {
					plan.daveEpochTargets = append(plan.daveEpochTargets, app)
				}
				continue
			}

			if application.LastInputCheckBlock < application.ForecloseBlock &&
				application.HasDataAvailabilitySelector(DataAvailability_InputBox) {
				plan.iConsensusInputTargets = append(plan.iConsensusInputTargets, app)
			}
			continue
		}

		if application.CanExecute() {
			plan.outputTargets = append(plan.outputTargets, app)
			if application.IsDaveConsensus() {
				plan.daveEpochTargets = append(plan.daveEpochTargets, app)
			} else {
				plan.iConsensusInputTargets = append(plan.iConsensusInputTargets, app)
			}
		}
	}
	return plan
}

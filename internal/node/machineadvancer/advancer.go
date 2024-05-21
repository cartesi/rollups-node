// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machineadvancer

import . "github.com/cartesi/rollups-node/internal/node/model"

type Machine interface {
	Advance([]byte) ([]MachineOutput, []MachineReport, MachineOutputsHash, MachineHash, error)
}

func GetInputs() []MachineInput {
	return []MachineInput{}
}

func Store(outputs []MachineOutput,
	reports []MachineReport,
	outputsHash MachineOutputsHash,
	machineHash MachineHash) error {

	return nil
}

func StartAdvanceServer(machine Machine) {
	for {
		for _, input := range GetInputs() {
			outputs, reports, outputsHash, machineHash, err := machine.Advance(input)
			if err != nil {
				panic("TODO")
			}

			err = Store(outputs, reports, outputsHash, machineHash)
			if err != nil {
				panic("TODO")
			}
		}
		// TODO: wait
	}
}

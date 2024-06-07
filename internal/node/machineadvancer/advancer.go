// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machineadvancer

import "github.com/cartesi/rollups-node/internal/node/nodemachine"

type Input = []byte
type Output = []byte
type Report = []byte
type Hash = [32]byte

func GetInputs() []Input {
	return []Input{}
}

func Store(outputs []Output, reports []Report, outputsHash Hash, machineHash Hash) error {
	return nil
}

func StartAdvanceServer(machine *nodemachine.NodeMachine) {
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
	}
}

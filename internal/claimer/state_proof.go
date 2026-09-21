// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
)

func consensusMachineValidityProof(proof model.StateProof) iconsensus.MachineValidityProof {
	return iconsensus.MachineValidityProof{
		IflagsYProof: iconsensus.LeafProof{
			DataBlock: proof.IflagsYDataBlock,
			Siblings:  append([][32]byte(nil), proof.IflagsYProof...),
		},
		HtifTohostProof: iconsensus.LeafProof{
			DataBlock: proof.HtifTohostDataBlock,
			Siblings:  append([][32]byte(nil), proof.HtifTohostProof...),
		},
		TxBufferProof: iconsensus.LeafProof{
			DataBlock: proof.TxBufferDataBlock,
			Siblings:  append([][32]byte(nil), proof.TxBufferProof...),
		},
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestStageTournamentResultEncodesModelProof(t *testing.T) {
	proof := repotest.KeccakStateProof(common.HexToHash("0xab"))
	contractABI, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	require.NoError(t, err)
	want, err := contractABI.Pack("stageTournamentResult", big.NewInt(3), idaveconsensus.MachineValidityProof{
		IflagsYProof:    idaveconsensus.LeafProof{DataBlock: proof.IflagsYDataBlock, Siblings: proof.IflagsYProof},
		HtifTohostProof: idaveconsensus.LeafProof{DataBlock: proof.HtifTohostDataBlock, Siblings: proof.HtifTohostProof},
		TxBufferProof:   idaveconsensus.LeafProof{DataBlock: proof.TxBufferDataBlock, Siblings: proof.TxBufferProof},
	})
	require.NoError(t, err)
	packed, err := contractABI.Pack("stageTournamentResult", big.NewInt(3), daveMachineValidityProof(proof))
	require.NoError(t, err)
	require.Equal(t, want, packed)
	// Encoding does not alias mutable database proof slices.
	adapted := daveMachineValidityProof(proof)
	require.NotEmpty(t, adapted.IflagsYProof.Siblings)
	original := adapted.IflagsYProof.Siblings[0]
	proof.IflagsYProof[0] = [32]byte{}
	require.Equal(t, original, adapted.IflagsYProof.Siblings[0])
}

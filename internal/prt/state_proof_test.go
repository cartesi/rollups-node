// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestStageTournamentResultEncodesModelProof(t *testing.T) {
	proof := repotest.KeccakStateProof(common.HexToHash("0xab"))
	contract, err := idaveconsensus.NewIDaveConsensus(common.HexToAddress("0x1234"), nil)
	require.NoError(t, err)
	adapter := &DaveConsensusAdapterImpl{consensus: contract}
	opts := &bind.TransactOpts{
		Context: t.Context(), Nonce: big.NewInt(0), GasPrice: big.NewInt(1), GasLimit: 1_000_000,
		NoSend: true,
		Signer: func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) { return tx, nil },
	}
	tx, err := adapter.StageTournamentResult(opts, 3, proof)
	require.NoError(t, err)
	contractABI, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	require.NoError(t, err)
	want, err := contractABI.Pack("stageTournamentResult", big.NewInt(3), idaveconsensus.MachineValidityProof{
		IflagsYProof:    idaveconsensus.LeafProof{DataBlock: proof.IflagsYDataBlock, Siblings: proof.IflagsYProof},
		HtifTohostProof: idaveconsensus.LeafProof{DataBlock: proof.HtifTohostDataBlock, Siblings: proof.HtifTohostProof},
		TxBufferProof:   idaveconsensus.LeafProof{DataBlock: proof.TxBufferDataBlock, Siblings: proof.TxBufferProof},
	})
	require.NoError(t, err)
	require.Equal(t, want, tx.Data())
}

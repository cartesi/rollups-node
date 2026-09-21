// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"io"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestConsensusMachineValidityProofUsesContractFieldOrder(t *testing.T) {
	canonical := repotest.KeccakStateProof(common.HexToHash("0xa1"))

	wire := consensusMachineValidityProof(canonical)
	require.Equal(t, [32]byte(canonical.IflagsYDataBlock), wire.IflagsYProof.DataBlock)
	require.Equal(t, canonical.IflagsYProof, wire.IflagsYProof.Siblings)
	require.Equal(t, [32]byte(canonical.HtifTohostDataBlock), wire.HtifTohostProof.DataBlock)
	require.Equal(t, canonical.HtifTohostProof, wire.HtifTohostProof.Siblings)
	require.Equal(t, [32]byte(canonical.TxBufferDataBlock), wire.TxBufferProof.DataBlock)
	require.Equal(t, canonical.TxBufferProof, wire.TxBufferProof.Siblings)

	wire.IflagsYProof.Siblings[0][0] ^= 0xff
	require.NotEqual(t, canonical.IflagsYProof[0], wire.IflagsYProof.Siblings[0],
		"the contract proof must own its sibling storage")
}

func TestSubmitClaimUsesMachineRootAndOutputsRoot(t *testing.T) {
	app := makeApplication()
	epoch := makeComputedEpoch(app, 3)
	proof, err := epoch.StateProof()
	require.NoError(t, err)
	capture := &claimSubmitterCapture{}
	blockchain := &claimerBlockchain{
		txOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{}),
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	txHash, err := blockchain.submitClaimToBlockchain(t.Context(), capture, app, epoch, proof)
	require.NoError(t, err)
	require.Equal(t, capture.tx.Hash(), txHash)
	require.Equal(t, app.IApplicationAddress, capture.appContract)
	require.Equal(t, new(big.Int).SetUint64(epoch.LastBlock), capture.lastProcessedBlockNumber)
	require.Equal(t, [32]byte(proof.MachineHash), capture.machineMerkleRoot)
	require.Equal(t, [32]byte(proof.IflagsYDataBlock), capture.proof.IflagsYProof.DataBlock)
	require.Equal(t, proof.IflagsYProof, capture.proof.IflagsYProof.Siblings)
	require.Equal(t, [32]byte(proof.HtifTohostDataBlock), capture.proof.HtifTohostProof.DataBlock)
	require.Equal(t, proof.HtifTohostProof, capture.proof.HtifTohostProof.Siblings)
	require.Equal(t, [32]byte(proof.TxBufferDataBlock), capture.proof.TxBufferProof.DataBlock)
	require.Equal(t, proof.TxBufferProof, capture.proof.TxBufferProof.Siblings)
}

type claimSubmitterCapture struct {
	tx                       *types.Transaction
	appContract              common.Address
	lastProcessedBlockNumber *big.Int
	machineMerkleRoot        [32]byte
	proof                    iconsensus.MachineValidityProof
}

func (c *claimSubmitterCapture) SubmitClaim(
	_ *bind.TransactOpts,
	appContract common.Address,
	lastProcessedBlockNumber *big.Int,
	machineMerkleRoot [32]byte,
	proof iconsensus.MachineValidityProof,
) (*types.Transaction, error) {
	c.tx = types.NewTx(&types.LegacyTx{Nonce: 7})
	c.appContract = appContract
	c.lastProcessedBlockNumber = new(big.Int).Set(lastProcessedBlockNumber)
	c.machineMerkleRoot = machineMerkleRoot
	c.proof = proof
	return c.tx, nil
}

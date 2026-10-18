// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
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

func TestClaimCallUsesMachineRootAndCompleteProof(t *testing.T) {
	app := makeApplication()
	epoch := makeComputedEpoch(app, 3)
	proof, err := epoch.StateProof()
	require.NoError(t, err)
	call, err := claimCall(app, epoch, proof, true)
	require.NoError(t, err)
	parsed, err := iconsensus.IConsensusMetaData.GetAbi()
	require.NoError(t, err)
	values, err := parsed.Methods[claimSubmitMethod].Inputs.Unpack(call.request.Data[4:])
	require.NoError(t, err)
	require.Equal(t, app.IApplicationAddress, values[0])
	require.Equal(t, new(big.Int).SetUint64(epoch.LastBlock), values[1])
	require.Equal(t, [32]byte(proof.MachineHash), values[2])
	wire := *abi.ConvertType(values[3], new(iconsensus.MachineValidityProof)).(*iconsensus.MachineValidityProof)
	require.Equal(t, consensusMachineValidityProof(proof), wire)
	require.Equal(t, app.IConsensusAddress, call.request.To)
	require.Equal(t, submitClaimKind, call.action.Kind)
	changed := *epoch
	root := common.HexToHash("0x1234")
	changed.MachineHash = &root
	changedProof := proof
	changedProof.MachineHash = root
	other, err := claimCall(app, &changed, changedProof, true)
	require.NoError(t, err)
	require.NotEqual(t, call.action, other.action, "changed protocol content must have a new identity")
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func creationFixture(t *testing.T, address common.Address, descriptor TournamentDescriptor) *TournamentCreation {
	t.Helper()
	bond, err := model.Uint256FromBig(big.NewInt(1))
	require.NoError(t, err)
	return &TournamentCreation{Address: address, Descriptor: descriptor, BondValue: bond,
		Event: model.TournamentCreationEvent{BlockNumber: descriptor.StartInstant, TxHash: common.HexToHash("0xabc"), LogIndex: 2}}
}

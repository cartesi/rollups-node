// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestSubmitterAddress(t *testing.T) {
	address := common.HexToAddress("0x1234")
	tests := []struct {
		name        string
		factory     ethutil.TransactOptsFactory
		wantAddress common.Address
		wantEnabled bool
	}{
		{name: "disabled"},
		{
			name:        "enabled",
			factory:     ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: address}),
			wantAddress: address,
			wantEnabled: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Service{txOptsFactory: test.factory}
			gotAddress, gotEnabled := s.SubmitterAddress()
			require.Equal(t, test.wantAddress, gotAddress)
			require.Equal(t, test.wantEnabled, gotEnabled)
		})
	}
}

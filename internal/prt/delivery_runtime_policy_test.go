// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestDeliveryPolicyHasOneOptionalPriceCeiling(t *testing.T) {
	cfg := prtStartupConfig(true)
	cfg.BlockchainTxMaxFeePerGas = ""
	policy, err := deliveryPolicy(cfg)
	require.NoError(t, err)
	require.Nil(t, policy.MaxFee)
	cfg.BlockchainTxMaxFeePerGas = "123456789"
	cfg.BlockchainLegacyEnabled = true
	policy, err = deliveryPolicy(cfg)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(123456789), policy.MaxFee)
	require.True(t, policy.Legacy)
}

func TestDeliveryPolicyRejectsInvalidPriceCeiling(t *testing.T) {
	for _, value := range []string{"0", "-1", "1.5", "1e9", " 10"} {
		t.Run(value, func(t *testing.T) {
			cfg := prtStartupConfig(true)
			cfg.BlockchainTxMaxFeePerGas = value
			_, err := deliveryPolicy(cfg)
			require.ErrorContains(t, err, "MAX_FEE_PER_GAS")
		})
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package auth

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestNodeSubmittersResolveDistinctAddressesBeforeStartup(t *testing.T) {
	viper.Reset()
	config.SetDefaults()
	t.Cleanup(func() { viper.Reset(); config.SetDefaults() })
	viper.Set(config.AUTH_KIND, "mnemonic")
	viper.Set(config.AUTH_MNEMONIC, ethutil.FoundryMnemonic)
	viper.Set(config.AUTH_MNEMONIC_ACCOUNT_INDEX, 0)
	viper.Set(config.PRT_AUTH_KIND, "mnemonic")
	viper.Set(config.PRT_AUTH_MNEMONIC, ethutil.FoundryMnemonic)
	viper.Set(config.PRT_AUTH_MNEMONIC_ACCOUNT_INDEX, 6)
	claimer, prt, err := GetNodeTransactOptsFactories(t.Context(), big.NewInt(31337))
	require.NoError(t, err)
	require.NotEqual(t, claimer.From(), prt.From())
	viper.Set(config.PRT_AUTH_MNEMONIC_ACCOUNT_INDEX, 0)
	_, _, err = GetNodeTransactOptsFactories(t.Context(), big.NewInt(31337))
	require.ErrorContains(t, err, "claimer and prt require distinct submitting addresses")
	require.ErrorContains(t, err, claimer.From().Hex())
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

const testPrivateKey = "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

func TestGetTransactOptsBlockchainGasLimit(t *testing.T) {
	setupPrivateKeyAuth := func(t *testing.T) {
		t.Helper()
		viper.Reset()
		config.SetDefaults()
		viper.Set(config.AUTH_KIND, "private_key")
		viper.Set(config.AUTH_PRIVATE_KEY, testPrivateKey)
	}

	t.Run("default zero leaves gas limit unset", func(t *testing.T) {
		setupPrivateKeyAuth(t)

		txOpts, err := GetTransactOpts(context.Background(), big.NewInt(31337))
		require.NoError(t, err)
		require.Zero(t, txOpts.GasLimit)
	})

	t.Run("non-zero config sets gas limit", func(t *testing.T) {
		setupPrivateKeyAuth(t)
		viper.Set(config.BLOCKCHAIN_GAS_LIMIT, "123456")

		txOpts, err := GetTransactOpts(context.Background(), big.NewInt(31337))
		require.NoError(t, err)
		require.Equal(t, uint64(123456), txOpts.GasLimit)
	})
}

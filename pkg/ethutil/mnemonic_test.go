// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func testKey(t *testing.T, mnemonic string, account uint32, expected string) {
	expectedKey, err := crypto.HexToECDSA(expected)
	require.Nil(t, err)

	key, err := mnemonicToPrivateKey(mnemonic, account)
	require.Nil(t, err)
	require.Equal(t, expectedKey, key)
}

func TestMnemonicToPrivateKey(t *testing.T) {
	mnemonic := "test test test test test test test test test test test junk"
	testKey(t, mnemonic, 0, "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	testKey(t, mnemonic, 1, "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
}

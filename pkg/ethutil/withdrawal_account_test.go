// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestFormatTokenAmount(t *testing.T) {
	cases := []struct {
		raw      string
		decimals uint8
		want     string
	}{
		{"0", 0, "0"},
		{"42", 0, "42"},
		{"1500000", 6, "1.5"},
		{"1234567", 6, "1.234567"},
		{"1000000", 6, "1"},
		{"1", 6, "0.000001"},
		{"1000000000000000000", 18, "1"},
		{"1500000000000000000", 18, "1.5"},
		{"999999999", 8, "9.99999999"},
		{"1", 18, "0.000000000000000001"},
		{"79228162514264337593543950335", 6, "79228162514264337593543.950335"},
	}
	for _, c := range cases {
		raw, ok := new(big.Int).SetString(c.raw, 10)
		require.True(t, ok)
		before := new(big.Int).Set(raw)
		got := formatTokenAmount(raw, c.decimals)
		require.Equalf(t, c.want, got, "formatTokenAmount(%s, %d)", c.raw, c.decimals)
		require.Zero(t, before.Cmp(raw), "formatTokenAmount mutated its input")
	}
}

func TestDecodeUSDAccountExactUint96Record(t *testing.T) {
	recipient := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	account := common.FromHex("0x0102030405060708090a0b0cbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	before := bytes.Clone(account)

	gotRecipient, gotBalance, err := decodeUSDAccount(account)
	require.NoError(t, err)
	require.Equal(t, recipient, gotRecipient)
	require.Equal(t, "3727165692135864801209549313", gotBalance.String())
	require.Equal(t, before, account, "decodeUSDAccount mutated its source bytes")
}

func TestDecodeUSDAccountRequiresExactSize(t *testing.T) {
	for _, size := range []int{0, usdAccountSize - 1, usdAccountSize + 1} {
		_, _, err := decodeUSDAccount(make([]byte, size))
		require.ErrorContains(t, err, "exactly 32 bytes")
	}
}

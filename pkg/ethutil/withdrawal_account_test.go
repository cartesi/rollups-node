// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestFormatTokenAmount(t *testing.T) {
	cases := []struct {
		raw      uint64
		decimals uint8
		want     string
	}{
		{0, 0, "0"},
		{42, 0, "42"},
		{1_500_000, 6, "1.5"},
		{1_234_567, 6, "1.234567"},
		{1_000_000, 6, "1"},
		{1, 6, "0.000001"},
		{1_000_000_000_000_000_000, 18, "1"},
		{1_500_000_000_000_000_000, 18, "1.5"},
		{999_999_999, 8, "9.99999999"},
		{1, 18, "0.000000000000000001"},
	}
	for _, c := range cases {
		got := formatTokenAmount(c.raw, c.decimals)
		require.Equalf(t, c.want, got, "formatTokenAmount(%d, %d)", c.raw, c.decimals)
	}
}

func TestDecodeUSDAccount_AcceptsMinimumAndPaddedAccount(t *testing.T) {
	recipient := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	account := make([]byte, 32)
	binary.LittleEndian.PutUint64(account[:8], 75)
	copy(account[8:28], recipient.Bytes())

	gotRecipient, gotBalance := decodeUSDAccount(account[:28])
	require.Equal(t, recipient, gotRecipient)
	require.Equal(t, uint64(75), gotBalance)

	gotRecipient, gotBalance = decodeUSDAccount(account)
	require.Equal(t, recipient, gotRecipient)
	require.Equal(t, uint64(75), gotBalance)
}

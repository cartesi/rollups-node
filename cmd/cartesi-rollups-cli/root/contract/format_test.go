// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatAddr(t *testing.T) {
	tests := []struct {
		name string
		addr common.Address
		want string
	}{
		{
			name: "zero address",
			addr: common.Address{},
			want: "0x0000000000000000000000000000000000000000",
		},
		{
			name: "normal address uses EIP-55 checksum",
			addr: common.HexToAddress("0xDEADBEEF11112222333344445555666677778888"),
			want: "0xdeadbeEF11112222333344445555666677778888",
		},
		{
			name: "lowercase input normalizes to checksum",
			addr: common.HexToAddress("0xdeadbeef11112222333344445555666677778888"),
			want: "0xdeadbeEF11112222333344445555666677778888",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAddr(tt.addr)
			assert.Equal(t, tt.want, got)
			// Always 42 characters: 0x + 40 hex chars
			assert.Len(t, got, 42)
		})
	}
}

func TestFormatHash(t *testing.T) {
	tests := []struct {
		name string
		hash [32]byte
		want string
	}{
		{
			name: "zero hash",
			hash: [32]byte{},
			want: "0x0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name: "normal hash",
			hash: common.HexToHash(
				"0xABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"),
			want: "0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatHash(tt.hash)
			// Always 66 characters: 0x + 64 hex chars
			assert.Len(t, got, 66)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWeiToETH(t *testing.T) {
	tests := []struct {
		name string
		wei  *big.Int
		want string
	}{
		{
			name: "zero",
			wei:  big.NewInt(0),
			want: "0.000000 ETH",
		},
		{
			name: "one wei",
			wei:  big.NewInt(1),
			want: "0.000000 ETH", // too small to show at 6 decimal places
		},
		{
			name: "one ETH",
			wei:  new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil),
			want: "1.000000 ETH",
		},
		{
			name: "0.05 ETH",
			wei:  new(big.Int).Mul(big.NewInt(5), new(big.Int).Exp(big.NewInt(10), big.NewInt(16), nil)),
			want: "0.050000 ETH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weiToETH(tt.wei)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTournamentStatus(t *testing.T) {
	assert.Equal(t, "OPEN", tournamentStatus(false, false))
	assert.Equal(t, "CLOSED (matches still running)", tournamentStatus(true, false))
	assert.Equal(t, "FINISHED", tournamentStatus(false, true))
	assert.Equal(t, "FINISHED", tournamentStatus(true, true))
}

func TestMatchDeletionReason(t *testing.T) {
	assert.Equal(t, "STEP (on-chain proof)", matchDeletionReason(0))
	assert.Equal(t, "TIMEOUT", matchDeletionReason(1))
	assert.Equal(t, "CHILD_TOURNAMENT", matchDeletionReason(2))
	assert.Equal(t, "UNKNOWN(42)", matchDeletionReason(42))
}

func TestMatchWinner(t *testing.T) {
	assert.Equal(t, "NONE (both eliminated)", matchWinner(0))
	assert.Equal(t, "ONE", matchWinner(1))
	assert.Equal(t, "TWO", matchWinner(2))
	assert.Equal(t, "UNKNOWN(99)", matchWinner(99))
}

func TestFormatBlockTime(t *testing.T) {
	assert.Equal(t, "", formatBlockTime(0))
	// Unix timestamp 1700000000 = 2023-11-14 22:13:20 UTC
	got := formatBlockTime(1700000000)
	assert.Contains(t, got, "2023-11-14")
	assert.Contains(t, got, "UTC")
	assert.True(t, got[0] == ' ' && got[1] == '(')
}

func TestPrinterFooter(t *testing.T) {
	var buf bytes.Buffer
	p := &printer{w: &buf}

	p.footer(12345, 11155111)
	out := buf.String()
	assert.Contains(t, out, "12345")
	assert.Contains(t, out, "11155111")
	assert.Contains(t, out, "experimental diagnostic tool")
}

func TestPrinterFooterWithTimestamp(t *testing.T) {
	var buf bytes.Buffer
	p := &printer{w: &buf}

	p.footer(12345, 1, 1700000000)
	out := buf.String()
	assert.Contains(t, out, "12345")
	assert.Contains(t, out, "2023-11-14")
	assert.Contains(t, out, "UTC")
	assert.Contains(t, out, "experimental diagnostic tool")
}

func TestPrinterField(t *testing.T) {
	var buf bytes.Buffer
	p := &printer{w: &buf}

	p.field("Name", "Value")
	out := buf.String()
	assert.Contains(t, out, "Name")
	assert.Contains(t, out, "Value")
}

func TestPrinterWithSection(t *testing.T) {
	var buf bytes.Buffer
	p := &printer{w: &buf}

	p.withSection("Section Title", func() {
		p.field("Key", "Val")
	})

	out := buf.String()
	assert.Contains(t, out, "Section Title")
	assert.Contains(t, out, "Key")
	// After withSection, indent should be back to 0
	assert.Equal(t, 0, p.indent)
}

func TestPrinterNestedSections(t *testing.T) {
	var buf bytes.Buffer
	p := &printer{w: &buf}

	p.withSection("Outer", func() {
		require.Equal(t, 1, p.indent)
		p.withSection("Inner", func() {
			require.Equal(t, 2, p.indent)
			p.field("Deep", "Value")
		})
		require.Equal(t, 1, p.indent)
	})
	require.Equal(t, 0, p.indent)
}

func TestPrinterFieldErr(t *testing.T) {
	var buf bytes.Buffer
	p := &printer{w: &buf}

	p.fieldErr("test label", assert.AnError)
	out := buf.String()
	assert.Contains(t, out, "ERROR")
	assert.Contains(t, out, "test label")
	assert.Contains(t, out, assert.AnError.Error())
}

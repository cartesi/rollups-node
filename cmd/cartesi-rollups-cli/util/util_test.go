// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package util

import (
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// The 0x-bypass invariant is the whole point of allowing remote / reader-only
// hosts to use the foreclose / prove-drive-root / withdraw subcommands
// against an application that is NOT registered in any local repository.
// If a future change reorders the function so the database lookup happens
// before the prefix check, every one of those CLIs silently starts requiring
// CARTESI_DATABASE_CONNECTION. These tests pin the invariant by setting the
// DB env to something deliberately broken — a real DB lookup against this
// value would fail loudly, so a passing test means the 0x branch ran first.

func TestResolveApplicationAddress_HexBypassesDB(t *testing.T) {
	t.Setenv("CARTESI_DATABASE_CONNECTION", "postgres://nobody@nowhere:1/nodb")
	t.Setenv("CARTESI_DATABASE_CONNECTION_FILE", "")

	addr := "0x1111111111111111111111111111111111111111"
	got, err := ResolveApplicationAddress(context.Background(), addr)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(addr), got)
}

func TestResolveApplicationAddress_UppercaseHexPrefixAlsoBypasses(t *testing.T) {
	t.Setenv("CARTESI_DATABASE_CONNECTION", "postgres://nobody@nowhere:1/nodb")
	t.Setenv("CARTESI_DATABASE_CONNECTION_FILE", "")

	addr := "0X2222222222222222222222222222222222222222"
	got, err := ResolveApplicationAddress(context.Background(), addr)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(addr), got)
}

func TestResolveApplicationAddress_InvalidHex(t *testing.T) {
	t.Setenv("CARTESI_DATABASE_CONNECTION", "postgres://nobody@nowhere:1/nodb")
	t.Setenv("CARTESI_DATABASE_CONNECTION_FILE", "")

	cases := []string{
		"0xnothex",
		"0x123", // too short
		"0x11111111111111111111111111111111111111111111", // too long
		"0xZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",     // non-hex chars
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := ResolveApplicationAddress(context.Background(), in)
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid Ethereum address")
		})
	}
}

// When the caller passes a name and CARTESI_DATABASE_CONNECTION is not set,
// the error message must point the user at the 0x-bypass alternative — the
// CLI's documented escape hatch for remote / reader-only operation.
func TestResolveApplicationAddress_NameWithoutDBPointsAtBypass(t *testing.T) {
	t.Setenv("CARTESI_DATABASE_CONNECTION", "")
	t.Setenv("CARTESI_DATABASE_CONNECTION_FILE", "")

	_, err := ResolveApplicationAddress(context.Background(), "some-app-name")
	require.Error(t, err)
	msg := err.Error()
	require.True(t,
		strings.Contains(msg, "0x") && strings.Contains(msg, "address"),
		"name-without-DB error must point at the 0x-bypass: got %q", msg)
}

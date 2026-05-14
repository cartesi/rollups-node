// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"bytes"
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestEncodeWithdrawalConfig(t *testing.T) {
	cases := []iapplicationfactory.WithdrawalConfig{
		{},
		{
			Guardian:                common.HexToAddress("0x1111111111111111111111111111111111111111"),
			Log2LeavesPerAccount:    0,
			Log2MaxNumOfAccounts:    20,
			AccountsDriveStartIndex: 33554432,
			WithdrawalOutputBuilder: common.HexToAddress("0x2222222222222222222222222222222222222222"),
		},
		{
			Guardian:                common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"),
			Log2LeavesPerAccount:    7,
			Log2MaxNumOfAccounts:    19,
			AccountsDriveStartIndex: 12345,
			WithdrawalOutputBuilder: common.HexToAddress("0xcafebabecafebabecafebabecafebabecafebabe"),
		},
	}
	for i, wc := range cases {
		b, err := EncodeWithdrawalConfig(wc)
		require.NoError(t, err, "case %d encode", i)
		require.Equal(t, 160, len(b), "case %d encoded length (5 * 32 = 160 bytes)", i)

		// Round-trip: unpack the encoded bytes and assert field-by-field equality
		// with the original. Pinning this protects against an abigen field-order
		// shift between the four binding packages that share the WithdrawalConfig
		// shape (iapplicationfactory, iselfhostedapplicationfactory, idaveappfactory,
		// iapplication) — a silent reorder there would break encoding and the
		// length-only check would not catch it.
		unpacked, err := withdrawalConfigArgs.Unpack(b)
		require.NoError(t, err, "case %d unpack", i)
		require.Len(t, unpacked, 1, "case %d unpack arity", i)
		got := *abi.ConvertType(unpacked[0],
			new(iapplicationfactory.WithdrawalConfig)).(*iapplicationfactory.WithdrawalConfig)
		require.Equal(t, wc, got, "case %d round-trip", i)
	}

	// Zero-valued config must encode to 160 zero bytes — the canonical
	// "no foreclosure" sentinel used as the DEFAULT value in the deploy tx
	// ABI and assumed by downstream readers.
	zeroBytes, err := EncodeWithdrawalConfig(iapplicationfactory.WithdrawalConfig{})
	require.NoError(t, err)
	require.True(t, bytes.Equal(zeroBytes, make([]byte, 160)),
		"all-zero config must encode to 160 zero bytes")
}

func TestValidateWithdrawalConfig(t *testing.T) {
	tests := []struct {
		name    string
		wc      iapplicationfactory.WithdrawalConfig
		wantErr string // substring expected in the error message; "" means no error
	}{
		{
			name: "all zeros is valid (no foreclosure)",
			wc:   iapplicationfactory.WithdrawalConfig{},
		},
		{
			name: "typical realistic config",
			wc: iapplicationfactory.WithdrawalConfig{
				Guardian:                common.HexToAddress("0x1"),
				Log2LeavesPerAccount:    0,
				Log2MaxNumOfAccounts:    20,
				AccountsDriveStartIndex: 33554432,
				WithdrawalOutputBuilder: common.HexToAddress("0x2"),
			},
		},
		{
			name: "drive size at the memory boundary, start=0 (valid)",
			wc: iapplicationfactory.WithdrawalConfig{
				// 5 + 0 + 59 = 64 == log2MemorySize, start=0 -> end = 1 << 64 == 2^64 == memorySize
				Log2LeavesPerAccount:    0,
				Log2MaxNumOfAccounts:    59,
				AccountsDriveStartIndex: 0,
			},
		},
		{
			name: "drive too large (log2 sum > 64)",
			wc: iapplicationfactory.WithdrawalConfig{
				Log2LeavesPerAccount: 60,
				Log2MaxNumOfAccounts: 60,
			},
			wantErr: "larger than machine memory",
		},
		{
			name: "drive end overflows past memory (start>0 at boundary)",
			wc: iapplicationfactory.WithdrawalConfig{
				// 5 + 0 + 59 = 64 == log2MemorySize, start=1 -> end = 2 << 64 > 2^64
				Log2LeavesPerAccount:    0,
				Log2MaxNumOfAccounts:    59,
				AccountsDriveStartIndex: 1,
			},
			wantErr: "past machine memory",
		},
		{
			name: "drive end past machine memory (start non-zero)",
			wc: iapplicationfactory.WithdrawalConfig{
				// 5 + 0 + 30 = 35; start = 2^34 -> (start+1) << 35 > 2^64
				Log2LeavesPerAccount:    0,
				Log2MaxNumOfAccounts:    30,
				AccountsDriveStartIndex: 1 << 34,
			},
			wantErr: "past machine memory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWithdrawalConfig(tc.wc)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

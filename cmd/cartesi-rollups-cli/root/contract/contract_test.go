// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeUint64(t *testing.T) {
	tests := []struct {
		name    string
		value   *big.Int
		field   string
		want    uint64
		wantErr bool
	}{
		{
			name:  "zero",
			value: big.NewInt(0),
			field: "test",
			want:  0,
		},
		{
			name:  "normal value",
			value: big.NewInt(12345),
			field: "block",
			want:  12345,
		},
		{
			name:  "max uint64",
			value: new(big.Int).SetUint64(math.MaxUint64),
			field: "max",
			want:  math.MaxUint64,
		},
		{
			name:    "overflow",
			value:   new(big.Int).Add(new(big.Int).SetUint64(math.MaxUint64), big.NewInt(1)),
			field:   "overflow",
			wantErr: true,
		},
		{
			name:    "negative",
			value:   big.NewInt(-1),
			field:   "negative",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeUint64(tt.value, tt.field)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.field)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseBlockFlag(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *big.Int
		wantErr bool
	}{
		{
			name:  "empty string returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "latest returns nil",
			input: "latest",
			want:  nil,
		},
		{
			name:  "decimal number",
			input: "12345",
			want:  big.NewInt(12345),
		},
		{
			name:  "hex number",
			input: "0xFF",
			want:  big.NewInt(255),
		},
		{
			name:  "zero",
			input: "0",
			want:  big.NewInt(0),
		},
		{
			name:    "negative number",
			input:   "-1",
			wantErr: true,
		},
		{
			name:    "invalid string",
			input:   "abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBlockFlag(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.want == nil {
					assert.Nil(t, got)
				} else {
					require.NotNil(t, got)
					assert.Equal(t, 0, tt.want.Cmp(got))
				}
			}
		})
	}
}

func TestSafeUint64Nil(t *testing.T) {
	_, err := safeUint64(nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestValidateHash(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid with prefix", "0x" + strings.Repeat("ab", 32), false},
		{"valid uppercase", "0x" + strings.Repeat("AB", 32), false},
		{"no prefix accepted", strings.Repeat("ab", 32), false},
		{"too short", "0xabcd", true},
		{"too long", "0x" + strings.Repeat("ab", 33), true},
		{"invalid hex", "0x" + strings.Repeat("gg", 32), true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHash(tt.input, "test hash")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConsensusTypeString(t *testing.T) {
	assert.Equal(t, "Authority", consensusAuthority.String())
	assert.Equal(t, "Quorum", consensusQuorum.String())
	assert.Equal(t, "DaveConsensus (PRT)", consensusDave.String())
	assert.Equal(t, "Unknown", consensusUnknown.String())
}

// TestIConsensusV3InterfaceID locks down the v3 interface ID computation.
// If a method is renamed in the binding or this list drifts from the
// IConsensus.sol interface, this test surfaces the change as a value mismatch.
func TestIConsensusV3InterfaceID(t *testing.T) {
	// Non-zero, distinct from the v2 IDs.
	assert.NotEqual(t, [4]byte{}, iConsensusInterfaceIDv30,
		"v3 interface ID should be non-zero")
	assert.NotEqual(t, iConsensusInterfaceIDv220, iConsensusInterfaceIDv30,
		"v3 interface ID should differ from v2.2.0")
	assert.NotEqual(t, iConsensusInterfaceIDv21x, iConsensusInterfaceIDv30,
		"v3 interface ID should differ from v2.1.x")
}

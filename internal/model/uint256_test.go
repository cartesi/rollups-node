// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestUint256Codec(t *testing.T) {
	const maximum = "115792089237316195423570985008687907853269984665640564039457584007913129639935"
	const over = "115792089237316195423570985008687907853269984665640564039457584007913129639936"
	const maxHex = `"0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"`
	mapping := pgtype.NewMap()
	for _, decimal := range []string{"0", "18446744073709551616", maximum} {
		var value Uint256
		if err := value.Scan(decimal); err != nil {
			t.Fatal(err)
		}
		for _, format := range []int16{pgtype.TextFormatCode, pgtype.BinaryFormatCode} {
			encoded, err := mapping.Encode(pgtype.NumericOID, format, value, nil)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Uint256
			if err := mapping.Scan(pgtype.NumericOID, format, encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if value != decoded {
				t.Fatalf("numeric round trip: got %v want %v", decoded, value)
			}
		}
		if decimal == maximum {
			encoded, err := json.Marshal(value)
			if err != nil || string(encoded) != maxHex {
				t.Fatalf("JSON: %s, %v", encoded, err)
			}
		}
	}
	for _, src := range []any{nil, "", "-1", "0.5", over, 123, "1e78"} {
		value := Uint256(*uint256.NewInt(7))
		before := value
		if err := value.Scan(src); err == nil {
			t.Fatalf("accepted invalid SQL value %#v", src)
		}
		if value != before {
			t.Fatalf("changed receiver after SQL error %#v", src)
		}
	}
	for _, raw := range []string{`"0x0"`, `"0x10000000000000000"`, maxHex} {
		var value Uint256
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(value)
		if err != nil || string(encoded) != raw {
			t.Fatalf("JSON round trip: %s, %v", encoded, err)
		}
	}
	for _, raw := range []string{
		`null`, `1`, `"1"`, `"0x"`, `"0x00"`, `"-0x1"`,
		`"0x10000000000000000000000000000000000000000000000000000000000000000"`,
	} {
		value := Uint256(*uint256.NewInt(7))
		before := value
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			t.Fatalf("accepted invalid JSON %s", raw)
		}
		if value != before {
			t.Fatalf("changed receiver after JSON error %s", raw)
		}
	}
	var nullable *Uint256
	if err := mapping.Scan(pgtype.NumericOID, pgtype.TextFormatCode, nil, &nullable); err != nil {
		t.Fatal(err)
	}
	if nullable != nil {
		t.Fatal("SQL NULL must remain nil")
	}
	for _, format := range []int16{pgtype.TextFormatCode, pgtype.BinaryFormatCode} {
		scaledInteger := pgtype.Numeric{Int: big.NewInt(100), Exp: -2, Valid: true}
		encoded, err := mapping.Encode(pgtype.NumericOID, format, scaledInteger, nil)
		require.NoError(t, err)
		var decoded Uint256
		require.NoError(t, mapping.Scan(pgtype.NumericOID, format, encoded, &decoded))
		require.Equal(t, big.NewInt(1), decoded.ToBig())
	}
}

func TestUint256FromBig(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	for _, value := range []*big.Int{nil, big.NewInt(-1), limit} {
		_, err := Uint256FromBig(value)
		require.ErrorIs(t, err, ErrInvalidUint256)
	}
	maximum := new(big.Int).Sub(new(big.Int).Set(limit), big.NewInt(1))
	value, err := Uint256FromBig(maximum)
	require.NoError(t, err)
	require.Equal(t, maximum, value.ToBig())
	maximum.SetInt64(0)
	require.NotZero(t, value.ToBig().Sign(), "the model must not share mutable big.Int storage")
}

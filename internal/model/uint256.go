// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/holiman/uint256"
)

// Uint256 stores a Solidity unsigned integer without narrowing its value.
// SQL uses decimal NUMERIC values; JSON uses canonical hexadecimal quantities.
type Uint256 uint256.Int

var ErrInvalidUint256 = errors.New("invalid uint256")

func Uint256FromBig(value *big.Int) (Uint256, error) {
	if value == nil || value.Sign() < 0 {
		return Uint256{}, ErrInvalidUint256
	}
	parsed, overflow := uint256.FromBig(value)
	if overflow {
		return Uint256{}, ErrInvalidUint256
	}
	return Uint256(*parsed), nil
}

func (value Uint256) ToBig() *big.Int {
	return (*uint256.Int)(&value).ToBig()
}

func (value *Uint256) Scan(source any) error {
	if source == nil {
		return fmt.Errorf("cannot scan NULL into Uint256: %w", ErrInvalidUint256)
	}
	// PostgreSQL NUMERIC can retain decimal scale for an exact integer (1.00).
	// Remove only zero fractional digits; a fractional value is not uint256.
	var text string
	switch source := source.(type) {
	case string:
		text = source
	case []byte:
		text = string(source)
	default:
		return fmt.Errorf("unsupported Uint256 scan type %T: %w", source, ErrInvalidUint256)
	}
	if text == "" {
		return fmt.Errorf("empty Uint256 value: %w", ErrInvalidUint256)
	}
	if whole, fraction, found := strings.Cut(text, "."); found {
		if fraction == "" || strings.Trim(fraction, "0") != "" {
			return fmt.Errorf("fractional Uint256 value: %w", ErrInvalidUint256)
		}
		text = whole
	}
	var parsed uint256.Int
	if err := parsed.Scan(text); err != nil {
		return fmt.Errorf("scan Uint256: %w", err)
	}
	*value = Uint256(parsed)
	return nil
}

func (value Uint256) Value() (driver.Value, error) {
	return (*uint256.Int)(&value).Value()
}

func (value Uint256) MarshalJSON() ([]byte, error) {
	return json.Marshal((*uint256.Int)(&value).Hex())
}

func (value *Uint256) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	var parsed uint256.Int
	if err := parsed.SetFromHex(text); err != nil {
		return fmt.Errorf("decode Uint256: %w", err)
	}
	*value = Uint256(parsed)
	return nil
}

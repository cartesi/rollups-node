// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// CartesiMachineMarchID identifies the required machine architecture.
const CartesiMachineMarchID uint64 = 22

// ErrIncompatibleStateTransition identifies a different machine state transition.
var ErrIncompatibleStateTransition = errors.New("incompatible state transition")

// UarchPristineStateHash returns the required initial uarch state hash.
func UarchPristineStateHash() common.Hash {
	return common.Hash{
		0x55, 0x8f, 0x38, 0xf2, 0x35, 0x09, 0x70, 0xa0,
		0xa5, 0x66, 0xb2, 0x45, 0x0a, 0xfa, 0x64, 0x25,
		0x00, 0x59, 0xc1, 0xcf, 0xf6, 0x6c, 0x9a, 0x55,
		0x56, 0x0e, 0x5b, 0x93, 0xfa, 0x78, 0x39, 0x26,
	}
}

// CheckMarchID compares the subject's architecture with the required identity.
func CheckMarchID(subject string, marchID uint64) error {
	if marchID != CartesiMachineMarchID {
		return fmt.Errorf("%s marchid %d, expected %d: %w",
			subject, marchID, CartesiMachineMarchID, ErrIncompatibleStateTransition)
	}
	return nil
}

// CheckUarchPristineStateHash compares the subject's pristine uarch hash with the required identity.
func CheckUarchPristineStateHash(subject string, hash common.Hash) error {
	expected := UarchPristineStateHash()
	if hash != expected {
		return fmt.Errorf("%s pristine uarch hash %s, expected %s: %w",
			subject, hash, expected, ErrIncompatibleStateTransition)
	}
	return nil
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// CartesiMachineMarchID identifies the required machine architecture.
const CartesiMachineMarchID uint64 = 23

// ErrIncompatibleStateTransition identifies a different machine state transition.
var ErrIncompatibleStateTransition = errors.New("incompatible state transition")

// UarchPristineStateHash returns the required initial uarch state hash.
func UarchPristineStateHash() common.Hash {
	return common.Hash{
		0x89, 0xae, 0x34, 0xba, 0x1d, 0xc6, 0x5a, 0x8a,
		0x36, 0x68, 0xb4, 0x84, 0xbc, 0xed, 0xf8, 0x97,
		0x66, 0x5c, 0x80, 0x3c, 0x41, 0xdc, 0x9e, 0xf9,
		0xf1, 0xfa, 0xf1, 0x47, 0xb9, 0xd4, 0x43, 0xd7,
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

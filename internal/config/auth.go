// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

// Auth objects are used to sign transactions.
type Auth any

// Allows signing through private keys.
type AuthPrivateKey struct {
	PrivateKey string
}

// Allows signing through mnemonics.
type AuthMnemonic struct {
	Mnemonic     string
	AccountIndex int
}

// Allows signing through AWS services.
type AuthAWS struct {
	KeyID  string
	Region string
}

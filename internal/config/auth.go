// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import "log/slog"

// Auth objects are used to sign transactions.
type Auth any

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

func (a AuthMnemonic) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("Mnemonic", "[REDACTED]"),
		slog.String("AccountIndex", "[REDACTED]"),
	)
}

func (a AuthAWS) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("KeyID", "[REDACTED]"),
		slog.String("Region", "[REDACTED]"),
	)
}

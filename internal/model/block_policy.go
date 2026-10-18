// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import "strings"

// Provisional identifies the supported policies whose observations can regress.
// It does not validate configuration; pending and unknown policies are not supported.
func (e DefaultBlock) Provisional() bool {
	return e == DefaultBlock_Latest || e == DefaultBlock_Safe
}

// Tag returns the JSON-RPC spelling. Configuration validation remains separate.
func (e DefaultBlock) Tag() string { return strings.ToLower(e.String()) }

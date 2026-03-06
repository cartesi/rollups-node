// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package cli defines the JSON output types used by the CLI commands.
// Types here are CLI-specific (SendResult, ExecuteResult, ValidateResult).
// Response envelope types (ListResponse, SingleResponse, Pagination) live
// in internal/jsonrpc/api.
package cli

// SendResult is the JSON output of the "send" CLI command.
type SendResult struct {
	ApplicationAddress string `json:"application_address"`
	InputIndex         string `json:"input_index"`
	BlockNumber        string `json:"block_number"`
}

// ExecuteResult is the JSON output of the "execute" CLI command.
type ExecuteResult struct {
	TransactionHash string `json:"transaction_hash"`
}

// ValidateResult is the JSON output of the "validate" CLI command.
type ValidateResult struct {
	Valid bool `json:"valid"`
}

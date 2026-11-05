// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

// Pagination represents common pagination structure used in responses.
type Pagination struct {
	TotalCount uint64 `json:"total_count"`
	Limit      uint64 `json:"limit"`
	Offset     uint64 `json:"offset"`
}

// ListResponse is the generic envelope for list endpoints.
type ListResponse[T any] struct {
	Data       []T        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// SingleResponse is the generic envelope for get endpoints.
type SingleResponse[T any] struct {
	Data T `json:"data"`
}

type NodeInfo struct {
	ChainID      string `json:"chain_id"`
	Version      string `json:"version"`
	DefaultBlock string `json:"default_block"` // FINALIZED | SAFE | LATEST | PENDING
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

import (
	"encoding/json"
	"io"
)

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

// WriteJSON encodes one item at a time so a bounded writer can stop a large
// response without first allocating its entire JSON representation. Repository
// rows and individual items still require their own working memory.
func (r ListResponse[T]) WriteJSON(w io.Writer) error {
	write := func(s string) error { _, err := io.WriteString(w, s); return err }
	if err := write(`{"data":`); err != nil {
		return err
	}
	if r.Data == nil {
		if err := write("null"); err != nil {
			return err
		}
	} else {
		if err := write("["); err != nil {
			return err
		}
		for i, item := range r.Data {
			if i > 0 {
				if err := write(","); err != nil {
					return err
				}
			}
			data, err := json.Marshal(item)
			if err != nil {
				return err
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
		}
		if err := write("]"); err != nil {
			return err
		}
	}
	if err := write(`,"pagination":`); err != nil {
		return err
	}
	data, err := json.Marshal(r.Pagination)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return write("}")
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

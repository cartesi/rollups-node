// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Input struct {
	// Input index starting from genesis
	Index uint64 `json:"index"`
	// Status of the input
	Status string `json:"status"`
	// Input data as a blob, starting with '0x'
	Blob hexutil.Bytes `json:"blob"`
}

type Output struct {
	// Input whose processing produced the output
	InputIndex uint64 `json:"inputIndex"`
	// Output index within the context of the input that produced it
	Index uint64 `json:"index"`
	// Output data as a blob, starting with '0x'
	Blob hexutil.Bytes `json:"blob"`
}

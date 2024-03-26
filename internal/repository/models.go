// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Input struct {
	// Input index starting from genesis
	Index int `json:"index"`
	// Status of the input
	Status string `json:"status"`
	// Input data as a blob, starting with '0x'
	Blob hexutil.Bytes `json:"blob"`
}

type Output struct {
	// Input whose processing produced the output
	InputIndex int `json:"inputIndex"`
	// Output index within the context of the input that produced it
	Index int `json:"index"`
	// Output data as a blob, starting with '0x'
	Blob hexutil.Bytes `json:"blob"`
}

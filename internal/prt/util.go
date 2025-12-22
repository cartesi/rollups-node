// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

type JSONRPCInfo struct {
	Code    int
	Message string
	Data    any
	HasCode bool
	HasData bool
}

func ExtractJsonErrorInfo(err error) (JSONRPCInfo, bool) {
	var out JSONRPCInfo
	if err == nil {
		return out, false
	}

	var e rpc.Error
	if errors.As(err, &e) {
		out.Code = e.ErrorCode()
		out.Message = e.Error()
		out.HasCode = true
	}

	var de rpc.DataError
	if errors.As(err, &de) {
		out.Data = de.ErrorData()
		out.HasData = true
		if !out.HasCode {
			out.Message = de.Error()
		}
	}

	return out, out.HasCode || out.HasData
}

func asBytes32Slice(proof []common.Hash) [][32]byte {
	return *(*[][32]byte)(unsafe.Pointer(&proof))
}

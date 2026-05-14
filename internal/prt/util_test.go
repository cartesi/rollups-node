// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

// rpcDataError simulates an RPC error with revert data, as returned by
// eth_estimateGas when a contract reverts.
type rpcDataError struct {
	code int
	msg  string
	data any
}

func (e *rpcDataError) Error() string  { return e.msg }
func (e *rpcDataError) ErrorCode() int { return e.code }
func (e *rpcDataError) ErrorData() any { return e.data }

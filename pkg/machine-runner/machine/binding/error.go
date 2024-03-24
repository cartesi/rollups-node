// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package binding

// #include <stdlib.h>
// #include <cartesi-machine/jsonrpc-machine-c-api.h>
import "C"

type Error struct {
	Code    ErrorCode
	Message string
}

func newError(code C.int, message *C.char) error {
	defer C.cm_delete_cstring(message)
	if code := newErrorCode(C.CM_ERROR(code)); code != ErrorOk {
		return Error{Code: code, Message: C.GoString(message)}
	}
	return nil
}

func (err Error) String() string {
	return "cartesi-machine error: " + err.Code.String() + " (" + err.Message + ")"
}

func (err Error) Error() string {
	return err.String()
}

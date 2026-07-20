// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package is a binding to the emulator's C API.
// Refer to the machine-c files in the emulator's repository for documentation
// (mainly machine-c-api.h and jsonrpc-machine-c-api.h).
package emulator

// #include <stdlib.h>
// #include "cartesi-machine/cm-jsonrpc.h"
import "C"

import (
	"unsafe"
)

// -----------------------------------------------------------------------------
// RemoteMachine: the remote machine object
// -----------------------------------------------------------------------------

type RemoteMachine struct {
	Machine
}

// set_timeout
func (m *RemoteMachine) SetTimeout(milliseconds int64) error {
	var err error
	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_set_timeout(m.ptr, C.int64_t(milliseconds)))
	})
	return err
}

// get_timeout
func (m *RemoteMachine) GetTimeout() (int64, error) {
	var ms C.int64_t
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_get_timeout(m.ptr, &ms))
	})

	if err != nil {
		return 0, err
	}
	return int64(ms), nil
}

// set_cleanup_call
func (m *RemoteMachine) SetCleanupCall(call int32) error {
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_set_cleanup_call(m.ptr, C.cm_jsonrpc_cleanup_call(call)))
	})

	return err
}

// get_cleanup_call
func (m *RemoteMachine) GetCleanupCall() (int32, error) {
	var call C.cm_jsonrpc_cleanup_call
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_get_cleanup_call(m.ptr, &call))
	})

	if err != nil {
		return 0, err
	}
	return int32(call), nil
}

// get_server_address
func (m *RemoteMachine) GetServerAddress() (string, error) {
	var cAddr *C.char
	var err error
	var addr string

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_get_server_address(m.ptr, &cAddr))
		addr = C.GoString(cAddr)
	})

	if err != nil {
		return "", err
	}
	return addr, nil
}

// get_server_version
func (m *RemoteMachine) GetServerVersion() (string, error) {
	var cVer *C.char
	var err error
	var ver string

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_get_server_version(m.ptr, &cVer))
		ver = C.GoString(cVer)
	})

	if err != nil {
		return "", err
	}
	return ver, nil
}

// fork_server
func (m *RemoteMachine) ForkServer() (*RemoteMachine, string, uint32, error) {
	var forked *C.cm_machine
	var cAddr *C.char
	var addr string
	var pid C.uint32_t
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_fork_server(m.ptr, &forked, &cAddr, &pid))
		addr = C.GoString(cAddr)
	})

	if err != nil {
		return nil, "", 0, err
	}
	return &RemoteMachine{Machine: Machine{ptr: forked}}, addr, uint32(pid), nil
}

// rebind_server
func (m *RemoteMachine) RebindServer(newAddr string) (string, error) {
	var cBoundAddr *C.char
	var boundAddr string
	var err error

	m.callCAPI(func() {
		cAddr := C.CString(newAddr)
		defer C.free(unsafe.Pointer(cAddr))

		err = newError(C.cm_jsonrpc_rebind_server(m.ptr, cAddr, &cBoundAddr))
		boundAddr = C.GoString(cBoundAddr)
	})

	if err != nil {
		return "", err
	}
	return boundAddr, nil
}

// shutdown_server
func (m *RemoteMachine) ShutdownServer() error {
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_shutdown_server(m.ptr))
	})

	return err
}

// emancipate_server
func (m *RemoteMachine) EmancipateServer() error {
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_emancipate_server(m.ptr))
	})

	return err
}

// delay_next_request
func (m *RemoteMachine) DelayNextRequest(milliseconds uint64) error {
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_jsonrpc_delay_next_request(m.ptr, C.uint64_t(milliseconds)))
	})

	return err
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package is a binding to the emulator's C API.
// Refer to the machine-c files in the emulator's repository for documentation
// (mainly machine-c-api.h and jsonrpc-machine-c-api.h).
//
//nolint:gocritic // CGo output-parameter wrappers trigger dupSubExpr false positives.
package emulator

// #cgo LDFLAGS: -lcartesi -lcartesi_jsonrpc
// #include <stdlib.h>
// #include "cartesi-machine/cm-jsonrpc.h"
import "C"

import (
	"encoding/json"
	"runtime"
	"time"
	"unsafe"
)

// GetVersion returns the linked library's encoded runtime semantic version.
func GetVersion() uint64 {
	return uint64(C.cm_get_version())
}

// DefaultMarchID reads the architecture from the linked library's default configuration.
func DefaultMarchID() (uint64, error) {
	var local Machine
	config, err := local.GetDefaultConfig()
	if err != nil {
		return 0, err
	}
	var decoded struct {
		Processor struct {
			Registers struct {
				MarchID uint64 `json:"marchid"`
			} `json:"registers"`
		} `json:"processor"`
	}
	if err := json.Unmarshal([]byte(config), &decoded); err != nil {
		return 0, err
	}
	return decoded.Processor.Registers.MarchID, nil
}

// GetUarchPristineHash reads the pristine uarch hash from the linked library.
func GetUarchPristineHash() (Hash, error) {
	var local Machine
	return local.GetUarchPristineHash()
}

func ConnectServer(address string, timeout time.Duration) (*RemoteMachine, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cAddr := C.CString(address)
	defer C.free(unsafe.Pointer(cAddr))

	var cm *C.cm_machine
	if err := newError(C.cm_jsonrpc_connect_server(cAddr, C.int64_t(timeout.Milliseconds()), &cm)); err != nil {
		return nil, err
	}
	return &RemoteMachine{Machine: Machine{ptr: cm}}, nil
}

func SpawnServer(address string, timeout time.Duration) (*RemoteMachine, string, uint32, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cAddr := C.CString(address)
	defer C.free(unsafe.Pointer(cAddr))

	var cm *C.cm_machine
	var boundAddr *C.char
	var pid C.uint32_t
	code := C.cm_jsonrpc_spawn_server(cAddr, C.int64_t(timeout.Milliseconds()), &cm, &boundAddr, &pid)
	if err := newError(code); err != nil {
		return nil, "", 0, err
	}
	return &RemoteMachine{Machine: Machine{ptr: cm}}, C.GoString(boundAddr), uint32(pid), nil
}

func CreateMachine(config, runtimeConfig, dir string) (*Machine, error) {
	machine := &Machine{}
	err := machine.Create(config, runtimeConfig, dir)
	return machine, err
}

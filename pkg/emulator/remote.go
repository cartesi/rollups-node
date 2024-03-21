// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

// #include <stdlib.h>
// #include "cartesi-machine/jsonrpc-machine-c-api.h"
import "C"
import "unsafe"

// A connection to the remote jsonrpc machine manager.
type RemoteMachineManager struct {
	c *C.cm_jsonrpc_mg_mgr

	Address string
}

func NewRemoteMachineManager(address string) (*RemoteMachineManager, error) {
	manager := &RemoteMachineManager{Address: address}
	cRemoteAddress := C.CString(address)
	defer C.free(unsafe.Pointer(cRemoteAddress))
	var msg *C.char
	code := C.cm_create_jsonrpc_mg_mgr(cRemoteAddress, &manager.c, &msg)
	return manager, newError(code, msg)
}

func (remote *RemoteMachineManager) Delete() {
	if remote.c != nil {
		C.cm_delete_jsonrpc_mg_mgr(remote.c)
		remote.c = nil
	}
}

func (remote *RemoteMachineManager) NewMachine(
	config *MachineConfig,
	runtime *MachineRuntimeConfig,
) (*Machine, error) {
	var msg *C.char
	machine := &Machine{remote: remote}
	configRef := config.makeCRef()
	defer configRef.free()
	runtimeRef := runtime.makeCRef()
	defer runtimeRef.free()
	code := C.cm_create_jsonrpc_machine(remote.c, configRef.cref, runtimeRef.cref, &machine.c, &msg)
	return machine, newError(code, msg)
}

func (remote *RemoteMachineManager) LoadMachine(
	directory string,
	runtime *MachineRuntimeConfig,
) (*Machine, error) {
	var msg *C.char
	machine := &Machine{remote: remote}
	dir := C.CString(directory)
	defer C.free(unsafe.Pointer(dir))
	runtimeRef := runtime.makeCRef()
	defer runtimeRef.free()
	code := C.cm_load_jsonrpc_machine(remote.c, dir, runtimeRef.cref, &machine.c, &msg)
	return machine, newError(code, msg)
}

func (remote *RemoteMachineManager) GetMachine() (*Machine, error) {
	var msg *C.char
	machine := &Machine{remote: remote}
	code := C.cm_get_jsonrpc_machine(remote.c, &machine.c, &msg)
	return machine, newError(code, msg)
}

func (remote *RemoteMachineManager) GetDefaultMachineConfig() (*MachineConfig, error) {
	var msg *C.char
	theirCfg := theirMachineConfigCRef{}
	defer theirCfg.free()
	code := C.cm_jsonrpc_get_default_config(remote.c, &theirCfg.cref, &msg)
	if err := newError(code, msg); err != nil {
		return nil, err
	}
	return theirCfg.makeGoRef(), nil
}

func (remote *RemoteMachineManager) Fork() (newAddress string, _ error) {
	var msg *C.char
	var address *C.char
	defer C.cm_delete_cstring(address)
	code := C.cm_jsonrpc_fork(remote.c, &address, &msg)
	return C.GoString(address), newError(code, msg)
}

func (remote *RemoteMachineManager) Shutdown() error {
	var msg *C.char
	code := C.cm_jsonrpc_shutdown(remote.c, &msg)
	return newError(code, msg)
}

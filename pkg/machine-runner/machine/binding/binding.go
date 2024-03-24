// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// TODO: retry com backoff nas funções exportadas.
package binding

// #cgo LDFLAGS: -lcartesi -lcartesi_jsonrpc
// #include <stdlib.h>
// #include <cartesi-machine/jsonrpc-machine-c-api.h>
import "C"
import (
	"unsafe"
)

type Binding struct {
	machine *C.cm_machine

	// memory ranges
	RxBufferStart uint64
	TxBufferStart uint64
}

// ------------------------------
// Constructors
// ------------------------------

func Load(snapshot string, remote *Remote, config RuntimeConfig) (*Binding, error) {
	var message *C.char

	dir := C.CString(snapshot)
	defer C.free(unsafe.Pointer(dir))
	runtimeConfig := config.toC()
	var machine *C.cm_machine

	// Loads the remote cartesi machine.
	code := C.cm_load_jsonrpc_machine(remote.mgr, dir, &runtimeConfig, &machine, &message)
	if err := newError(code, message); err != nil {
		return nil, err
	}

	// Gets the cartesi machine configuration.
	var machineConfig *C.cm_machine_config
	code = C.cm_get_initial_config(machine, &machineConfig, &message)
	if err := newError(code, message); err != nil {
		return nil, err
	}

	return &Binding{
		machine:       machine,
		RxBufferStart: uint64(machineConfig.cmio.rx_buffer.start),
		TxBufferStart: uint64(machineConfig.cmio.tx_buffer.start),
	}, nil
}

func From(remote *Remote) (*Binding, error) {
	var message *C.char
	var machine *C.cm_machine
	code := C.cm_get_jsonrpc_machine(remote.mgr, &machine, &message)
	if err := newError(code, message); err != nil {
		return nil, err
	}

	// Gets the cartesi machine configuration.
	var machineConfig *C.cm_machine_config
	code = C.cm_get_initial_config(machine, &machineConfig, &message)
	if err := newError(code, message); err != nil {
		return nil, err
	}

	return &Binding{
		machine:       machine,
		RxBufferStart: uint64(machineConfig.cmio.rx_buffer.start),
		TxBufferStart: uint64(machineConfig.cmio.tx_buffer.start),
	}, nil
}

// ------------------------------
// Bindings
// ------------------------------

func (binding *Binding) Delete() {
	C.cm_delete_machine(binding.machine)
}

func (binding *Binding) Destroy() error {
	var message *C.char
	code := C.cm_destroy(binding.machine, &message)
	return newError(code, message)
}

type Cycle uint64

func (binding *Binding) Run(cycles Cycle) (BreakReason, error) {
	var message *C.char
	var breakReason C.CM_BREAK_REASON

	code := C.cm_machine_run(binding.machine, C.uint64_t(cycles), &breakReason, &message)
	if err := newError(code, message); err != nil {
		return InvalidBreakReason, err
	}

	return newBreakReason(breakReason), nil
}

func (binding Binding) ReadMachineCycle() (Cycle, error) {
	var message *C.char
	var value C.uint64_t
	code := C.cm_read_mcycle(binding.machine, &value, &message)
	err := newError(code, message)
	return Cycle(value), err
}

func (binding Binding) ReadIflagsY() (bool, error) {
	var message *C.char
	var value C.bool
	code := C.cm_read_iflags_Y(binding.machine, &value, &message)
	err := newError(code, message)
	return bool(value), err
}

func (binding Binding) ResetIflagsY() error {
	var message *C.char
	code := C.cm_reset_iflags_Y(binding.machine, &message)
	return newError(code, message)
}

func (binding Binding) ReadHtifIyield() (uint64, error) {
	var message *C.char
	var value C.uint64_t
	code := C.cm_read_htif_iyield(binding.machine, &value, &message)
	err := newError(code, message)
	return uint64(value), err
}

func (binding Binding) ReadHtifToHostData() (uint64, error) {
	var message *C.char
	var value C.uint64_t
	code := C.cm_read_htif_tohost_data(binding.machine, &value, &message)
	return uint64(value), newError(code, message)
}

func (binding Binding) ReadHtifFromHostData() (uint64, error) {
	var message *C.char
	var value C.uint64_t
	code := C.cm_read_htif_fromhost(binding.machine, &value, &message)
	return uint64(value), newError(code, message)
}

func (binding Binding) WriteHtifFromHostData(value uint64) error {
	var message *C.char
	code := C.cm_write_htif_fromhost_data(binding.machine, C.uint64_t(value), &message)
	return newError(code, message)
}

func (binding Binding) ReadMemory(address, length uint64) ([]byte, error) {
	var message *C.char
	data := (*C.uchar)(C.malloc(C.size_t(length * C.sizeof_uchar)))
	defer C.free(unsafe.Pointer(data))

	code := C.cm_read_memory(binding.machine,
		C.uint64_t(address),
		data,
		C.uint64_t(length),
		&message,
	)
	if err := newError(code, message); err != nil {
		return nil, err
	}

	return C.GoBytes(unsafe.Pointer(data), C.int(length)), nil
}

func (binding *Binding) WriteMemory(address uint64, data []byte) error {
	{
		var message *C.char

		length := C.size_t(len(data))
		data := C.CBytes(data)
		defer C.free(data)

		code := C.cm_write_memory(
			binding.machine,
			C.uint64_t(address),
			(*C.uchar)(data),
			length,
			&message,
		)
		return newError(code, message)
	}
}

func (binding *Binding) Snapshot() error {
	var message *C.char
	code := C.cm_snapshot(binding.machine, &message)
	return newError(code, message)
}

func (binding *Binding) Rollback() error {
	var message *C.char
	code := C.cm_rollback(binding.machine, &message)
	return newError(code, message)
}

// ------------------------------
// Decorated
// ------------------------------

func (binding Binding) ReadYieldReason() (YieldReason, error) {
	value, err := binding.ReadHtifToHostData()
	return newYieldReason(value >> 32), err
}

// ------------------------------
// TODO
// ------------------------------

func createJsonRpcMgr(address string) (*C.cm_jsonrpc_mg_mgr, error) {
	{
		var message *C.char
		address := C.CString(address)
		defer C.free(unsafe.Pointer(address))
		var mgr *C.cm_jsonrpc_mg_mgr

		code := C.cm_create_jsonrpc_mg_mgr(address, &mgr, &message)
		if err := newError(code, message); err != nil {
			return nil, err
		}

		return mgr, nil
	}
}

func forkJsonrpcMgr(mgr *C.cm_jsonrpc_mg_mgr) (string, error) {
	var message *C.char
	var address *C.char
	defer C.cm_delete_cstring(address)

	code := C.cm_jsonrpc_fork(mgr, &address, &message)
	if err := newError(code, message); err != nil {
		return "", err
	}

	return C.GoString(address), nil
}

func deleteJsonrpcMgr(mgr *C.cm_jsonrpc_mg_mgr) {
	C.cm_delete_jsonrpc_mg_mgr(mgr)
}

func jsonrpcShutdown(mgr *C.cm_jsonrpc_mg_mgr) error {
	var message *C.char
	code := C.cm_jsonrpc_shutdown(mgr, &message)
	return newError(code, message)
}

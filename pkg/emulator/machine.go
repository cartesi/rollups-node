// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

// #include <stdlib.h>
// #include "cartesi-machine/jsonrpc-machine-c-api.h"
import "C"
import "unsafe"

// A local or remote machine.
type Machine struct {
	c      *C.cm_machine
	remote *RemoteMachineManager
}

func (machine *Machine) GetInitialConfig() (*MachineConfig, error) {
	var msg *C.char
	theirCfg := theirMachineConfigCRef{}
	defer theirCfg.free()
	code := C.cm_get_initial_config(machine.c, &theirCfg.cref, &msg)
	if err := newError(code, msg); err != nil {
		return nil, err
	}
	return theirCfg.makeGoRef(), nil
}

func NewMachine(config *MachineConfig, runtime *MachineRuntimeConfig) (*Machine, error) {
	machine := &Machine{}
	configRef := config.makeCRef()
	defer configRef.free()
	runtimeRef := runtime.makeCRef()
	defer runtimeRef.free()
	var msg *C.char
	code := C.cm_create_machine(configRef.cref, runtimeRef.cref, &machine.c, &msg)
	return machine, newError(code, msg)
}

func LoadMachine(dir string, runtime *MachineRuntimeConfig) (*Machine, error) {
	machine := &Machine{}
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	runtimeRef := runtime.makeCRef()
	defer runtimeRef.free()
	var msg *C.char
	code := C.cm_load_machine(cDir, runtimeRef.cref, &machine.c, &msg)
	return machine, newError(code, msg)
}

func (machine *Machine) Store(dir string) error {
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	var msg *C.char
	code := C.cm_store(machine.c, cDir, &msg)
	return newError(code, msg)
}

func (machine *Machine) Delete() {
	if machine.c != nil {
		C.cm_delete_machine(machine.c)
		machine.c = nil
	}
}

func (machine *Machine) Destroy() error {
	var msg *C.char
	code := C.cm_destroy(machine.c, &msg)
	return newError(code, msg)
}

func (machine *Machine) Snapshot() error {
	var msg *C.char
	code := C.cm_snapshot(machine.c, &msg)
	return newError(code, msg)
}

func (machine *Machine) Rollback() error {
	var msg *C.char
	code := C.cm_rollback(machine.c, &msg)
	return newError(code, msg)
}

func (machine *Machine) Run(mcycleEnd uint64) (BreakReason, error) {
	var msg *C.char
	var reason C.CM_BREAK_REASON
	code := C.cm_machine_run(machine.c, C.uint64_t(mcycleEnd), &reason, &msg)
	if err := newError(code, msg); err != nil {
		return BreakReasonFailed, err
	}
	return (BreakReason)(reason), nil
}

func (machine *Machine) GetRootHash() (hash MerkleTreeHash, _ error) {
	var msg *C.char
	var chash C.cm_hash
	code := C.cm_get_root_hash(machine.c, &chash, &msg)
	if err := newError(code, msg); err != nil {
		return hash, err
	}

	for i := 0; i < 32; i++ {
		hash[i] = byte(chash[i])
	}
	return hash, nil
}

func (machine Machine) ReadMCycle() (uint64, error) {
	var msg *C.char
	var value C.uint64_t
	code := C.cm_read_mcycle(machine.c, &value, &msg)
	return uint64(value), newError(code, msg)
}

func (machine *Machine) ReplaceMemoryRange(newRange *MemoryRangeConfig) error {
	var msg *C.char
	newRangeRef := newRange.makeCRef()
	defer newRangeRef.free()
	code := C.cm_replace_memory_range(machine.c, newRangeRef.cref, &msg)
	return newError(code, msg)
}

func (machine *Machine) ReadMemory(address, length uint64) ([]byte, error) {
	var msg *C.char
	data := make([]byte, length)
	code := C.cm_read_memory(machine.c,
		C.uint64_t(address),
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.uint64_t(length),
		&msg)
	return data, newError(code, msg)
}

func (machine *Machine) WriteMemory(address uint64, data []byte) error {
	var msg *C.char
	code := C.cm_write_memory(machine.c,
		C.uint64_t(address),
		(*C.uchar)(unsafe.Pointer(&data[0])),
		C.size_t(len(data)),
		&msg)
	return newError(code, msg)
}

func (machine *Machine) ReadCSR(r ProcessorCSR) (uint64, error) {
	var msg *C.char
	var value C.uint64_t
	code := C.cm_read_csr(machine.c, C.CM_PROC_CSR(r), &value, &msg)
	return uint64(value), newError(code, msg)
}

func (machine *Machine) WriteCSR(r ProcessorCSR, value uint64) error {
	var msg *C.char
	code := C.cm_write_csr(machine.c, C.CM_PROC_CSR(r), C.uint64_t(value), &msg)
	return newError(code, msg)
}

func (machine *Machine) ReadX(i int) (uint64, error) {
	var msg *C.char
	var value C.uint64_t
	code := C.cm_read_x(machine.c, C.int(i), &value, &msg)
	return uint64(value), newError(code, msg)
}

func (machine *Machine) WriteX(i int, value uint64) error {
	var msg *C.char
	code := C.cm_write_x(machine.c, C.int(i), C.uint64_t(value), &msg)
	return newError(code, msg)
}

func (machine *Machine) ReadF(i int) (uint64, error) {
	var msg *C.char
	var value C.uint64_t
	code := C.cm_read_f(machine.c, C.int(i), &value, &msg)
	return uint64(value), newError(code, msg)
}

func (machine *Machine) WriteF(i int, value uint64) error {
	var msg *C.char
	code := C.cm_write_f(machine.c, C.int(i), C.uint64_t(value), &msg)
	return newError(code, msg)
}

func (machine *Machine) ReadIFlagsX() (bool, error) {
	var msg *C.char
	var value C.bool
	code := C.cm_read_iflags_X(machine.c, &value, &msg)
	return bool(value), newError(code, msg)
}

func (machine *Machine) ResetIFlagsX() error {
	var msg *C.char
	code := C.cm_reset_iflags_X(machine.c, &msg)
	return newError(code, msg)
}

func (machine *Machine) SetIFlagsX() error {
	var msg *C.char
	code := C.cm_set_iflags_X(machine.c, &msg)
	return newError(code, msg)
}

func (machine *Machine) ReadIFlagsY() (bool, error) {
	var msg *C.char
	var value C.bool
	code := C.cm_read_iflags_Y(machine.c, &value, &msg)
	return bool(value), newError(code, msg)
}

func (machine *Machine) ResetIFlagsY() error {
	var msg *C.char
	code := C.cm_reset_iflags_Y(machine.c, &msg)
	return newError(code, msg)
}

func (machine *Machine) SetIFlagsY() error {
	var msg *C.char
	code := C.cm_set_iflags_Y(machine.c, &msg)
	return newError(code, msg)
}

func (machine *Machine) ReadIFlagsH() (bool, error) {
	var msg *C.char
	var value C.bool
	code := C.cm_read_iflags_H(machine.c, &value, &msg)
	return bool(value), newError(code, msg)
}

func (machine *Machine) SetIFlagsH() error {
	var msg *C.char
	code := C.cm_set_iflags_H(machine.c, &msg)
	return newError(code, msg)
}

func (machine *Machine) ReadHtifToHostData() (uint64, error) {
	var msg *C.char
	var value C.uint64_t
	code := C.cm_read_htif_tohost_data(machine.c, &value, &msg)
	return uint64(value), newError(code, msg)
}

func (machine *Machine) WriteHtifFromHostData(value uint64) error {
	var msg *C.char
	code := C.cm_write_htif_fromhost_data(machine.c, C.uint64_t(value), &msg)
	return newError(code, msg)
}

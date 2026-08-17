// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package is a binding to the emulator's C API.
// Refer to the machine-c files in the emulator's repository for documentation
// (mainly machine-c-api.h and jsonrpc-machine-c-api.h).
//
//nolint:gocritic // CGo output-parameter wrappers trigger dupSubExpr false positives.
package emulator

// #include <stdlib.h>
// #include <string.h>
// #include "cartesi-machine/cm.h"
//
// static inline cm_error go_cm_send_cmio_response(
//     cm_machine *m,
//     uint16_t reason,
//     const uint8_t *data,
//     uint64_t length,
//     const uint8_t *revert_root_hash)
// {
//     return cm_send_cmio_response(
//         m, reason, data, length, (const cm_hash *) revert_root_hash);
// }
import "C"

import (
	"encoding/json"
	"runtime"
	"sync"
	"unsafe"
)

const HashSize = C.sizeof_cm_hash

// Common type aliases
type Hash = [HashSize]byte

// RevertUarchTail is the emulator-owned JSON array of base64-encoded state
// hashes for the reset-delimited uarch period corresponding to a revert root.
// It remains opaque so collector output can be passed back without decoding and
// re-encoding every hash.
type RevertUarchTail json.RawMessage

// -----------------------------------------------------------------------------
// Machine Methods
// -----------------------------------------------------------------------------

type Machine struct {
	ptr *C.cm_machine
	mu  sync.Mutex
}

func (m *Machine) callCAPI(f func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	f()
}

// destroy
func (m *Machine) Destroy() error {
	var err error
	m.callCAPI(func() {
		err = newError(C.cm_destroy(m.ptr))
	})
	return err
}

// delete
func (m *Machine) Delete() {
	m.callCAPI(func() {
		C.cm_delete(m.ptr)
	})
}

// create
func (m *Machine) Create(config, runtimeConfig, dir string) error {
	var err error
	m.callCAPI(func() {
		var cConfig *C.char
		var cRuntime *C.char
		var cDir *C.char
		if config != "" {
			cConfig = C.CString(config)
			defer C.free(unsafe.Pointer(cConfig))
		}
		if runtimeConfig != "" {
			cRuntime = C.CString(runtimeConfig)
			defer C.free(unsafe.Pointer(cRuntime))
		}
		if dir != "" {
			cDir = C.CString(dir)
			defer C.free(unsafe.Pointer(cDir))
		}
		err = newError(C.cm_create_new(cConfig, cRuntime, cDir, &m.ptr))
	})
	return err
}

// get_default_config
func (m *Machine) GetDefaultConfig() (string, error) {
	var cfg *C.char
	var err error
	var res string

	m.callCAPI(func() {
		// passing 'm->ptr' so it can fetch remote defaults if the API supports that
		err = newError(C.cm_get_default_config(m.ptr, &cfg))
		if err != nil || cfg == nil {
			return
		}
		res = C.GoString(cfg)
		// no need to free 'cfg' here, as it is a static string
	})

	if err != nil {
		return "", err
	}
	return res, nil
}

// get_initial_config
func (m *Machine) GetInitialConfig() (string, error) {
	var cfg *C.char
	var err error
	var res string

	m.callCAPI(func() {
		err = newError(C.cm_get_initial_config(m.ptr, &cfg))
		if err != nil || cfg == nil {
			return
		}
		res = C.GoString(cfg)
		// no need to free 'cfg' here, as it is a static string
	})

	if err != nil {
		return "", err
	}
	return res, nil
}

// get_proof
func (m *Machine) GetProof(address uint64, log2TargetSize, log2RootSize int32) (string, error) {
	var proof *C.char
	var err error
	var res string

	m.callCAPI(func() {
		err = newError(C.cm_get_proof(
			m.ptr,
			C.uint64_t(address),
			C.int32_t(log2TargetSize),
			C.int32_t(log2RootSize),
			&proof,
		))
		if err != nil || proof == nil {
			return
		}
		res = C.GoString(proof)
		// no need to free 'proof' here, as it is a static string
	})

	if err != nil {
		return "", err
	}
	return res, nil
}

// get_reg_address
func (m *Machine) GetRegAddress(reg RegID) (uint64, error) {
	var val C.uint64_t
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_get_reg_address(m.ptr, C.cm_reg(reg), &val))
	})

	if err != nil {
		return 0, err
	}
	return uint64(val), nil
}

// get_root_hash
func (m *Machine) GetRootHash() (Hash, error) {
	var cHash C.cm_hash
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_get_root_hash(m.ptr, &cHash))
	})
	if err != nil {
		return Hash{}, err
	}

	// Zero-copy: reinterpret C array as Go array
	return *(*Hash)(unsafe.Pointer(&cHash)), nil
}

// get_runtime_config
func (m *Machine) GetRuntimeConfig() (string, error) {
	var rcfg *C.char
	var err error
	var str string

	m.callCAPI(func() {
		err = newError(C.cm_get_runtime_config(m.ptr, &rcfg))
		if err != nil || rcfg == nil {
			return
		}
		str = C.GoString(rcfg)
		// no need to free 'rcfg' here, as it is a static string
	})

	if err != nil {
		return "", err
	}
	return str, nil
}

// is_empty
func (m *Machine) IsEmpty() (bool, error) {
	var yes C.bool
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_is_empty(m.ptr, &yes))
	})

	if err != nil {
		return false, err
	}
	return bool(yes), nil
}

// load
func (m *Machine) Load(dir string, runtimeConfig string) error {
	var err error

	m.callCAPI(func() {
		cDir := C.CString(dir)
		defer C.free(unsafe.Pointer(cDir))

		var cRuntime *C.char
		if runtimeConfig != "" {
			cRuntime = C.CString(runtimeConfig)
			defer C.free(unsafe.Pointer(cRuntime))
		}
		err = newError(C.cm_load(m.ptr, cDir, cRuntime, SharingNone))
	})

	return err
}

// read_memory
func (m *Machine) ReadMemory(address, length uint64) ([]byte, error) {
	if length == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, length)
	var err error

	m.callCAPI(func() {
		err = newError(
			C.cm_read_memory(m.ptr, C.uint64_t(address),
				(*C.uchar)(unsafe.Pointer(&buf[0])),
				C.uint64_t(length)),
		)
	})

	if err != nil {
		return nil, err
	}
	return buf, nil
}

// write_memory
func (m *Machine) WriteMemory(address uint64, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	var err error

	m.callCAPI(func() {
		code := C.cm_write_memory(m.ptr,
			C.uint64_t(address),
			(*C.uchar)(unsafe.Pointer(&data[0])),
			C.uint64_t(len(data)))
		err = newError(code)
	})

	return err
}

// read_reg
func (m *Machine) ReadReg(r RegID) (uint64, error) {
	var val C.uint64_t
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_read_reg(m.ptr, C.cm_reg(r), &val))
	})

	if err != nil {
		return 0, err
	}
	return uint64(val), nil
}

// write_reg
func (m *Machine) WriteReg(r RegID, value uint64) error {
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_write_reg(m.ptr, C.cm_reg(r), C.uint64_t(value)))
	})

	return err
}

// receive_cmio_request
func (m *Machine) ReceiveCmioRequest() (uint8, uint16, []byte, error) {
	var cCmd C.uint8_t
	var cReason C.uint16_t
	var cLen C.uint64_t
	var data []byte
	var err error

	m.callCAPI(func() {
		// This is how the emulator own source code does it,
		// first call to get the length then allocate and read the data
		err = newError(C.cm_receive_cmio_request(
			m.ptr,
			&cCmd,
			&cReason,
			(*C.uint8_t)(unsafe.Pointer(nil)),
			&cLen,
		))
		if err != nil {
			return
		}
		data = make([]byte, cLen)
		if cLen != 0 {
			err = newError(C.cm_receive_cmio_request(
				m.ptr,
				(*C.uint8_t)(unsafe.Pointer(nil)),
				(*C.uint16_t)(unsafe.Pointer(nil)),
				(*C.uint8_t)(unsafe.Pointer(&data[0])),
				&cLen,
			))
		}
	})

	if err != nil {
		return 0, 0, nil, err
	}
	return uint8(cCmd), uint16(cReason), data, nil
}

// run
func (m *Machine) Run(mcycleEnd uint64) (BreakReason, error) {
	var br C.cm_break_reason
	var err error

	m.callCAPI(func() {
		err = newError(C.cm_run(m.ptr, C.uint64_t(mcycleEnd), &br))
	})

	if err != nil {
		return BreakReasonFailed, err
	}
	return BreakReason(br), nil
}

// collect_mcycle_root_hashes
func (m *Machine) CollectMCycleRootHashes(
	mcycleEnd,
	log2McyclePeriod,
	mcyclePhase uint64,
	log2BundleMcycleCount int32,
	previousPartialBundle json.RawMessage,
) ([]byte, error) {
	var err error
	var result []byte

	m.callCAPI(func() {
		var cResult *C.char
		var previousPartialBundleC *C.char
		if len(previousPartialBundle) > 0 {
			previousPartialBundleC = C.CString(string(previousPartialBundle))
			defer C.free(unsafe.Pointer(previousPartialBundleC))
		}
		err = newError(C.cm_collect_mcycle_root_hashes(
			m.ptr,
			C.uint64_t(mcycleEnd),
			C.uint64_t(log2McyclePeriod),
			C.uint64_t(mcyclePhase),
			C.int32_t(log2BundleMcycleCount),
			previousPartialBundleC,
			&cResult))
		if err == nil {
			result = []byte(C.GoString(cResult))
		}
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// collect_uarch_cycle_root_hashes
func (m *Machine) CollectUarchCycleRootHashes(
	mcycleEnd uint64,
	log2BundleUarchCycleCount int32,
	revertUarchTail RevertUarchTail,
) ([]byte, error) {
	var err error
	var result []byte

	m.callCAPI(func() {
		var cResult *C.char
		var revertUarchTailC *C.char
		if len(revertUarchTail) > 0 {
			revertUarchTailC = C.CString(string(revertUarchTail))
			defer C.free(unsafe.Pointer(revertUarchTailC))
		}
		err = newError(C.cm_collect_uarch_cycle_root_hashes(
			m.ptr,
			C.uint64_t(mcycleEnd),
			C.int32_t(log2BundleUarchCycleCount),
			revertUarchTailC,
			&cResult))
		if err == nil {
			result = []byte(C.GoString(cResult))
		}
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// send_cmio_response
func (m *Machine) SendCmioResponse(reason uint16, data []byte, revertRootHash *Hash) error {
	var err error

	m.callCAPI(func() {
		var ptrData *C.uint8_t
		sizeData := C.uint64_t(len(data))
		if sizeData > 0 {
			ptrData = (*C.uint8_t)(unsafe.Pointer(&data[0]))
		}
		var ptrHash *C.uint8_t
		if revertRootHash != nil {
			ptrHash = (*C.uint8_t)(unsafe.Pointer(revertRootHash))
		}
		err = newError(C.go_cm_send_cmio_response(
			m.ptr,
			C.uint16_t(reason),
			ptrData,
			sizeData,
			ptrHash,
		))
	})

	return err
}

// set_runtime_config
func (m *Machine) SetRuntimeConfig(runtimeJSON string) error {
	var err error

	m.callCAPI(func() {
		var rc *C.char
		if runtimeJSON != "" {
			rc = C.CString(runtimeJSON)
			defer C.free(unsafe.Pointer(rc))
		}
		err = newError(C.cm_set_runtime_config(m.ptr, rc))
	})

	return err
}

// store
func (m *Machine) Store(directory string) error {
	var err error

	m.callCAPI(func() {
		cDir := C.CString(directory)
		defer C.free(unsafe.Pointer(cDir))
		err = newError(C.cm_store(m.ptr, cDir, SharingAll))
	})

	return err
}

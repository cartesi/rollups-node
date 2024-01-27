// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package libcmt

// #cgo LDFLAGS: -lcmt
// #include <stdlib.h>
// #include <string.h>
// #include "libcmt/rollup.h"
// #include "libcmt/io.h"
import "C"
import (
	"fmt"
	"unsafe"
)

type RequestType = uint8

const AdvanceStateRequest RequestType = C.CMT_IO_REASON_ADVANCE
const InspectStateRequest RequestType = C.CMT_IO_REASON_INSPECT

type Finish struct {
	AcceptPreviousRequest    bool
	NextRequestType          RequestType
	NextRequestPayloadLength uint32
}

type Input struct {
	Sender         [20]byte
	BlockNumber    uint64
	BlockTimestamp uint64
	Index          uint64
	Data           []byte
}

type Query struct {
	Data []byte
}

// ------------------------------------------------------------------------------------------------

type Rollup struct {
	inner *C.cmt_rollup_t
}

func NewRollup() (*Rollup, error) {
	var rollup C.cmt_rollup_t
	errno := C.cmt_rollup_init(&rollup)
	return &Rollup{inner: &rollup}, toError(errno)
}

func (rollup *Rollup) Destroy() {
	C.cmt_rollup_fini(rollup.inner)
}

func (rollup *Rollup) Finish(accept bool) (*Finish, error) {
	finish := C.cmt_rollup_finish_t{
		accept_previous_request: C.bool(accept),
	}

	errno := C.cmt_rollup_finish(rollup.inner, &finish)
	if err := toError(errno); err != nil {
		return nil, err
	}

	return &Finish{
		AcceptPreviousRequest:    bool(finish.accept_previous_request),
		NextRequestType:          RequestType(finish.next_request_type),
		NextRequestPayloadLength: uint32(finish.next_request_payload_length),
	}, nil
}

func (rollup *Rollup) EmitVoucher(address [20]byte, value []byte, voucher []byte) error {
	addressLength, addressData := C.uint(20), C.CBytes(address[:])
	defer C.free(addressData)

	valueLength, valueData := C.uint(len(value)), C.CBytes(value)
	defer C.free(valueData)

	voucherLength, voucherData := C.uint(len(voucher)), C.CBytes(voucher)
	defer C.free(voucherData)

	return toError(C.cmt_rollup_emit_voucher(rollup.inner,
		addressLength, addressData,
		valueLength, valueData,
		voucherLength, voucherData,
	))
}

func (rollup *Rollup) EmitNotice(notice []byte) error {
	length, data := C.uint(len(notice)), C.CBytes(notice)
	defer C.free(data)
	return toError(C.cmt_rollup_emit_notice(rollup.inner, length, data))
}

func (rollup *Rollup) EmitReport(report []byte) error {
	length, data := C.uint(len(report)), C.CBytes(report)
	defer C.free(data)
	return toError(C.cmt_rollup_emit_report(rollup.inner, length, data))
}

func (rollup *Rollup) EmitException(exception []byte) error {
	length, data := C.uint(len(exception)), C.CBytes(exception)
	defer C.free(data)
	return toError(C.cmt_rollup_emit_exception(rollup.inner, length, data))
}

func (rollup *Rollup) ReadAdvanceState() (*Input, error) {
	var advance C.cmt_rollup_advance_t
	errno := C.cmt_rollup_read_advance_state(rollup.inner, &advance)
	if err := toError(errno); err != nil {
		return nil, err
	}
	// TODO: should I free inner.data?

	var sender [20]byte
	for i, v := range advance.sender {
		sender[i] = byte(v)
	}

	return &Input{
		Data:           C.GoBytes(advance.data, C.int(advance.length)),
		Sender:         sender,
		BlockNumber:    uint64(advance.block_number),
		BlockTimestamp: uint64(advance.block_timestamp),
		Index:          uint64(advance.index),
	}, nil
}

func (rollup *Rollup) ReadInspectState() (*Query, error) {
	var query C.cmt_rollup_inspect_t
	errno := C.cmt_rollup_read_inspect_state(rollup.inner, &query)
	if err := toError(errno); err != nil {
		return nil, err
	}
	// TODO: should I free query.data?

	return &Query{Data: C.GoBytes(query.data, C.int(query.length))}, nil
}

func (rollup *Rollup) LoadMerkle(path string) error {
	s := C.CString(path)
	defer C.free(unsafe.Pointer(s))
	return toError(C.cmt_rollup_load_merkle(rollup.inner, s))
}

func (rollup *Rollup) SaveMerkle(path string) error {
	s := C.CString(path)
	defer C.free(unsafe.Pointer(s))
	return toError(C.cmt_rollup_save_merkle(rollup.inner, s))
}

// ------------------------------------------------------------------------------------------------

func toError(errno C.int) error {
	if errno < 0 {
		s := C.strerror(-errno)
		defer C.free(unsafe.Pointer(s))
		return fmt.Errorf("%s (%d)", C.GoString(s), errno)
	} else {
		return nil
	}
}

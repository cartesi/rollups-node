// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package libcmt provides bindings for the libcmt C library.
// It facilitates the development of applications meant to run in the cartesi-machine
// by handling IO and the communication protocol with the machine emulator.
package libcmt

// #cgo CFLAGS: -I/usr/riscv64-linux-gnu/include
// #cgo LDFLAGS: -L/usr/riscv64-linux-gnu/lib -lcmt
// #include <string.h>
// #include "libcmt/abi.h"
// #include "libcmt/io.h"
// #include "libcmt/rollup.h"
import "C"
import (
	"errors"
	"fmt"
)

type (
	RequestType uint8
	Address     = [20]byte
)

var (
	ErrClosed = errors.New("rollup already closed")

	ErrFinish           = errors.New("failed to finish")
	ErrReadAdvanceState = errors.New("failed to read the advance's state")
	ErrReadInspectState = errors.New("failed to read the inspect's state")
	ErrEmitVoucher      = errors.New("failed to emit voucher")
	ErrEmitNotice       = errors.New("failed to emit notice")
	ErrEmitReport       = errors.New("failed to emit report")
	ErrEmitException    = errors.New("failed to emit exception")
)

const (
	AdvanceState RequestType = C.HTIF_YIELD_REASON_ADVANCE
	InspectState RequestType = C.HTIF_YIELD_REASON_INSPECT
)

type Input struct {
	ChainId        uint64
	AppContract    Address
	Sender         Address
	BlockNumber    uint64
	BlockTimestamp uint64
	Index          uint64
	Data           []byte
}

type Query struct {
	Data []byte
}

type Rollup struct{ c *C.cmt_rollup_t }

// NewRollup returns a new [Rollup].
func NewRollup() (*Rollup, error) {
	var c C.cmt_rollup_t
	errno := C.cmt_rollup_init(&c)
	rollup := &Rollup{c: &c}
	return rollup, toError(errno)
}

// Close closes the rollup, rendering it unusable.
// Close will return an error if it has already been called.
func (rollup *Rollup) Close() error {
	if rollup.c == nil {
		return ErrClosed
	}
	C.cmt_rollup_fini(rollup.c)
	rollup.c = nil
	return nil
}

// Finish accepts or rejects the previous advance/inspect request.
// It then waits for the next request and returns its type.
func (rollup *Rollup) Finish(accept bool) (RequestType, error) {
	finish := C.cmt_rollup_finish_t{accept_previous_request: C.bool(accept)}
	errno := C.cmt_rollup_finish(rollup.c, &finish)
	if err := toError(errno); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrFinish, err)
	}
	return RequestType(finish.next_request_type), nil
}

// ReadAdvanceState returns the [Input] from an advance-state request.
func (rollup *Rollup) ReadAdvanceState() (Input, error) {
	var advance C.cmt_rollup_advance_t
	errno := C.cmt_rollup_read_advance_state(rollup.c, &advance)
	if err := toError(errno); err != nil {
		return Input{}, fmt.Errorf("%w: %w", ErrReadAdvanceState, err)
	}
	return Input{
		ChainId:        uint64(advance.chain_id),
		AppContract:    toAddress(advance.app_contract),
		Sender:         toAddress(advance.msg_sender),
		BlockNumber:    uint64(advance.block_number),
		BlockTimestamp: uint64(advance.block_timestamp),
		Index:          uint64(advance.index),
		Data:           C.GoBytes(advance.payload, C.int(advance.payload_length)),
	}, nil
}

// ReadInspectState returns the [Query] from an inspect-state request.
func (rollup *Rollup) ReadInspectState() (Query, error) {
	var inspect C.cmt_rollup_inspect_t
	errno := C.cmt_rollup_read_inspect_state(rollup.c, &inspect)
	if err := toError(errno); err != nil {
		return Query{}, fmt.Errorf("%w: %w", ErrReadInspectState, err)
	}
	return Query{Data: C.GoBytes(inspect.payload, C.int(inspect.payload_length))}, nil
}

// EmitVoucher emits a voucher and returns its index.
func (rollup *Rollup) EmitVoucher(address Address, value []byte, voucher []byte) (uint64, error) {
	addressData := C.CBytes(address[:])
	// defer C.free(addressData)

	valueLength, valueData := C.uint(len(value)), C.CBytes(value)
	// defer C.free(valueData)

	voucherLength, voucherData := C.uint(len(voucher)), C.CBytes(voucher)
	// defer C.free(voucherData)

	var index C.uint64_t
	err := toError(C.cmt_rollup_emit_voucher(rollup.c,
		C.CMT_ADDRESS_LENGTH, addressData,
		valueLength, valueData,
		voucherLength, voucherData,
		&index,
	))

	return uint64(index), fmt.Errorf("%w: %w", ErrEmitVoucher, err)
}

// EmitNotice emits a notice and returns its index.
func (rollup *Rollup) EmitNotice(notice []byte) (uint64, error) {
	length, data := C.uint(len(notice)), C.CBytes(notice)
	// defer C.free(data)
	var index C.uint64_t
	err := toError(C.cmt_rollup_emit_notice(rollup.c, length, data, &index))
	return uint64(index), fmt.Errorf("%w: %w", ErrEmitNotice, err)
}

// EmitReport emits a report.
func (rollup *Rollup) EmitReport(report []byte) error {
	length, data := C.uint(len(report)), C.CBytes(report)
	// defer C.free(data)
	err := toError(C.cmt_rollup_emit_report(rollup.c, length, data))
	return fmt.Errorf("%w: %w", ErrEmitReport, err)

}

// EmitException emits an exception.
func (rollup *Rollup) EmitException(exception []byte) error {
	length, data := C.uint(len(exception)), C.CBytes(exception)
	// defer C.free(data)
	err := toError(C.cmt_rollup_emit_exception(rollup.c, length, data))
	return fmt.Errorf("%w: %w", ErrEmitException, err)
}

// ------------------------------------------------------------------------------------------------

func toError(errno C.int) error {
	if errno < 0 {
		s := C.strerror(-errno)
		// defer C.free(unsafe.Pointer(s))
		return fmt.Errorf("%s (%d)", C.GoString(s), errno)
	} else {
		return nil
	}
}

func toAddress(c [C.CMT_ADDRESS_LENGTH]C.uint8_t) Address {
	var address Address
	for i, v := range c {
		address[i] = byte(v)
	}
	return address
}

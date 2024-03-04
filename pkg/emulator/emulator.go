// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Cartesi Machine C API wrapper

package emulator

/*
#cgo LDFLAGS: -lcartesi -lcartesi_jsonrpc
#include "machine-c-api.h"
#include "jsonrpc-machine-c-api.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/hex"
	"fmt"
	"unsafe"
)

type ErrorCode int32

const (
	ErrorCodeOk                   ErrorCode = C.CM_ERROR_OK
	ErrorCodeInvalidArgument      ErrorCode = C.CM_ERROR_INVALID_ARGUMENT
	ErrorCodeDomainError          ErrorCode = C.CM_ERROR_DOMAIN_ERROR
	ErrorCodeLengthError          ErrorCode = C.CM_ERROR_LENGTH_ERROR
	ErrorCodeOutOfRange           ErrorCode = C.CM_ERROR_OUT_OF_RANGE
	ErrorCodeLogicError           ErrorCode = C.CM_ERROR_LOGIC_ERROR
	ErrorCodeBadOptionalAccess    ErrorCode = C.CM_ERROR_BAD_OPTIONAL_ACCESS
	ErrorCodeRuntimeError         ErrorCode = C.CM_ERROR_RUNTIME_ERROR
	ErrorCodeRangeError           ErrorCode = C.CM_ERROR_RANGE_ERROR
	ErrorCodeOverflowError        ErrorCode = C.CM_ERROR_OVERFLOW_ERROR
	ErrorCodeUnderflowError       ErrorCode = C.CM_ERROR_UNDERFLOW_ERROR
	ErrorCodeRegexError           ErrorCode = C.CM_ERROR_REGEX_ERROR
	ErrorCodeSystemIosBaseFailure ErrorCode = C.CM_ERROR_SYSTEM_IOS_BASE_FAILURE
	ErrorCodeFilesystemError      ErrorCode = C.CM_ERROR_FILESYSTEM_ERROR
	ErrorCodeAtomicTxError        ErrorCode = C.CM_ERROR_ATOMIC_TX_ERROR
	ErrorCodeNonexistingLocalTime ErrorCode = C.CM_ERROR_NONEXISTING_LOCAL_TIME
	ErrorCodeAmbiguousLocalTime   ErrorCode = C.CM_ERROR_AMBIGUOUS_LOCAL_TIME
	ErrorCodeFormatError          ErrorCode = C.CM_ERROR_FORMAT_ERROR
	ErrorCodeBadTypeid            ErrorCode = C.CM_ERROR_BAD_TYPEID
	ErrorCodeBadCast              ErrorCode = C.CM_ERROR_BAD_CAST
	ErrorCodeBadAnyCast           ErrorCode = C.CM_ERROR_BAD_ANY_CAST
	ErrorCodeBadWeakPtr           ErrorCode = C.CM_ERROR_BAD_WEAK_PTR
	ErrorCodeBadFunctionCall      ErrorCode = C.CM_ERROR_BAD_FUNCTION_CALL
	ErrorCodeBadAlloc             ErrorCode = C.CM_ERROR_BAD_ALLOC
	ErrorCodeBadArrayNewLength    ErrorCode = C.CM_ERROR_BAD_ARRAY_NEW_LENGTH
	ErrorCodeBadException         ErrorCode = C.CM_ERROR_BAD_EXCEPTION
	ErrorCodeBadVariantAccess     ErrorCode = C.CM_ERROR_BAD_VARIANT_ACCESS
	ErrorCodeException            ErrorCode = C.CM_ERROR_EXCEPTION
	ErrorCodeUnknown              ErrorCode = C.CM_ERROR_UNKNOWN
)

func isFailure(cerr C.int) bool {
	return ErrorCode(cerr) != ErrorCodeOk
}

type Error struct {
	Code ErrorCode
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("cartesi machine error %d (%s)", e.Code, e.Msg)
}

func newError(code C.int, message *C.char) error {
	defer C.cm_delete_cstring(message)
	if code != C.CM_ERROR_OK {
		return &Error{Code: ErrorCode(code), Msg: C.GoString(message)}
	}
	return nil
}

type BreakReason int32

const (
	BreakReasonFailed               BreakReason = C.CM_BREAK_REASON_FAILED
	BreakReasonHalted               BreakReason = C.CM_BREAK_REASON_HALTED
	BreakReasonYieldedManually      BreakReason = C.CM_BREAK_REASON_YIELDED_MANUALLY
	BreakReasonYieldedAutomatically BreakReason = C.CM_BREAK_REASON_YIELDED_AUTOMATICALLY
	BreakReasonYieldedSoftly        BreakReason = C.CM_BREAK_REASON_YIELDED_SOFTLY
	BreakReasonReachedTargetMcycle  BreakReason = C.CM_BREAK_REASON_REACHED_TARGET_MCYCLE
)

type ProcessorCSR int32

const (
	ProcCsrPc            ProcessorCSR = C.CM_PROC_PC
	ProcCsrFcsr          ProcessorCSR = C.CM_PROC_FCSR
	ProcCsrMvendorid     ProcessorCSR = C.CM_PROC_MVENDORID
	ProcCsrMarchid       ProcessorCSR = C.CM_PROC_MARCHID
	ProcCsrMimpid        ProcessorCSR = C.CM_PROC_MIMPID
	ProcCsrMcycle        ProcessorCSR = C.CM_PROC_MCYCLE
	ProcCsrIcycleinstret ProcessorCSR = C.CM_PROC_ICYCLEINSTRET
	ProcCsrMstatus       ProcessorCSR = C.CM_PROC_MSTATUS
	ProcCsrMtvec         ProcessorCSR = C.CM_PROC_MTVEC
	ProcCsrMscratch      ProcessorCSR = C.CM_PROC_MSCRATCH
	ProcCsrMepc          ProcessorCSR = C.CM_PROC_MEPC
	ProcCsrMcause        ProcessorCSR = C.CM_PROC_MCAUSE
	ProcCsrMtval         ProcessorCSR = C.CM_PROC_MTVAL
	ProcCsrMisa          ProcessorCSR = C.CM_PROC_MISA
	ProcCsrMie           ProcessorCSR = C.CM_PROC_MIE
	ProcCsrMip           ProcessorCSR = C.CM_PROC_MIP
	ProcCsrMedeleg       ProcessorCSR = C.CM_PROC_MEDELEG
	ProcCsrMideleg       ProcessorCSR = C.CM_PROC_MIDELEG
	ProcCsrMcounteren    ProcessorCSR = C.CM_PROC_MCOUNTEREN
	ProcCsrMenvcfg       ProcessorCSR = C.CM_PROC_MENVCFG
	ProcCsrStvec         ProcessorCSR = C.CM_PROC_STVEC
	ProcCsrSscratch      ProcessorCSR = C.CM_PROC_SSCRATCH
	ProcCsrSepc          ProcessorCSR = C.CM_PROC_SEPC
	ProcCsrScause        ProcessorCSR = C.CM_PROC_SCAUSE
	ProcCsrStval         ProcessorCSR = C.CM_PROC_STVAL
	ProcCsrSatp          ProcessorCSR = C.CM_PROC_SATP
	ProcCsrScounteren    ProcessorCSR = C.CM_PROC_SCOUNTEREN
	ProcCsrSenvcfg       ProcessorCSR = C.CM_PROC_SENVCFG
	ProcCsrIlrsc         ProcessorCSR = C.CM_PROC_ILRSC
	ProcCsrIflags        ProcessorCSR = C.CM_PROC_IFLAGS
	ProcCsrClintMtimecmp ProcessorCSR = C.CM_PROC_CLINT_MTIMECMP
	ProcCsrHtifTohost    ProcessorCSR = C.CM_PROC_HTIF_TOHOST
	ProcCsrHtifFromhost  ProcessorCSR = C.CM_PROC_HTIF_FROMHOST
	ProcCsrHtifIhalt     ProcessorCSR = C.CM_PROC_HTIF_IHALT
	ProcCsrHtifIconsole  ProcessorCSR = C.CM_PROC_HTIF_ICONSOLE
	ProcCsrHtifIyield    ProcessorCSR = C.CM_PROC_HTIF_IYIELD
	ProcCsrUarchPc       ProcessorCSR = C.CM_PROC_UARCH_PC
	ProcCsrUarchCycle    ProcessorCSR = C.CM_PROC_UARCH_CYCLE
	ProcCsrUarchHaltFlag ProcessorCSR = C.CM_PROC_UARCH_HALT_FLAG
)

type MachineRuntimeConfig struct {
	Concurrency       ConcurrencyRuntimeConfig
	Htif              HtifRuntimeConfig
	SkipRootHashCheck bool
	SkipVersionCheck  bool
	SoftYield         bool
}

type HtifRuntimeConfig struct {
	NoConsolePutchar bool
}

type ConcurrencyRuntimeConfig struct {
	UpdateMerkleTree uint64
}

type MachineConfig struct {
	Processor  ProcessorConfig
	Ram        RamConfig
	Dtb        DtbConfig
	FlashDrive []MemoryRangeConfig
	Tlb        TlbConfig
	Clint      ClintConfig
	Htif       HtifConfig
	Cmio       CmioConfig
	Uarch      UarchConfig
}

type ProcessorConfig struct {
	X             [32]uint64
	F             [32]uint64
	Pc            uint64
	Fcsr          uint64
	Mvendorid     uint64
	Marchid       uint64
	Mimpid        uint64
	Mcycle        uint64
	Icycleinstret uint64
	Mstatus       uint64
	Mtvec         uint64
	Mscratch      uint64
	Mepc          uint64
	Mcause        uint64
	Mtval         uint64
	Misa          uint64
	Mie           uint64
	Mip           uint64
	Medeleg       uint64
	Mideleg       uint64
	Mcounteren    uint64
	Menvcfg       uint64
	Stvec         uint64
	Sscratch      uint64
	Sepc          uint64
	Scause        uint64
	Stval         uint64
	Satp          uint64
	Scounteren    uint64
	Senvcfg       uint64
	Ilrsc         uint64
	Iflags        uint64
}

type RamConfig struct {
	Length        uint64
	ImageFilename string
}

type DtbConfig struct {
	Bootargs      string
	Init          string
	Entrypoint    string
	ImageFilename string
}

type MemoryRangeConfig struct {
	Start         uint64
	Length        uint64
	Shared        bool
	ImageFilename string
}

type TlbConfig struct {
	ImageFilename string
}

type ClintConfig struct {
	Mtimecmp uint64
}

type HtifConfig struct {
	Fromhost       uint64
	Tohost         uint64
	ConsoleGetchar bool
	YieldManual    bool
	YieldAutomatic bool
}

type CmioConfig struct {
	HsaValue bool
	RxBuffer MemoryRangeConfig
	TxBuffer MemoryRangeConfig
}

type UarchRamConfig struct {
	ImageFilename string
}

type UarchProcessorConfig struct {
	X        [32]uint64
	Pc       uint64
	Cycle    uint64
	HaltFlag bool
}

type UarchConfig struct {
	Processor UarchProcessorConfig
	Ram       UarchRamConfig
}

////////////////////////////////////////
// Helpers and utils
////////////////////////////////////////

type ourMemoryRangeConfig struct {
	cref *C.cm_memory_range_config
}

func (config *MemoryRangeConfig) makeCRef() (ref *ourMemoryRangeConfig) {
	ref = &ourMemoryRangeConfig{
		cref: (*C.cm_memory_range_config)(C.calloc(1, C.sizeof_cm_memory_range_config)),
	}
	c := ref.cref
	c.start = (C.uint64_t)(config.Start)
	c.length = (C.uint64_t)(config.Length)
	c.shared = (C.bool)(config.Shared)
	c.image_filename = makeCString(&config.ImageFilename)
	return ref
}

func (configRef *ourMemoryRangeConfig) free() {
	if configRef == nil || configRef.cref == nil {
		return
	}
	C.free(unsafe.Pointer(configRef.cref.image_filename))
	C.free(unsafe.Pointer(configRef.cref))
	configRef.cref = nil
}

// cm_machine_runtime_config allocated by us
type ourMachineRuntimeConfigCRef struct {
	cref *C.cm_machine_runtime_config
}

func (config *MachineRuntimeConfig) makeCRef() (ref *ourMachineRuntimeConfigCRef) {
	ref = &ourMachineRuntimeConfigCRef{
		cref: (*C.cm_machine_runtime_config)(C.calloc(1, C.sizeof_cm_machine_runtime_config)),
	}
	cRuntime := ref.cref
	cRuntime.skip_root_hash_check = (C.bool)(config.SkipRootHashCheck)
	cRuntime.skip_version_check = (C.bool)(config.SkipVersionCheck)
	cRuntime.soft_yield = (C.bool)(config.SoftYield)

	cHtif := &ref.cref.htif
	htif := &config.Htif
	cHtif.no_console_putchar = (C.bool)(htif.NoConsolePutchar)

	cConcurrency := &ref.cref.concurrency
	concurrency := &config.Concurrency
	cConcurrency.update_merkle_tree = (C.uint64_t)(concurrency.UpdateMerkleTree)

	return ref
}

func (configRef *ourMachineRuntimeConfigCRef) free() {
	if configRef == nil || configRef.cref == nil {
		return
	}
	C.free(unsafe.Pointer(configRef.cref))
	configRef.cref = nil
}

// cm_machine_config allocated by us
type ourMachineConfigCRef struct {
	cref *C.cm_machine_config
}

func (config *MachineConfig) makeCRef() (ref *ourMachineConfigCRef) {
	ref = &ourMachineConfigCRef{
		cref: (*C.cm_machine_config)(C.calloc(1, C.sizeof_cm_machine_config)),
	}
	// Processor
	cProcessor := &ref.cref.processor
	processor := &config.Processor
	for i := 0; i < 31; i++ {
		cProcessor.x[i+1] = (C.uint64_t)(processor.X[i])
	}
	for i := 0; i < 31; i++ {
		cProcessor.f[i+1] = (C.uint64_t)(processor.F[i])
	}
	cProcessor.pc = (C.uint64_t)(processor.Pc)
	cProcessor.fcsr = (C.uint64_t)(processor.Fcsr)
	cProcessor.mvendorid = (C.uint64_t)(processor.Mvendorid)
	cProcessor.marchid = (C.uint64_t)(processor.Marchid)
	cProcessor.mimpid = (C.uint64_t)(processor.Mimpid)
	cProcessor.mcycle = (C.uint64_t)(processor.Mcycle)
	cProcessor.icycleinstret = (C.uint64_t)(processor.Icycleinstret)
	cProcessor.mstatus = (C.uint64_t)(processor.Mstatus)
	cProcessor.mtvec = (C.uint64_t)(processor.Mtvec)
	cProcessor.mscratch = (C.uint64_t)(processor.Mscratch)
	cProcessor.mepc = (C.uint64_t)(processor.Mepc)
	cProcessor.mcause = (C.uint64_t)(processor.Mcause)
	cProcessor.mtval = (C.uint64_t)(processor.Mtval)
	cProcessor.misa = (C.uint64_t)(processor.Misa)
	cProcessor.mie = (C.uint64_t)(processor.Mie)
	cProcessor.mip = (C.uint64_t)(processor.Mip)
	cProcessor.medeleg = (C.uint64_t)(processor.Medeleg)
	cProcessor.mideleg = (C.uint64_t)(processor.Mideleg)
	cProcessor.mcounteren = (C.uint64_t)(processor.Mcounteren)
	cProcessor.menvcfg = (C.uint64_t)(processor.Menvcfg)
	cProcessor.stvec = (C.uint64_t)(processor.Stvec)
	cProcessor.sscratch = (C.uint64_t)(processor.Sscratch)
	cProcessor.sepc = (C.uint64_t)(processor.Sepc)
	cProcessor.scause = (C.uint64_t)(processor.Scause)
	cProcessor.stval = (C.uint64_t)(processor.Stval)
	cProcessor.satp = (C.uint64_t)(processor.Satp)
	cProcessor.scounteren = (C.uint64_t)(processor.Scounteren)
	cProcessor.senvcfg = (C.uint64_t)(processor.Senvcfg)
	cProcessor.ilrsc = (C.uint64_t)(processor.Ilrsc)
	cProcessor.iflags = (C.uint64_t)(processor.Iflags)

	cRam := &ref.cref.ram
	ram := &config.Ram
	cRam.length = (C.uint64_t)(ram.Length)
	cRam.image_filename = makeCString(&ram.ImageFilename)

	cDtb := &ref.cref.dtb
	dtb := &config.Dtb
	cDtb.bootargs = makeCString(&dtb.Bootargs)
	cDtb.init = makeCString(&dtb.Init)
	cDtb.entrypoint = makeCString(&dtb.Entrypoint)
	cDtb.image_filename = makeCString(&dtb.ImageFilename)

	// flash
	cFlashDrive := &ref.cref.flash_drive
	flashDrive := &config.FlashDrive
	cFlashDrive.count = (C.ulong)(len(*flashDrive))
	cFlashDrive.entry = (*C.cm_memory_range_config)(C.calloc((C.ulong)(len(*flashDrive)), C.sizeof_cm_memory_range_config))
	for i, v := range *flashDrive {
		offset := C.sizeof_cm_memory_range_config * i
		addr := unsafe.Pointer(uintptr(unsafe.Pointer(cFlashDrive.entry)) + uintptr(offset))
		mr := (*C.cm_memory_range_config)(addr)
		mr.start = (C.uint64_t)(v.Start)
		mr.length = (C.uint64_t)(v.Length)
		mr.shared = (C.bool)(v.Shared)
		mr.image_filename = makeCString(&v.ImageFilename)
	}

	cTlb := &ref.cref.tlb
	tlb := &config.Tlb
	cTlb.image_filename = makeCString(&tlb.ImageFilename)

	cClint := &ref.cref.clint
	clint := &config.Clint
	cClint.mtimecmp = (C.uint64_t)(clint.Mtimecmp)

	cHtif := &ref.cref.htif
	htif := &config.Htif
	cHtif.tohost = (C.uint64_t)(htif.Tohost)
	cHtif.fromhost = (C.uint64_t)(htif.Fromhost)
	cHtif.console_getchar = (C.bool)(htif.ConsoleGetchar)
	cHtif.yield_manual = (C.bool)(htif.YieldManual)
	cHtif.yield_automatic = (C.bool)(htif.YieldAutomatic)

	cCmio := &ref.cref.cmio
	cmio := &config.Cmio
	cCmio.has_value = (C.bool)(cmio.HsaValue)
	cCmio.rx_buffer.start = (C.uint64_t)(cmio.RxBuffer.Start)
	cCmio.rx_buffer.length = (C.uint64_t)(cmio.RxBuffer.Length)
	cCmio.rx_buffer.shared = (C.bool)(cmio.RxBuffer.Shared)
	cCmio.rx_buffer.image_filename = makeCString(&cmio.RxBuffer.ImageFilename)
	cCmio.tx_buffer.start = (C.uint64_t)(cmio.TxBuffer.Start)
	cCmio.tx_buffer.length = (C.uint64_t)(cmio.TxBuffer.Length)
	cCmio.tx_buffer.shared = (C.bool)(cmio.TxBuffer.Shared)
	cCmio.tx_buffer.image_filename = makeCString(&cmio.TxBuffer.ImageFilename)

	cUarch := &ref.cref.uarch
	uarch := &config.Uarch

	cUarchProcessor := &cUarch.processor
	uarchProcessor := &uarch.Processor
	for i := 0; i < 32; i++ {
		cUarchProcessor.x[i] = (C.uint64_t)(uarchProcessor.X[i])
	}
	cUarchProcessor.pc = (C.uint64_t)(uarchProcessor.Pc)
	cUarchProcessor.cycle = (C.uint64_t)(uarchProcessor.Cycle)
	cUarchProcessor.halt_flag = (C.bool)(uarchProcessor.HaltFlag)

	cUarchRam := &cUarch.ram
	uarchRam := &uarch.Ram
	cUarchRam.image_filename = makeCString(&uarchRam.ImageFilename)

	return ref
}

func (configCRef *ourMachineConfigCRef) free() {
	if configCRef == nil || configCRef.cref == nil {
		return
	}
	C.free(unsafe.Pointer(configCRef.cref.ram.image_filename))
	C.free(unsafe.Pointer(configCRef.cref.dtb.bootargs))
	C.free(unsafe.Pointer(configCRef.cref.dtb.init))
	C.free(unsafe.Pointer(configCRef.cref.dtb.entrypoint))
	C.free(unsafe.Pointer(configCRef.cref.dtb.image_filename))
	C.free(unsafe.Pointer(configCRef.cref.flash_drive.entry))

	C.free(unsafe.Pointer(configCRef.cref.cmio.rx_buffer.image_filename))
	C.free(unsafe.Pointer(configCRef.cref.cmio.tx_buffer.image_filename))
	C.free(unsafe.Pointer(configCRef.cref.uarch.ram.image_filename))
	C.free(unsafe.Pointer(configCRef.cref))
	configCRef.cref = nil
}

// cm_machine_config allocated by the emulator
type theirMachineConfigCRef struct {
	cref *C.cm_machine_config
}

func (configCRef *theirMachineConfigCRef) free() {
	if configCRef != nil && configCRef.cref != nil {
		C.cm_delete_machine_config(configCRef.cref)
		configCRef.cref = nil
	}
}

func (configCRef *theirMachineConfigCRef) makeGoRef() (cfg *MachineConfig) {
	cfg = &MachineConfig{}
	c := configCRef.cref
	// Processor
	processor := &cfg.Processor
	for i := 0; i < 30; i++ {
		processor.X[i] = (uint64)(c.processor.x[i+1])
	}
	for i := 0; i < 31; i++ {
		processor.F[i] = (uint64)(c.processor.f[i+1])
	}
	processor.Pc = (uint64)(c.processor.pc)
	processor.Fcsr = (uint64)(c.processor.fcsr)
	processor.Mvendorid = (uint64)(c.processor.mvendorid)
	processor.Marchid = (uint64)(c.processor.marchid)
	processor.Mimpid = (uint64)(c.processor.mimpid)
	processor.Mcycle = (uint64)(c.processor.mcycle)
	processor.Icycleinstret = (uint64)(c.processor.icycleinstret)
	processor.Mstatus = (uint64)(c.processor.mstatus)
	processor.Mtvec = (uint64)(c.processor.mtvec)
	processor.Mscratch = (uint64)(c.processor.mscratch)
	processor.Mepc = (uint64)(c.processor.mepc)
	processor.Mcause = (uint64)(c.processor.mcause)
	processor.Mtval = (uint64)(c.processor.mtval)
	processor.Misa = (uint64)(c.processor.misa)
	processor.Mie = (uint64)(c.processor.mie)
	processor.Mip = (uint64)(c.processor.mip)
	processor.Medeleg = (uint64)(c.processor.medeleg)
	processor.Mideleg = (uint64)(c.processor.mideleg)
	processor.Mcounteren = (uint64)(c.processor.mcounteren)
	processor.Menvcfg = (uint64)(c.processor.menvcfg)
	processor.Stvec = (uint64)(c.processor.stvec)
	processor.Sscratch = (uint64)(c.processor.sscratch)
	processor.Sepc = (uint64)(c.processor.sepc)
	processor.Scause = (uint64)(c.processor.scause)
	processor.Stval = (uint64)(c.processor.stval)
	processor.Satp = (uint64)(c.processor.satp)
	processor.Scounteren = (uint64)(c.processor.scounteren)
	processor.Senvcfg = (uint64)(c.processor.senvcfg)
	processor.Ilrsc = (uint64)(c.processor.ilrsc)
	processor.Iflags = (uint64)(c.processor.iflags)

	// Ram
	ram := &cfg.Ram
	ram.Length = (uint64)(c.ram.length)
	ram.ImageFilename = C.GoString(c.ram.image_filename)

	// Dtb
	dtb := &cfg.Dtb
	dtb.Bootargs = C.GoString(c.dtb.bootargs)
	dtb.Init = C.GoString(c.dtb.init)
	dtb.Entrypoint = C.GoString(c.dtb.entrypoint)
	dtb.ImageFilename = C.GoString(c.dtb.image_filename)

	// FlashDrive
	//flashDrive := &cfg.FlashDrive
	for i := 0; i < int(c.flash_drive.count); i++ {
		offset := C.sizeof_cm_memory_range_config * i
		addr := unsafe.Pointer(uintptr(unsafe.Pointer(c.flash_drive.entry)) + uintptr(offset))
		mr := (*C.cm_memory_range_config)(addr)
		cfg.FlashDrive = append(cfg.FlashDrive, MemoryRangeConfig{
			Start:         (uint64)(mr.start),
			Length:        (uint64)(mr.length),
			Shared:        (bool)(mr.shared),
			ImageFilename: C.GoString(mr.image_filename),
		})
	}

	// Tlb
	tlb := &cfg.Tlb
	tlb.ImageFilename = C.GoString(c.tlb.image_filename)

	// Clint
	clint := &cfg.Clint
	clint.Mtimecmp = (uint64)(c.clint.mtimecmp)

	// Htif
	htif := &cfg.Htif
	htif.Tohost = (uint64)(c.htif.tohost)
	htif.Fromhost = (uint64)(c.htif.fromhost)
	htif.ConsoleGetchar = (bool)(c.htif.console_getchar)
	htif.YieldManual = (bool)(c.htif.yield_manual)
	htif.YieldAutomatic = (bool)(c.htif.yield_automatic)

	// CMIO
	cmio := &cfg.Cmio
	cmio.HsaValue = (bool)(c.cmio.has_value)
	cmio.RxBuffer = MemoryRangeConfig{
		Start:         (uint64)(c.cmio.rx_buffer.start),
		Length:        (uint64)(c.cmio.rx_buffer.length),
		Shared:        (bool)(c.cmio.rx_buffer.shared),
		ImageFilename: C.GoString(c.cmio.rx_buffer.image_filename),
	}
	cmio.TxBuffer = MemoryRangeConfig{
		Start:         (uint64)(c.cmio.tx_buffer.start),
		Length:        (uint64)(c.cmio.tx_buffer.length),
		Shared:        (bool)(c.cmio.tx_buffer.shared),
		ImageFilename: C.GoString(c.cmio.tx_buffer.image_filename),
	}

	// Uarch
	uarch := &cfg.Uarch
	uarchProcessor := &uarch.Processor
	for i := 0; i < 32; i++ {
		uarchProcessor.X[i] = (uint64)(c.uarch.processor.x[i])
	}
	uarchProcessor.Pc = (uint64)(c.uarch.processor.pc)
	uarchProcessor.Cycle = (uint64)(c.uarch.processor.cycle)
	uarchProcessor.HaltFlag = (bool)(c.uarch.processor.halt_flag)

	uarchRam := &uarch.Ram
	uarchRam.ImageFilename = C.GoString(c.uarch.ram.image_filename)

	return cfg
}

func makeCString(s *string) *C.char {
	if s == nil || *s == "" {
		return nil
	}
	return C.CString(*s)
}

///////////////////////////
// Public API
///////////////////////////

func NewDefaultMachineConfig() *MachineConfig {
	ref := theirMachineConfigCRef{}
	defer ref.free()
	ref.cref = C.cm_new_default_machine_config()
	return ref.makeGoRef()
}

// a connection to the remote jsonrpc machine manager
type RemoteMachineManager struct {
	RemoteAddress string
	cref          *C.cm_jsonrpc_mg_mgr
}

// a local or remote machine
type Machine struct {
	cref          *C.cm_machine
	remoteManager *RemoteMachineManager
}

func (mgr *RemoteMachineManager) Free() {
	if mgr != nil && mgr.cref != nil {
		C.cm_delete_jsonrpc_mg_mgr(mgr.cref)
		mgr.cref = nil
	}
}

func (machine *Machine) Free() {
	if machine == nil || machine.cref == nil {
		return
	}
	C.cm_delete_machine(machine.cref)
	machine.cref = nil
}

func (machine *Machine) Store(dir string) error {
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	var cerr *C.char
	if e := C.cm_store(machine.cref, cDir, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func NewMachine(config *MachineConfig, runtime *MachineRuntimeConfig) (*Machine, error) {
	machine := &Machine{}
	configRef := config.makeCRef()
	defer configRef.free()
	runtimeRef := runtime.makeCRef()
	defer runtimeRef.free()
	var cerr *C.char
	if e := C.cm_create_machine(configRef.cref, runtimeRef.cref, &machine.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return machine, nil
}

func LoadMachine(dir string, runtime *MachineRuntimeConfig) (*Machine, error) {
	machine := &Machine{}
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	runtimeRef := runtime.makeCRef()
	defer runtimeRef.free()
	var cerr *C.char
	if e := C.cm_load_machine(cDir, runtimeRef.cref, &machine.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)

	}
	return machine, nil
}

func GetDefaultConfig() (*MachineConfig, error) {
	theirCfg := theirMachineConfigCRef{}
	defer theirCfg.free()
	var cerr *C.char
	if e := C.cm_get_default_config(&theirCfg.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return theirCfg.makeGoRef(), nil
}

func (m *Machine) Run(mcycleEnd uint64) (BreakReason, error) {
	var cerr *C.char
	var creason C.CM_BREAK_REASON
	if e := C.cm_machine_run(m.cref, C.uint64_t(mcycleEnd), &creason, &cerr); isFailure(e) {
		return BreakReasonFailed, newError(e, cerr)
	}
	return (BreakReason)(creason), nil
}

func (machine *Machine) GetInitialConfig() (*MachineConfig, error) {
	theirCfg := theirMachineConfigCRef{}
	defer theirCfg.free()
	var cerr *C.char
	if e := C.cm_get_initial_config(machine.cref, &theirCfg.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return theirCfg.makeGoRef(), nil
}

func (machine *Machine) Destroy() error {
	var cerr *C.char
	if e := C.cm_destroy(machine.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) ReadCSR(r ProcessorCSR) (uint64, error) {
	var cval C.uint64_t
	var cerr *C.char
	if e := C.cm_read_csr(machine.cref, C.CM_PROC_CSR(r), &cval, &cerr); isFailure(e) {
		return 0, newError(e, cerr)
	}
	return uint64(cval), nil
}

func (machine *Machine) WriteCSR(r ProcessorCSR, val uint64) error {
	var cerr *C.char
	if e := C.cm_write_csr(machine.cref, C.CM_PROC_CSR(r), C.uint64_t(val), &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) ReadX(i int) (uint64, error) {
	var cval C.uint64_t
	var cerr *C.char
	if e := C.cm_read_x(machine.cref, C.int(i), &cval, &cerr); isFailure(e) {
		return 0, newError(e, cerr)
	}
	return uint64(cval), nil
}

func (machine *Machine) WriteX(i int, val uint64) error {
	var cerr *C.char
	if e := C.cm_write_x(machine.cref, C.int(i), C.uint64_t(val), &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) ReadF(i int) (uint64, error) {
	var cval C.uint64_t
	var cerr *C.char
	if e := C.cm_read_f(machine.cref, C.int(i), &cval, &cerr); isFailure(e) {
		return 0, newError(e, cerr)
	}
	return uint64(cval), nil
}

func (machine *Machine) ReadIFlagsX() (bool, error) {
	var cval C.bool
	var cerr *C.char
	if e := C.cm_read_iflags_X(machine.cref, &cval, &cerr); isFailure(e) {
		return false, newError(e, cerr)
	}
	return bool(cval), nil
}

func (machine *Machine) ResetIFlagsX() error {
	var cerr *C.char
	if e := C.cm_reset_iflags_X(machine.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) SetIFlagsX() error {
	var cerr *C.char
	if e := C.cm_set_iflags_X(machine.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) ReadIFlagsY() (bool, error) {
	var cval C.bool
	var cerr *C.char
	if e := C.cm_read_iflags_Y(machine.cref, &cval, &cerr); isFailure(e) {
		return false, newError(e, cerr)
	}
	return bool(cval), nil
}

func (machine *Machine) ResetIFlagsY() error {
	var cerr *C.char
	if e := C.cm_reset_iflags_Y(machine.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) SetIFlagsY() error {
	var cerr *C.char
	if e := C.cm_set_iflags_Y(machine.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) ReadIFlagsH() (bool, error) {
	var cval C.bool
	var cerr *C.char
	if e := C.cm_read_iflags_H(machine.cref, &cval, &cerr); isFailure(e) {
		return false, newError(e, cerr)
	}
	return bool(cval), nil
}

func (machine *Machine) SetIFlagsH() error {
	var cerr *C.char
	if e := C.cm_set_iflags_H(machine.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

type MerkleTreeHash [32]byte

func (hash *MerkleTreeHash) String() string {
	return hex.EncodeToString(hash[:])
}

func (machine *Machine) GetRootHash() (*MerkleTreeHash, error) {
	var chash C.cm_hash
	var cerr *C.char
	if e := C.cm_get_root_hash(machine.cref, &chash, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	hash := &MerkleTreeHash{}
	for i := 0; i < 32; i++ {
		hash[i] = byte(chash[i])
	}
	return hash, nil
}

func (machine *Machine) WriteF(i int, val uint64) error {
	var cerr *C.char
	if e := C.cm_write_f(machine.cref, C.int(i), C.uint64_t(val), &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func NewRemoteMachineManager(remoteAddress string) (*RemoteMachineManager, error) {
	manager := &RemoteMachineManager{RemoteAddress: remoteAddress}
	cRemoteAddress := C.CString(remoteAddress)
	defer C.free(unsafe.Pointer(cRemoteAddress))
	var cerr *C.char
	if e := C.cm_create_jsonrpc_mg_mgr(cRemoteAddress, &manager.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return manager, nil
}

func (mgr *RemoteMachineManager) Shutdown() error {
	var cerr *C.char
	if e := C.cm_jsonrpc_shutdown(mgr.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (mgr *RemoteMachineManager) Fork() (*string, error) {
	var cNewAddress *C.char = nil
	var cerr *C.char
	if e := C.cm_jsonrpc_fork(mgr.cref, &cNewAddress, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	newAddress := C.GoString(cNewAddress)
	C.cm_delete_cstring(cNewAddress)
	return &newAddress, nil
}

func (mgr *RemoteMachineManager) NewMachine(config *MachineConfig, runtime *MachineRuntimeConfig) (*Machine, error) {
	machine := &Machine{remoteManager: mgr}
	configRef := config.makeCRef()
	defer configRef.free()
	runtimeRef := runtime.makeCRef()
	defer runtimeRef.free()
	var cerr *C.char
	if e := C.cm_create_jsonrpc_machine(mgr.cref, configRef.cref, runtimeRef.cref, &machine.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return machine, nil
}

func (mgr *RemoteMachineManager) GetDefaultConfig() (*MachineConfig, error) {
	theirCfg := theirMachineConfigCRef{}
	defer theirCfg.free()
	var cerr *C.char
	if e := C.cm_jsonrpc_get_default_config(mgr.cref, &theirCfg.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return theirCfg.makeGoRef(), nil
}

func (mgr *RemoteMachineManager) LoadMachine(dir string, runtime *MachineRuntimeConfig) (*Machine, error) {
	machine := &Machine{remoteManager: mgr}
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	runtimeRef := runtime.makeCRef()
	defer runtimeRef.free()
	var cerr *C.char
	if e := C.cm_load_jsonrpc_machine(mgr.cref, cDir, runtimeRef.cref, &machine.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return machine, nil
}

func (mgr *RemoteMachineManager) GetMachine() (*Machine, error) {
	machine := &Machine{remoteManager: mgr}
	var cerr *C.char
	if e := C.cm_get_jsonrpc_machine(mgr.cref, &machine.cref, &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return machine, nil
}

func (machine *Machine) WriteMemory(address uint64, data []byte) error {
	var cerr *C.char
	if e := C.cm_write_memory(machine.cref, C.uint64_t(address), (*C.uchar)(&data[0]), C.size_t(len(data)), &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) ReadMemory(address uint64, length uint64) ([]byte, error) {
	data := make([]byte, length)
	var cerr *C.char
	if e := C.cm_read_memory(machine.cref, C.uint64_t(address), (*C.uchar)(&data[0]), C.uint64_t(length), &cerr); isFailure(e) {
		return nil, newError(e, cerr)
	}
	return data, nil
}

func (machine *Machine) ReplaceMemoryRange(newRange *MemoryRangeConfig) error {
	newRangeRef := newRange.makeCRef()
	defer newRangeRef.free()
	var cerr *C.char
	if e := C.cm_replace_memory_range(machine.cref, newRangeRef.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) Snapshot() error {
	var cerr *C.char
	if e := C.cm_snapshot(machine.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

func (machine *Machine) Rollback() error {
	var cerr *C.char
	if e := C.cm_rollback(machine.cref, &cerr); isFailure(e) {
		return newError(e, cerr)
	}
	return nil
}

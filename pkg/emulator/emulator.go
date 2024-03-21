// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

// #cgo LDFLAGS: -lcartesi -lcartesi_jsonrpc
// #include <stdlib.h>
// #include "cartesi-machine/jsonrpc-machine-c-api.h"
import "C"

import (
	"encoding/hex"
	"fmt"
	"unsafe"
)

// ------------------------------------------------------------------------------------------------
// Error
// ------------------------------------------------------------------------------------------------

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

type Error struct {
	Code ErrorCode
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("cartesi machine error %d (%s)", e.Code, e.Msg)
}

func newError(code C.int, msg *C.char) error {
	defer C.cm_delete_cstring(msg)
	if code != C.CM_ERROR_OK {
		return &Error{Code: ErrorCode(code), Msg: C.GoString(msg)}
	}
	return nil
}

// ------------------------------------------------------------------------------------------------
// Types
// ------------------------------------------------------------------------------------------------

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

// ------------------------------------------------------------------------------------------------
// MachineConfig
// ------------------------------------------------------------------------------------------------

func NewDefaultMachineConfig() *MachineConfig {
	ref := theirMachineConfigCRef{}
	defer ref.free()
	ref.cref = C.cm_new_default_machine_config()
	return ref.makeGoRef()
}

func GetDefaultMachineConfig() (*MachineConfig, error) {
	theirCfg := theirMachineConfigCRef{}
	defer theirCfg.free()
	var msg *C.char
	code := C.cm_get_default_config(&theirCfg.cref, &msg)
	if err := newError(code, msg); err != nil {
		return nil, err
	}
	return theirCfg.makeGoRef(), nil
}

// ------------------------------------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------------------------------------

type MerkleTreeHash [32]byte

func (hash *MerkleTreeHash) String() string {
	return hex.EncodeToString(hash[:])
}

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
	cFlashDrive.entry = (*C.cm_memory_range_config)(C.calloc((C.ulong)(len(*flashDrive)),
		C.sizeof_cm_memory_range_config))
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

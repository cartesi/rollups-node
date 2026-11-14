// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/pkg/emulator"
)

const (
	outputsRootSize     = uint64(32)
	txBufferStart       = uint64(0x60800000)
	rootKindValue       = "value"
	rootKindLength      = "length"
	rootKindTemplate    = "template"
	rootKindRecovery    = "recovery"
	recoveryRootAddress = ramStart + 0x100
)

// After one input, clear the TX word and yield with the request held in x19.
// The initial state can have the correct empty root or the invalid zero root.
var invalidOutputsRootProgram = []uint32{
	0x000a3023, // sd zero,0(x20)
	0x000a3423, // sd zero,8(x20)
	0x000a3823, // sd zero,16(x20)
	0x000a3c23, // sd zero,24(x20)
	0x400082b7, // lui t0,0x40008: HTIF base address
	0x0002b423, // sd zero,8(t0): clear fromhost
	0x0132b023, // sd x19,0(t0): issue the accepted yield
	0xfe5ff06f, // jal zero,-28: repeat after the next input
}

// After the first bad yield, copy the stored empty root on every later input.
var recoveredOutputsRootProgram = []uint32{
	0x000ab283, // ld t0,0(x21)
	0x005a3023, // sd t0,0(x20)
	0x008ab283, // ld t0,8(x21)
	0x005a3423, // sd t0,8(x20)
	0x010ab283, // ld t0,16(x21)
	0x005a3823, // sd t0,16(x20)
	0x018ab283, // ld t0,24(x21)
	0x005a3c23, // sd t0,24(x20)
	0x400082b7, // lui t0,0x40008: HTIF base address
	0x0002b423, // sd zero,8(t0): clear fromhost
	0x0132b023, // sd x19,0(t0): issue the accepted yield
	0xfd5ff06f, // jal zero,-44: repeat the correct declaration
}

func runInvalidOutputsRoot(args []string) error {
	flags := flag.NewFlagSet("invalid-outputs-root", flag.ContinueOnError)
	output := flags.String("output", "", "snapshot output directory")
	kind := flags.String("kind", rootKindValue, "root declaration: value, length, template, or recovery")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *output == "" {
		return errors.New("invalid-outputs-root requires --output")
	}
	if *kind != rootKindValue && *kind != rootKindLength && *kind != rootKindTemplate && *kind != rootKindRecovery {
		return fmt.Errorf("unknown outputs-root failure %q", *kind)
	}
	if err := requireAbsent(*output); err != nil {
		return err
	}
	config, err := json.Marshal(map[string]any{
		"processor": map[string]any{"registers": map[string]any{"pc": ramStart}},
		"ram":       map[string]any{"length": unexpectedYieldRAMSize},
		"cmio":      map[string]any{"rx_buffer": map[string]any{}, "tx_buffer": map[string]any{}},
	})
	if err != nil {
		return fmt.Errorf("encode machine config: %w", err)
	}
	machine, err := emulator.CreateMachine(string(config), "", "")
	if err != nil {
		return fmt.Errorf("create machine: %w", err)
	}
	defer machine.Delete()
	defer func() { _ = machine.Destroy() }()

	instructions := invalidOutputsRootProgram
	if *kind == rootKindRecovery {
		instructions = append([]uint32{}, invalidOutputsRootProgram[:len(invalidOutputsRootProgram)-1]...)
		instructions = append(instructions, recoveredOutputsRootProgram...)
	}
	program := make([]byte, 4*len(instructions))
	for i, instruction := range instructions {
		binary.LittleEndian.PutUint32(program[4*i:], instruction)
	}
	if err := machine.WriteMemory(ramStart, program); err != nil {
		return fmt.Errorf("write guest program: %w", err)
	}
	if *kind != rootKindTemplate {
		root := merkle.CreatePostContext()[merkle.TREE_DEPTH]
		if err := machine.WriteMemory(txBufferStart, root[:]); err != nil {
			return fmt.Errorf("write initial outputs root: %w", err)
		}
	}
	if *kind == rootKindRecovery {
		root := merkle.CreatePostContext()[merkle.TREE_DEPTH]
		if err := machine.WriteMemory(recoveryRootAddress, root[:]); err != nil {
			return fmt.Errorf("write recovery outputs root: %w", err)
		}
		if err := machine.WriteReg(emulator.REG_X21, recoveryRootAddress); err != nil {
			return fmt.Errorf("initialize recovery root address: %w", err)
		}
	}
	length := outputsRootSize
	if *kind == rootKindLength {
		length--
	}
	request := htifYieldDevice<<56 | uint64(emulator.YieldManual)<<48 |
		uint64(emulator.ManualYieldReasonAccepted)<<32 | length
	for _, register := range []struct {
		id    emulator.RegID
		value uint64
	}{
		{emulator.REG_X19, request},
		{emulator.REG_X20, txBufferStart},
		{emulator.REG_IFLAGS_Y, 1},
		{emulator.REG_HTIF_TOHOST_DEV, htifYieldDevice},
		{emulator.REG_HTIF_TOHOST_CMD, uint64(emulator.YieldManual)},
		{emulator.REG_HTIF_TOHOST_REASON, uint64(emulator.ManualYieldReasonAccepted)},
		{emulator.REG_HTIF_TOHOST_DATA, outputsRootSize},
	} {
		if err := machine.WriteReg(register.id, register.value); err != nil {
			return fmt.Errorf("initialize register %d: %w", register.id, err)
		}
	}
	if err := requireAcceptedYield(machine); err != nil {
		return fmt.Errorf("generated snapshot: %w", err)
	}
	return store(machine, *output)
}

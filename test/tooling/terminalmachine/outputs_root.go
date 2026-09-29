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
	outputsRootSize  = uint64(32)
	txBufferStart    = uint64(0x60800000)
	rootKindValue    = "value"
	rootKindLength   = "length"
	rootKindTemplate = "template"
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

func runInvalidOutputsRoot(args []string) error {
	flags := flag.NewFlagSet("invalid-outputs-root", flag.ContinueOnError)
	output := flags.String("output", "", "snapshot output directory")
	kind := flags.String("kind", rootKindValue, "failure: value, length, or template")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *output == "" {
		return errors.New("invalid-outputs-root requires --output")
	}
	if *kind != rootKindValue && *kind != rootKindLength && *kind != rootKindTemplate {
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

	program := make([]byte, 4*len(invalidOutputsRootProgram))
	for i, instruction := range invalidOutputsRootProgram {
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

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Command terminalmachine creates machine snapshots that deterministically
// reach terminal execution outcomes during integration tests.
package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

const (
	minimumArgumentCount   = 2
	ramStart               = uint64(0x80000000)
	unexpectedYieldReason  = uint64(9)
	unexpectedYieldData    = uint64(23)
	htifYieldDevice        = uint64(2)
	mcycleOverflowHeadroom = uint64(255)
	unexpectedYieldRAMSize = uint64(4096)
	snapshotDirectoryMode  = os.FileMode(0o755)
	storedRootHashOffset   = int64(0x60)

	machineTimeout = 5 * time.Minute
)

var unexpectedYieldProgram = []uint32{
	0x400082b7, // lui t0,0x40008: load the HTIF base address
	0x0002b423, // sd zero,8(t0): clear fromhost
	0x0132b023, // sd x19,0(t0): write the prepared request to tohost
}

func main() {
	if len(os.Args) < minimumArgumentCount {
		fatalf("usage: terminalmachine <mcycle-overflow|unexpected-yield> [options]")
	}

	var err error
	switch os.Args[1] {
	case "mcycle-overflow":
		err = runMcycleOverflow(os.Args[2:])
	case "unexpected-yield":
		err = runUnexpectedYield(os.Args[2:])
	default:
		err = fmt.Errorf("unknown fixture %q", os.Args[1])
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func runMcycleOverflow(args []string) error {
	flags := flag.NewFlagSet("mcycle-overflow", flag.ContinueOnError)
	source := flags.String("source", "", "accepted-yield machine snapshot to clone")
	output := flags.String("output", "", "snapshot output directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *source == "" || *output == "" {
		return errors.New("mcycle-overflow requires --source and --output")
	}
	if err := requireAbsent(*output); err != nil {
		return err
	}

	machine, _, _, err := emulator.SpawnServer("127.0.0.1:0", machineTimeout)
	if err != nil {
		return fmt.Errorf("spawn emulator server: %w", err)
	}
	defer machine.Delete()
	defer func() { _ = machine.ShutdownServer() }()

	if err := machine.Load(*source, ""); err != nil {
		return fmt.Errorf("load source snapshot: %w", err)
	}
	if err := requireAcceptedYield(&machine.Machine); err != nil {
		return fmt.Errorf("source snapshot: %w", err)
	}

	// The emulator saturates the per-input cycle limit at UINT64_MAX. Starting
	// this close to the boundary makes normal advance-state delivery exercise
	// CM_BREAK_REASON_MCYCLE_OVERFLOW without changing the guest program.
	if err := machine.WriteReg(
		emulator.REG_MCYCLE, math.MaxUint64-mcycleOverflowHeadroom,
	); err != nil {
		return fmt.Errorf("set mcycle: %w", err)
	}
	if err := store(&machine.Machine, *output); err != nil {
		return err
	}
	if err := sendAdvanceAndRun(&machine.Machine, emulator.BreakReasonMcycleOverflow); err != nil {
		_ = os.RemoveAll(*output)
		return fmt.Errorf("validate stored fixture: %w", err)
	}
	return nil
}

func runUnexpectedYield(args []string) error {
	flags := flag.NewFlagSet("unexpected-yield", flag.ContinueOnError)
	output := flags.String("output", "", "snapshot output directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *output == "" {
		return errors.New("unexpected-yield requires --output")
	}
	if err := requireAbsent(*output); err != nil {
		return err
	}

	config, err := json.Marshal(map[string]any{
		"processor": map[string]any{
			"registers": map[string]any{"pc": ramStart},
		},
		"ram": map[string]any{"length": unexpectedYieldRAMSize},
		"cmio": map[string]any{
			"rx_buffer": map[string]any{},
			"tx_buffer": map[string]any{},
		},
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

	program := make([]byte, 4*len(unexpectedYieldProgram))
	for i, instruction := range unexpectedYieldProgram {
		binary.LittleEndian.PutUint32(program[4*i:], instruction)
	}
	if err := machine.WriteMemory(ramStart, program); err != nil {
		return fmt.Errorf("write guest program: %w", err)
	}

	request := htifYieldDevice<<56 |
		uint64(emulator.YieldManual)<<48 |
		unexpectedYieldReason<<32 |
		unexpectedYieldData
	registers := []struct {
		id    emulator.RegID
		value uint64
	}{
		{emulator.REG_X19, request},
		{emulator.REG_IFLAGS_Y, 1},
		{emulator.REG_HTIF_TOHOST_DEV, htifYieldDevice},
		{emulator.REG_HTIF_TOHOST_CMD, uint64(emulator.YieldManual)},
		{emulator.REG_HTIF_TOHOST_REASON, uint64(emulator.ManualYieldReasonAccepted)},
		{emulator.REG_HTIF_TOHOST_DATA, 0},
	}
	for _, register := range registers {
		if err := machine.WriteReg(register.id, register.value); err != nil {
			return fmt.Errorf("initialize register %d: %w", register.id, err)
		}
	}
	if err := requireAcceptedYield(machine); err != nil {
		return fmt.Errorf("generated snapshot: %w", err)
	}
	if err := store(machine, *output); err != nil {
		return err
	}
	if err := sendAdvanceAndRun(machine, emulator.BreakReasonYieldedManually); err != nil {
		_ = os.RemoveAll(*output)
		return fmt.Errorf("validate stored fixture: %w", err)
	}
	command, reason, _, err := machine.ReceiveCmioRequest()
	if err != nil {
		_ = os.RemoveAll(*output)
		return fmt.Errorf("read terminal CMIO request: %w", err)
	}
	if command != uint8(emulator.YieldManual) || reason != uint16(unexpectedYieldReason) {
		_ = os.RemoveAll(*output)
		return fmt.Errorf("expected manual yield reason %d, got command %d reason %d",
			unexpectedYieldReason, command, reason)
	}
	return nil
}

func sendAdvanceAndRun(machine *emulator.Machine, expected emulator.BreakReason) error {
	revertRootHash, err := machine.GetRootHash()
	if err != nil {
		return fmt.Errorf("read accepted-state root hash: %w", err)
	}
	if err := machine.SendCmioResponse(
		uint16(emulator.YieldReasonAdvanceState), nil, &revertRootHash,
	); err != nil {
		return fmt.Errorf("send advance-state response: %w", err)
	}
	result, err := machine.Run(math.MaxUint64)
	if err != nil {
		return fmt.Errorf("run machine: %w", err)
	}
	if result != expected {
		return fmt.Errorf("expected %s, got %s", expected, result)
	}
	return nil
}

func requireAcceptedYield(machine *emulator.Machine) error {
	command, reason, _, err := machine.ReceiveCmioRequest()
	if err != nil {
		return fmt.Errorf("read initial CMIO request: %w", err)
	}
	if command != uint8(emulator.YieldManual) ||
		reason != uint16(emulator.ManualYieldReasonAccepted) {
		return fmt.Errorf("expected manual accepted yield, got command %d reason %d", command, reason)
	}
	return nil
}

func requireAbsent(path string) error {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return fmt.Errorf("output %q already exists", path)
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect output %q: %w", path, err)
	}
	return nil
}

func store(machine *emulator.Machine, output string) error {
	if err := os.MkdirAll(filepath.Dir(output), snapshotDirectoryMode); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}

	// Force the emulator to materialize dirty Merkle nodes before storing.
	// The deploy CLI reads the cached root at hash_tree.sht offset 0x60, while
	// the advancer asks the emulator for the live root after loading. Comparing
	// both representations here prevents a fixture from registering one hash
	// and loading as another.
	expectedRootHash, err := machine.GetRootHash()
	if err != nil {
		return fmt.Errorf("calculate snapshot root hash: %w", err)
	}
	if err := machine.Store(output); err != nil {
		_ = os.RemoveAll(output)
		return fmt.Errorf("store snapshot: %w", err)
	}
	storedRootHash, err := readStoredRootHash(output)
	if err != nil {
		_ = os.RemoveAll(output)
		return err
	}
	if storedRootHash != expectedRootHash {
		_ = os.RemoveAll(output)
		return fmt.Errorf("stored root hash %x does not match emulator root %x",
			storedRootHash, expectedRootHash)
	}
	return nil
}

func readStoredRootHash(output string) (emulator.Hash, error) {
	var rootHash emulator.Hash
	file, err := os.Open(filepath.Join(output, "hash_tree.sht"))
	if err != nil {
		return rootHash, fmt.Errorf("open stored hash tree: %w", err)
	}
	defer file.Close()
	if _, err := file.ReadAt(rootHash[:], storedRootHashOffset); err != nil {
		return rootHash, fmt.Errorf("read stored root hash: %w", err)
	}
	return rootHash, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "terminalmachine: "+format+"\n", args...)
	os.Exit(1)
}

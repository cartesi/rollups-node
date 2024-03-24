// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Simple command line interface to the Cartesi Machine C API wrapper

package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

func main() {
	var machine *emulator.Machine
	defer machine.Delete()
	var mgr *emulator.RemoteMachineManager
	defer mgr.Free()
	var err error
	runtimeConfig := &emulator.MachineRuntimeConfig{}

	// Parse command line arguments
	loadDir := flag.String("load", "", "load machine previously stored in <directory>")
	storeDir := flag.String("store", "", "store machine to <directory>, where \"%h\" is substituted by the state hash in the directory name")
	remoteAddress := flag.String("remote-address", "", "use a remote cartesi machine listening to <address> instead of running a local cartesi machine")
	remoteShutdown := flag.Bool("remote-shutdown", false, "shutdown the remote cartesi machine after the execution")
	noRemoteCreate := flag.Bool("no-remote-create", false, "use existing cartesi machine in the remote server instead of creating a new one")
	noRemoteDestroy := flag.Bool("no-remote-destroy", false, "do not destroy the cartesi machine in the remote server after the execution")
	ramImage := flag.String("ram-image", "", "name of file containing RAM image")
	dtbImage := flag.String("dtb-image", "", "name of file containing DTB image (default: auto generated flattened device tree)")
	maxMcycle := flag.Uint64("max-mcycle", math.MaxUint64, "stop at a given mcycle")
	initialHash := flag.Bool("initial-hash", false, "print initial state hash before running machine")
	finalHash := flag.Bool("final-hash", false, "print final state hash when done")
	commandLine := flag.String("command", "", "command to run in the machine")
	flag.Parse()

	// Connect to remote server and load/get machine
	if remoteAddress != nil && *remoteAddress != "" {
		fmt.Println("Connecting to remote server at ", *remoteAddress)
		if mgr, err = emulator.NewRemoteMachineManager(*remoteAddress); err != nil {
			fmt.Fprintln(os.Stderr, "****** Error creating remote machine manager: ", err)
			os.Exit(1)
		}
		if noRemoteCreate != nil && *noRemoteCreate {
			fmt.Println("Using existing remote machine")
			if machine, err = mgr.GetMachine(); err != nil {
				fmt.Fprintln(os.Stderr, "****** Error getting remote machine: ", err)
				os.Exit(1)
			}
		} else if loadDir != nil && *loadDir != "" {
			fmt.Println("Loading remote machine from ", *loadDir)
			if machine, err = mgr.LoadMachine(*loadDir, runtimeConfig); err != nil {
				fmt.Fprintln(os.Stderr, "****** Error loading machine: ", err)
				os.Exit(1)
			}
		}
	} else if loadDir != nil && *loadDir != "" {
		fmt.Println("Loading machine from ", *loadDir)
		if machine, err = emulator.LoadMachine(*loadDir, runtimeConfig); err != nil {
			fmt.Fprintln(os.Stderr, "****** Error loading machine: ", err)
			os.Exit(1)
		}
	}

	// No machine yet: build configuration and create machine
	if machine == nil {
		// build machine configuration
		images_path := strings.TrimRight(os.Getenv("CARTESI_IMAGES_PATH"), "/") + "/"
		cfg := emulator.NewDefaultMachineConfig()
		cfg.Processor.Mimpid = math.MaxUint64
		cfg.Processor.Marchid = math.MaxUint64
		cfg.Processor.Mvendorid = math.MaxUint64
		cfg.Ram.ImageFilename = images_path + "linux.bin"
		if ramImage != nil && *ramImage != "" {
			fmt.Println("Using RAM image: ", *ramImage)
			cfg.Ram.ImageFilename = *ramImage
		}
		cfg.Ram.Length = 64 << 20
		cfg.FlashDrive = []emulator.MemoryRangeConfig{
			{
				Start:         0x80000000000000,
				Length:        0xffffffffffffffff,
				Shared:        false,
				ImageFilename: images_path + "rootfs.ext2",
			},
		}
		cfg.Dtb.Bootargs = "quiet earlycon=sbi console=hvc0 rootfstype=ext2 root=/dev/pmem0 rw init=/usr/sbin/cartesi-init"
		if dtbImage != nil && *dtbImage != "" {
			cfg.Dtb.ImageFilename = *dtbImage
		}
		cfg.Dtb.Init = `echo "Opa!"
			busybox mkdir -p /run/drive-label && echo "root" > /run/drive-label/pmem0\
			USER=dapp
		`
		if commandLine != nil && *commandLine != "" {
			cfg.Dtb.Init = *commandLine
		}
		// create machine using configuration
		if mgr == nil {
			fmt.Println("Creating local machine")
			if machine, err = emulator.NewMachine(cfg, runtimeConfig); err != nil {
				fmt.Fprintln(os.Stderr, "****** Error creating machine: ", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("Creating remote machine")
			if machine, err = mgr.NewMachine(cfg, runtimeConfig); err != nil {
				fmt.Fprintln(os.Stderr, "****** Error creating remote machine: ", err)
				os.Exit(1)
			}

		}
	}

	// No machine yet? Too bad
	if machine == nil {
		fmt.Fprintln(os.Stderr, "****** No machine to run")
		os.Exit(1)
	}

	// Print initial hash
	if initialHash != nil && *initialHash {
		var hash *emulator.MerkleTreeHash
		if hash, err = machine.GetRootHash(); err != nil {
			fmt.Fprintln(os.Stderr, "****** Error getting root hash: ", err)
			os.Exit(1)
		}
		fmt.Println("Initial hash: ", hash.String())
	}

	// Run machine
	var breakReason emulator.BreakReason
	if breakReason, err = machine.Run(*maxMcycle); err != nil {
		fmt.Fprintln(os.Stderr, "****** Error running machine: ", err)
		os.Exit(1)
	}
	switch breakReason {
	case emulator.BreakReasonFailed:
		fmt.Println("Machine failed")
	case emulator.BreakReasonHalted:
		fmt.Println("Machine halted")
	case emulator.BreakReasonYieldedManually:
		fmt.Println("Machine yielded manually")
	case emulator.BreakReasonYieldedAutomatically:
		fmt.Println("Machine yielded automatically")
	case emulator.BreakReasonYieldedSoftly:
		fmt.Println("Machine yielded softly")
	case emulator.BreakReasonReachedTargetMcycle:
		fmt.Println("Machine reached target mcycle")
	default:
		fmt.Println("Machine stopped for unknown reason")
	}

	cycle, _ := machine.ReadCSR(emulator.ProcCsrMcycle)
	fmt.Println("mcycle: ", cycle)

	// Print final hash
	if finalHash != nil && *finalHash {
		var hash *emulator.MerkleTreeHash
		if hash, err = machine.GetRootHash(); err == nil {
			fmt.Println("Final hash:   ", hash.String())
		}
	}

	// Store machine
	if storeDir != nil && *storeDir != "" {
		fmt.Println("Storing machine in ", *storeDir)
		if err = machine.Store(*storeDir); err != nil {
			fmt.Fprintln(os.Stderr, "****** Error storing machine: ", err)
			os.Exit(1)
		}
	}

	// Cleanup
	if mgr != nil {
		if !*noRemoteDestroy {
			fmt.Println("Destroying remote machine")
			if err = machine.Destroy(); err != nil {
				fmt.Fprintln(os.Stderr, "****** Error destroying remote machine: ", err)
				os.Exit(1)
			}
		}
		if *remoteShutdown {
			fmt.Println("Shutting down remote machine")
			if err = mgr.Shutdown(); err != nil {
				fmt.Fprintln(os.Stderr, "****** Error shutting down remote server: ", err)
				os.Exit(1)
			}
		}
	}
}

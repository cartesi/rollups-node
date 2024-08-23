// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package snapshot implements dynamic snapshot instantiation.
// It defines the Snapshot type, which manages the lifecycle of a snapshot.
package snapshot

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

const ramLength = 64 << 20

// ImagesPath is the path to the folder containing the linux.bin and rootfs.ext2 files.
// It can be redefined in case the files are not in the default folder.
var ImagesPath = "/usr/share/cartesi-machine/images/"

func init() {
	if value, ok := os.LookupEnv("CARTESI_TEST_MACHINE_IMAGES_PATH"); ok {
		ImagesPath = value
	}
}

type Snapshot struct {
	id   string // an unique id used to avoid name clashing
	temp string // path to the temporary directory containing snapshot relevant files
	path string // path to the snapshot directory

	// Reason why the snapshot stopped running before being stored.
	BreakReason emulator.BreakReason
}

// FromScript creates a snapshot given an script command.
func FromScript(command string, cycles uint64) (*Snapshot, error) {
	snapshot := &Snapshot{id: fmt.Sprintf("fromscript%d", rand.Int())}

	err := snapshot.createTempDir()
	if err != nil {
		return nil, err
	}

	config := defaultMachineConfig()
	config.Dtb.Entrypoint = command

	err = snapshot.createRunAndStore(config, cycles)
	return snapshot, err
}

// Path returns the path to the directory where the snapshot is stored.
func (snapshot *Snapshot) Path() string {
	return snapshot.path
}

// Close deletes the temporary directory created to hold the snapshot files.
func (snapshot *Snapshot) Close() error {
	return os.RemoveAll(snapshot.temp)
}

// ------------------------------------------------------------------------------------------------

func defaultMachineConfig() *emulator.MachineConfig {
	config := emulator.NewDefaultMachineConfig()
	config.Ram.Length = ramLength
	config.Ram.ImageFilename = ImagesPath + "linux.bin"
	config.Dtb.Bootargs = strings.Join([]string{"quiet",
		"no4lvl",
		"quiet",
		"earlycon=sbi",
		"console=hvc0",
		"rootfstype=ext2",
		"root=/dev/pmem0",
		"rw",
		"init=/usr/sbin/cartesi-init"}, " ")
	config.FlashDrive = []emulator.MemoryRangeConfig{{
		Start:         0x80000000000000, //nolint:mnd
		Length:        math.MaxUint64,
		ImageFilename: ImagesPath + "rootfs.ext2",
	}}
	return config
}

func (snapshot *Snapshot) createTempDir() error {
	temp, err := os.MkdirTemp("", snapshot.id+"-*")
	if err != nil {
		return err
	}
	snapshot.temp = temp
	snapshot.path = snapshot.temp + "/snapshot"
	return nil
}

func (snapshot *Snapshot) createRunAndStore(config *emulator.MachineConfig, cycles uint64) error {
	// Creates the (local) machine.
	machine, err := emulator.NewMachine(config, &emulator.MachineRuntimeConfig{})
	if err != nil {
		return errors.Join(err, snapshot.Close())
	}
	defer machine.Delete()

	// Runs the machine.
	snapshot.BreakReason, err = machine.Run(cycles)
	if err != nil {
		return errors.Join(err, snapshot.Close())
	}

	// Stores the machine.
	err = machine.Store(snapshot.path)
	if err != nil {
		return errors.Join(err, snapshot.Close())
	}

	return nil
}

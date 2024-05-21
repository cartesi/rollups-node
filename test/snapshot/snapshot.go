// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

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

const (
	ramLength = 64 << 20
)

var ImagesPath = "/usr/share/cartesi-machine/images/"

type Snapshot struct {
	pkg  string
	temp string

	// Directory where the snapshot was stored.
	Dir string

	// Reason why the snapshot stopped running before being stored.
	BreakReason emulator.BreakReason
}

// FromScript creates a snapshot given an one-line command.
func FromScript(command string, cycles uint64) (*Snapshot, error) {
	snapshot := &Snapshot{pkg: fmt.Sprintf("fromscript%d", rand.Int())}

	err := snapshot.createTempDir()
	if err != nil {
		return nil, err
	}

	config := defaultMachineConfig()
	config.Dtb.Entrypoint = command

	err = snapshot.createRunAndStore(config, cycles)
	return snapshot, err
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
	temp, err := os.MkdirTemp("", snapshot.pkg+"-*")
	if err != nil {
		return err
	}
	snapshot.temp = temp
	snapshot.Dir = snapshot.temp + "/snapshot"
	return nil
}

func (snapshot *Snapshot) createRunAndStore(config *emulator.MachineConfig, cycles uint64) error {
	// Creates the (local) machine.
	machine, err := emulator.NewMachine(config, &emulator.MachineRuntimeConfig{})
	if err != nil {
		return snapshot.errAndClose(err)
	}
	defer machine.Delete()

	// Runs the machine.
	snapshot.BreakReason, err = machine.Run(cycles)
	if err != nil {
		return snapshot.errAndClose(err)
	}

	// Stores the machine.
	err = machine.Store(snapshot.Dir)
	if err != nil {
		return snapshot.errAndClose(err)
	}

	return nil
}

func (snapshot Snapshot) errAndClose(err error) error {
	return errors.Join(err, snapshot.Close())
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package snapshot

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

const (
	ramLength = 64 << 20
)

var (
	ImagesPath = "/usr/share/cartesi-machine/images/"
	RiscvCC    = "riscv64-linux-gnu-gcc-12"
)

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

// FromGoCode creates a snapshot from Go code.
func FromGoCode(cycles uint64, code string) (*Snapshot, error) {
	pkg := fmt.Sprintf("fromgocode%d", rand.Int())

	const perm = 0775

	err := os.Mkdir(pkg, perm)
	if err != nil {
		return nil, err
	}

	defer os.RemoveAll(pkg)

	err = os.WriteFile(pkg+"/"+pkg+".go", []byte(code), perm)
	if err != nil {
		return nil, err
	}

	return FromGoProject(pkg, cycles)
}

// FromGoProject creates a snapshot from a Go project.
// It expects the main function to be at pkg/pkg.go.
func FromGoProject(pkg string, cycles uint64) (*Snapshot, error) {
	snapshot := &Snapshot{pkg: pkg}

	err := snapshot.createTempDir()
	if err != nil {
		return nil, err
	}

	// Building the riscv64 binary from source.
	sourceCode := pkg + "/" + pkg + ".go"
	cmd := exec.Command("go", "build", "-o", snapshot.temp, sourceCode)
	cmd.Env = append(os.Environ(), "CC="+RiscvCC, "CGO_ENABLED=1", "GOOS=linux", "GOARCH=riscv64")
	err = run(cmd)
	if err != nil {
		return nil, snapshot.errAndClose(err)
	}

	// Creating the .ext2 file.
	const k = uint64(10)
	blocks := strconv.FormatUint(k*1024, 10)
	ext2 := pkg + ".ext2"
	err = run(exec.Command("xgenext2fs", "-f", "-b", blocks, "-d", snapshot.temp, ext2))
	if err != nil {
		return nil, snapshot.errAndClose(err)
	}
	err = run(exec.Command("mv", ext2, snapshot.temp))
	if err != nil {
		return nil, snapshot.errAndClose(err)
	}

	// Modifying the default config.
	config := defaultMachineConfig()
	config.Dtb.Init = fmt.Sprintf(`
        echo "Test Cartesi Machine"
        busybox mkdir -p /run/drive-label && echo "root" > /run/drive-label/pmem0
        busybox mkdir -p "/mnt/%s" && busybox mount /dev/pmem1 "/mnt/%s"
        busybox mkdir -p /run/drive-label && echo "%s" > /run/drive-label/pmem1`,
		pkg, pkg, pkg)
	config.Dtb.Entrypoint = fmt.Sprintf("CMT_DEBUG=yes /mnt/%s/%s", pkg, pkg)
	config.FlashDrive = append(config.FlashDrive, emulator.MemoryRangeConfig{
		Start:         0x90000000000000, //nolint:mnd
		Length:        math.MaxUint64,
		ImageFilename: snapshot.temp + "/" + ext2,
	})

	err = snapshot.createRunAndStore(config, cycles)
	if err != nil {
		return nil, err
	}

	return snapshot, nil
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

func run(cmd *exec.Cmd) error {
	slog.Debug("running", "command", cmd.String())
	output, err := cmd.CombinedOutput()
	if s := string(output); s != "" {
		slog.Debug(s)
	}
	if err != nil {
		slog.Error(err.Error())
	}
	return err
}

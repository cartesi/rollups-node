// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package tests

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/stretchr/testify/require"
)

var imagesPath = "/usr/local/share/cartesi-machine/images/"

const riscvCC = "riscv64-linux-gnu-gcc-12"

func init() {
	if s, ok := os.LookupEnv("CARTESI_IMAGES_PATH"); ok {
		imagesPath = s
	}
}

func simpleSnapshot(t *testing.T,
	name string,
	entrypoint string,
	cycles uint64,
	expectedBreakReason emulator.BreakReason) {

	// Removes previously stored machine.
	runCmd(exec.Command("rm", "-rf", name))

	// Creates the machine configuration.
	config := defaultMachineConfig()
	config.Dtb.Entrypoint = entrypoint

	machineCreateRunStore(t, name, config, cycles, expectedBreakReason, name)
}

func crossCompiledSnapshot(t *testing.T,
	name string,
	cycles uint64,
	expectedBreakReason emulator.BreakReason) {

	// Removes previously stored machine.
	runCmd(exec.Command("rm", "-rf", name+"/snapshot"))

	// Removes temporary files.
	defer runCmd(
		exec.Command("rm", "-rf", name+"/temp"),
		exec.Command("rm", "-f", name+"/"+name+".ext2"))

	// Builds the riscv64 binary from the Go program.
	directory := name + "/temp/"
	cmd := exec.Command("go", "build", "-o", directory+name, name+"/main.go")
	cmd.Env = append(os.Environ(), "CC="+riscvCC, "CGO_ENABLED=1", "GOOS=linux", "GOARCH=riscv64")
	runCmd(cmd)

	// Creates the .ext2 file.
	k := uint64(10)
	ext2 := name + "/" + name + ".ext2"
	blocks := strconv.FormatUint(k*1024, 10)
	cmd = exec.Command("xgenext2fs", "-f", "-b", blocks, "-d", directory, ext2)
	runCmd(cmd)

	// Modifies the default config.
	config := defaultMachineConfig()
	config.Dtb.Init = fmt.Sprintf(`echo "Test Cartesi Machine"
        busybox mkdir -p /run/drive-label && echo "root" > /run/drive-label/pmem0
        busybox mkdir -p "/mnt/%s" && busybox mount /dev/pmem1 "/mnt/%s"
        busybox mkdir -p /run/drive-label && echo "%s" > /run/drive-label/pmem1`,
		name, name, name)
	config.Dtb.Entrypoint = fmt.Sprintf("CMT_DEBUG=yes /mnt/%s/%s", name, name)
	config.FlashDrive = append(config.FlashDrive, emulator.MemoryRangeConfig{
		Start:         0x90000000000000,
		Length:        math.MaxUint64,
		ImageFilename: name + "/" + name + ".ext2",
	})

	machineCreateRunStore(t, name, config, cycles, expectedBreakReason, name+"/snapshot")
}

// ------------------------------------------------------------------------------------------------

func defaultMachineConfig() *emulator.MachineConfig {
	config := emulator.NewDefaultMachineConfig()
	config.Ram.Length = 64 << 20
	config.Ram.ImageFilename = imagesPath + "linux.bin"
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
		Start:         0x80000000000000,
		Length:        math.MaxUint64,
		ImageFilename: imagesPath + "rootfs.ext2",
	}}
	return config
}

func machineCreateRunStore(t *testing.T,
	name string,
	config *emulator.MachineConfig,
	cycles uint64,
	expectedBreakReason emulator.BreakReason,
	storeAt string) {

	// Creates the (local) machine.
	machine, err := emulator.NewMachine(config, &emulator.MachineRuntimeConfig{})
	require.Nil(t, err)
	defer machine.Delete()

	// Runs the machine.
	reason, err := machine.Run(cycles)
	require.Nil(t, err)
	require.Equal(t, expectedBreakReason, reason, "%s != %s", expectedBreakReason, reason)

	// Stores the machine.
	err = machine.Store(storeAt)
	require.Nil(t, err)
}

func runCmd(cmds ...*exec.Cmd) {
	for _, cmd := range cmds {
		slog.Debug("running", "command", cmd.String())
		output, err := cmd.CombinedOutput()
		if s := string(output); s != "" {
			slog.Debug(s)
		}
		if err != nil {
			slog.Error(err.Error())
		}
	}
}

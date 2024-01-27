package tests

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

var RiscVCC = "riscv64-linux-gnu-gcc-12"

// CreateGollupSnapshot creates a cartesi machine snapshot
// with a block size of 1024*kblock
// from the Go program at name.
func CreateGollupSnapshot(name string, kblocks uint64) {
	slog.Info("----- Creating Gollup snapshot", "name", name)

	folder := name + "/"
	main := folder + "main.go"
	binary := "temp/" + name
	ext2 := name + ".ext2"
	snapshot := folder + "snapshot"

	defer func() {
		folder := name + "/"
		run(exec.Command("rm", "-f", folder+name),
			exec.Command("rm", "-f", name+".ext2"),
			exec.Command("rm", "-rf", "temp"))
	}()

	goCmd := exec.Command("go", "build", "-o", binary, main)
	goCmd.Env = append(os.Environ(),
		"CC="+RiscVCC,
		"CGO_ENABLED=1",
		"GOOS=linux",
		"GOARCH=riscv64",
	)

	blocks := strconv.FormatUint(kblocks*1024, 10)

	run(exec.Command("rm", "-rf", snapshot),
		goCmd,
		exec.Command("xgenext2fs", "-f", "-b", blocks, "-d", "temp", ext2),
		exec.Command("cartesi-machine",
			fmt.Sprintf("--flash-drive=label:%s,filename:%s", name, ext2),
			fmt.Sprintf("--store=%s", snapshot),
			"--", "CMT_DEBUG=yes", fmt.Sprintf("/mnt/%s/%s", name, name)))
}

func CreateSimpleSnapshot(name, bash string) {
	slog.Info("----- Creating simple snapshot", "name", name)
	run(exec.Command("rm", "-rf", name),
		exec.Command("mkdir", "-p", name),
		exec.Command("cartesi-machine", fmt.Sprintf("--store=%s", name+"/snapshot"), "--", bash))
}

func run(cmds ...*exec.Cmd) {
	for _, cmd := range cmds {
		slog.Info(cmd.String())
		output, err := cmd.CombinedOutput()
		if s := string(output); s != "" {
			slog.Info(s)
		}
		if err != nil {
			slog.Error(err.Error())
		}
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// processRSS reads resident memory, excluding virtual address-space reservations.
func processRSS(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "VmRSS:" && fields[2] == "kB" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			return kb * 1024, err
		}
	}
	return 0, fmt.Errorf("process %d has no VmRSS", pid)
}

// limitedNodeCommand uses a systemd scope to apply the same cgroup memory
// controls as a memory-limited container, without changing the node's filesystem
// or network. The default is 1 GiB for the process and all its descendants
// combined; swap is disabled so it cannot hide memory pressure. Scope mode
// preserves the PID, environment, working directory, and signal handling.
// CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES overrides the limit; 0 disables it.
// This setting belongs to the test runner, not extraEnv. Externally managed
// nodes are unaffected. Each service in multiprocess mode gets its own scope.
// Requires systemd-run and a user systemd manager with memory controller support
// (including when the integration runner itself runs inside a container).
func limitedNodeCommand(binary string, log io.Writer) (*exec.Cmd, error) {
	value := os.Getenv("CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES")
	if value == "" {
		value = "1073741824"
	}
	limit, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES %q: %w", value, err)
	}
	if limit == 0 {
		fmt.Fprintf(log, "%s: memory limit disabled\n", binary)
		return exec.Command(binary), nil //nolint:gosec
	}
	launcher, err := exec.LookPath("systemd-run")
	if err != nil {
		return nil, fmt.Errorf("memory limit requires systemd-run and a user systemd manager: %w", err)
	}
	value = strconv.FormatUint(limit, 10)
	fmt.Fprintf(log, "%s: cgroup memory limit %s bytes (including descendants), swap disabled\n", binary, value)
	return exec.Command(launcher, "--user", "--scope", "--quiet",
		"--property=MemoryMax="+value, "--property=MemorySwapMax=0",
		"--", binary), nil //nolint:gosec
}

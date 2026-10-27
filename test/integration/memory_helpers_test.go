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

// limitedNodeCommand applies a per-process Linux address-space limit before
// exec, preserving the node PID and signal handling. Allocation failures appear
// in the node log. This is virtual memory, not RSS: Go reservations and machine
// mappings count too. Children inherit the limit, each with its own allowance.
// CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES overrides the default 2.5 GiB; 0 disables
// the limit. This setting belongs to the test runner, not extraEnv. Externally
// managed nodes are unaffected. Enforcing the limit requires util-linux prlimit.
func limitedNodeCommand(binary string, log io.Writer) (*exec.Cmd, error) {
	value := os.Getenv("CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES")
	if value == "" {
		value = "2600000000"
	}
	limit, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES %q: %w", value, err)
	}
	if limit == 0 {
		fmt.Fprintf(log, "%s: memory limit disabled\n", binary)
		return exec.Command(binary), nil //nolint:gosec
	}
	prlimit, err := exec.LookPath("prlimit")
	if err != nil {
		return nil, fmt.Errorf("memory limit requires util-linux prlimit: %w", err)
	}
	value = strconv.FormatUint(limit, 10)
	fmt.Fprintf(log, "%s: virtual memory limit %s bytes per process\n", binary, value)
	return exec.Command(prlimit, "--as="+value+":"+value, "--", binary), nil //nolint:gosec
}

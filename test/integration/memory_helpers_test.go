// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// processRSS reads resident memory, excluding virtual address-space reservations.
func processRSS(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	return parseProcessRSS(data)
}

func parseProcessRSS(data []byte) (uint64, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "VmRSS:" && fields[2] == "kB" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			return kb * 1024, err
		}
	}
	return 0, fmt.Errorf("process status has no VmRSS")
}

// limitedNodeCommand runs the current binary in Docker with a default 1 GiB
// memory limit shared by all descendants and no swap. Each multiprocess service
// gets a separate container. CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES overrides the
// limit; 0 runs directly without Docker. External nodes are unaffected.
// CARTESI_TEST_NODE_IMAGE selects the runtime image (default:
// cartesi/rollups-node:tester, built with make tester-image). The image must
// provide the binary's runtime libraries. Host networking preserves localhost
// endpoints. Bind mounts require a local Linux Docker daemon with access to the
// runner's paths; a runner inside Docker must expose its paths to that daemon.
func limitedNodeCommand(binary string, log io.Writer, env []string, workDir string) (*exec.Cmd, error) {
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
		cmd := exec.Command(binary) //nolint:gosec
		cmd.Env, cmd.Dir = env, workDir
		return cmd, nil
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("memory limit requires Docker: %w", err)
	}
	executable, err := exec.LookPath(binary)
	if err != nil {
		return nil, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, err
	}
	// Bind the actual executable rather than a symlink with a host-only target.
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	root, err := nodeMountRoot()
	if err != nil {
		return nil, err
	}
	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return nil, err
	}
	// A unique name lets the subprocess lifecycle remove the container even when
	// the attached Docker client is killed before it can forward a signal.
	token, err := os.CreateTemp("", "cartesi-test-node-*")
	if err != nil {
		return nil, err
	}
	name := filepath.Base(token.Name())
	token.Close()
	os.Remove(token.Name())
	value = strconv.FormatUint(limit, 10)
	args := []string{"run", "--rm", "--pull=never", "--name", name,
		"--network=host", "--memory=" + value, "--memory-swap=" + value,
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--workdir", workDir, "--entrypoint", executable}
	mounts := map[string]bool{}
	mount := func(path string, readonly bool) error {
		path, err = filepath.Abs(path)
		if err != nil {
			return err
		}
		if mounts[path] {
			return nil
		}
		mounts[path] = true
		spec := "type=bind,src=" + path + ",dst=" + path
		if strings.ContainsAny(path, ",\n\r") {
			return fmt.Errorf("unsupported Docker bind path %q", path)
		}
		if readonly {
			spec += ",readonly"
		}
		args = append(args, "--mount", spec)
		return nil
	}
	for _, path := range []string{root, workDir, os.TempDir()} {
		if err := mount(path, false); err != nil {
			return nil, err
		}
	}
	if err := mount(executable, true); err != nil {
		return nil, err
	}
	// Use an owner-only environment file: Docker wrappers need not forward the
	// caller environment, and ordinary secret values stay out of process arguments.
	// Docker env files cannot represent multiline values; those use --env.
	environment := map[string]string{}
	for _, entry := range env {
		if key, val, ok := strings.Cut(entry, "="); ok {
			environment[key] = val
		}
	}
	var envFileData strings.Builder
	for key, val := range environment {
		if strings.ContainsAny(val, "\n\r") {
			args = append(args, "--env", key+"="+val)
		} else {
			fmt.Fprintf(&envFileData, "%s=%s\n", key, val)
		}
		paths := []string{}
		if key == "LD_LIBRARY_PATH" {
			paths = filepath.SplitList(val)
		}
		if strings.HasPrefix(key, "CARTESI_") && filepath.IsAbs(val) {
			paths = append(paths, val)
		}
		for _, path := range paths {
			if path == "" {
				continue
			}
			if _, err := os.Stat(path); err == nil {
				if err := mount(path, false); err != nil {
					return nil, err
				}
			}
		}
	}
	if _, err := os.Stat("/usr/share/cartesi-machine"); err == nil {
		if err := mount("/usr/share/cartesi-machine", true); err != nil {
			return nil, err
		}
	}
	image := os.Getenv("CARTESI_TEST_NODE_IMAGE")
	if image == "" {
		image = "cartesi/rollups-node:devel"
	}
	envFile, err := os.CreateTemp("", "cartesi-test-node-env-*")
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(envFile, envFileData.String()); err != nil {
		envFile.Close()
		os.Remove(envFile.Name())
		return nil, err
	}
	if err := envFile.Close(); err != nil {
		os.Remove(envFile.Name())
		return nil, err
	}
	args = append(args, "--env-file", envFile.Name(), image)
	fmt.Fprintf(log, "%s: Docker container %s, memory limit %s bytes, swap disabled\n", binary, name, value)
	cmd := exec.Command(docker, args...) //nolint:gosec
	cmd.Env = env
	return cmd, nil
}

func nodeContainerName(cmd *exec.Cmd) string {
	if len(cmd.Args) < 2 || cmd.Args[1] != "run" {
		return ""
	}
	index := slices.Index(cmd.Args, "--name")
	if index < 0 || index+1 >= len(cmd.Args) {
		return ""
	}
	name := cmd.Args[index+1]
	if !strings.HasPrefix(name, "cartesi-test-node-") {
		return ""
	}
	return name
}

func nodeMountRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found")
		}
		dir = parent
	}
}

func removeNodeEnvironment(cmd *exec.Cmd) {
	if nodeContainerName(cmd) == "" {
		return
	}
	index := slices.Index(cmd.Args, "--env-file")
	if index >= 0 && index+1 < len(cmd.Args) {
		_ = os.Remove(cmd.Args[index+1])
	}
}

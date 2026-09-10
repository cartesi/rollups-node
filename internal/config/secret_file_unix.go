// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build unix

package config

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// secretFileOpenFlags opens secret files without blocking (a read-only
// open() on a FIFO or blocking device node would otherwise wait for a
// writer, stalling startup before any log output exists) and without
// following symlinks (open fails with ELOOP instead).
const secretFileOpenFlags = os.O_RDONLY | syscall.O_NONBLOCK | syscall.O_NOFOLLOW

// checkSecretFilePermissions enforces the secret file permission policy.
// Only two canonical forms are accepted:
//
//   - Kubernetes secret volumes: root:fsGroup with mode 0440 (the file is
//     root-owned, its group is the process's effective group, and only the
//     owner and that group can read it);
//   - Compose / host files: owned by the process's user with mode 0400 or
//     0600 (owner-only; the owner write bit allows in-place rotation and
//     grants no additional read access).
//
// Anything else is rejected.
func checkSecretFilePermissions(path string, info fs.FileInfo) error {
	perm := info.Mode().Perm()
	stat := info.Sys().(*syscall.Stat_t)

	// kubernetes secret files:
	if stat.Uid == 0 && int(stat.Gid) == os.Getegid() && perm == 0o440 {
		return nil
	}

	// compose secret files:
	if int(stat.Uid) == os.Geteuid() && (perm == 0o600 || perm == 0o400) {
		return nil
	}
	return fmt.Errorf("secret file %q does not conform with uid/gid/mode rules", path)
}

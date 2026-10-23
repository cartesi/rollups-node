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

// O_NONBLOCK prevents a read-only open of a FIFO or blocking device from
// stalling startup before the logger exists. Symlinks are intentionally
// followed so Kubernetes Secret volumes work.
const secretFileOpenFlags = os.O_RDONLY | syscall.O_NONBLOCK

// checkSecretFilePermissions accepts only the two deployment forms the node
// supports:
//
//   - process-owned 0400/0600 files for local and Compose deployments;
//   - root-owned 0440/0640 files whose group is one of the process groups,
//     which is the standard Kubernetes Secret volume form.
//
// A root process may read any of those root-owned forms. Files owned by any
// other user are rejected.
func checkSecretFilePermissions(info fs.FileInfo) error {
	mode := info.Mode()
	if mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		return fmt.Errorf("%w: special mode bits are not permitted", ErrInsecureSecretFile)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: file metadata is unavailable", ErrInsecureSecretFile)
	}

	perm := mode.Perm()
	uid := int(stat.Uid)
	gid := int(stat.Gid)
	euid := os.Geteuid()
	egid := os.Getegid()

	switch {
	case euid == 0:
		if uid != 0 {
			return fmt.Errorf("%w: file is not owned by root", ErrInsecureSecretFile)
		}
		if perm != 0o400 && perm != 0o600 && perm != 0o440 && perm != 0o640 {
			return fmt.Errorf("%w: mode %04o is not permitted", ErrInsecureSecretFile, perm)
		}
	case uid == euid:
		if perm != 0o400 && perm != 0o600 {
			return fmt.Errorf("%w: mode %04o is not permitted", ErrInsecureSecretFile, perm)
		}
	case uid == 0:
		if perm != 0o440 && perm != 0o640 {
			return fmt.Errorf("%w: mode %04o is not permitted", ErrInsecureSecretFile, perm)
		}
		if !isProcessGroup(gid, egid) {
			return fmt.Errorf("%w: file group is not a process group", ErrInsecureSecretFile)
		}
	default:
		return fmt.Errorf("%w: file owner does not match the process", ErrInsecureSecretFile)
	}

	return nil
}

func isProcessGroup(gid, egid int) bool {
	if gid == egid {
		return true
	}
	groups, err := os.Getgroups()
	if err != nil {
		return false
	}
	for _, group := range groups {
		if group == gid {
			return true
		}
	}
	return false
}

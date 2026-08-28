// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

// ReadSecretFile reads the file at path and returns its contents verbatim.
//
// The path is opened once and every check (regular file, permissions) is
// performed on the opened descriptor, so validation always applies to the
// object that is actually read. On POSIX the open is non-blocking and does
// not follow symlinks (see secretFileOpenFlags), so a FIFO, blocking
// device node, or symlink at the path is rejected instead of stalling
// startup. The permission policy is enforced on POSIX systems (see
// checkSecretFilePermissions); on other platforms only the regular-file
// check applies.
//
// Errors report the path, never the file contents.
func ReadSecretFile(path string) ([]byte, error) {
	f, err := os.OpenFile(path, secretFileOpenFlags, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("secret file %q is a symlink", path)
		}
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("secret file %q is not a regular file", path)
	}
	if err := checkSecretFilePermissions(path, info); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

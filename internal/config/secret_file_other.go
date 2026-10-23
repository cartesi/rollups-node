// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build !unix

package config

import (
	"io/fs"
	"os"
)

// O_NONBLOCK is not available on non-Unix platforms.
const secretFileOpenFlags = os.O_RDONLY

// checkSecretFilePermissions is a no-op where POSIX mode and ownership
// semantics are unavailable. readSecretFile still enforces the regular-file
// and size checks.
func checkSecretFilePermissions(_ fs.FileInfo) error {
	return nil
}

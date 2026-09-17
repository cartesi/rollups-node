// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build !unix

package config

import (
	"io/fs"
	"os"
)

// secretFileOpenFlags: O_NONBLOCK does not exist on non-POSIX platforms.
const secretFileOpenFlags = os.O_RDONLY

// checkSecretFilePermissions is a no-op on platforms without POSIX mode
// semantics; the regular-file check in ReadSecretFile still applies.
func checkSecretFilePermissions(_ string, _ fs.FileInfo) error {
	return nil
}

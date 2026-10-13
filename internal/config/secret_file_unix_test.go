// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build linux || darwin

package config

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// A read-only open() on a FIFO blocks until a writer appears. Config is
// loaded before the logger exists, so a FIFO at a _FILE path must be
// rejected promptly instead of stalling startup with no output.
func TestReadSecretFileRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	_, err := ReadSecretFile(path)
	require.ErrorContains(t, err, "not a regular file")
}

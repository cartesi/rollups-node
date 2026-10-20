// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSecretFile(t *testing.T, mode os.FileMode, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte(contents), mode))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

func TestReadSecretFileRegularFile(t *testing.T) {
	const contents = "top secret value\n"

	for _, mode := range []os.FileMode{0o400, 0o600} {
		t.Run(mode.String(), func(t *testing.T) {
			path := writeSecretFile(t, mode, contents)
			data, err := readSecretFile(path)
			require.NoError(t, err)
			require.Equal(t, contents, string(data))
		})
	}
}

func TestReadSecretFileEmptyPath(t *testing.T) {
	_, err := readSecretFile("")
	require.ErrorIs(t, err, ErrNotDefined)
}

func TestReadSecretFileMissingFileDoesNotExposePath(t *testing.T) {
	const marker = "SYNTHETIC_MISSING_SECRET_PATH_7612094"
	_, err := readSecretFile(filepath.Join(t.TempDir(), marker))
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.NotContains(t, err.Error(), marker)
	var pathErr *fs.PathError
	require.False(t, errors.As(err, &pathErr))
}

func TestReadSecretFileRejectsNonRegularFile(t *testing.T) {
	_, err := readSecretFile(t.TempDir())
	require.ErrorIs(t, err, ErrInsecureSecretFile)
}

func TestReadSecretFileRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(maxSecretFileSize+1))
	require.NoError(t, f.Close())
	require.NoError(t, os.Chmod(path, 0o600))

	_, err = readSecretFile(path)
	require.ErrorIs(t, err, ErrInsecureSecretFile)
	require.ErrorContains(t, err, "exceeds")
}

func TestFileBackedValueUnset(t *testing.T) {
	resetSecretConfig(t)
	t.Setenv(AUTH_MNEMONIC, "")
	t.Setenv(AUTH_MNEMONIC_FILE, "")

	_, err := GetAuthMnemonic()
	require.ErrorIs(t, err, ErrNotDefined)
}

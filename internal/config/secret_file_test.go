// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func writeSecretFile(t *testing.T, mode os.FileMode, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte(contents), mode))
	return path
}

// skipIfNoPermissionPolicy skips tests that depend on the permission policy,
// which the test suite only exercises on linux and darwin.
func skipIfNoPermissionPolicy(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("permission policy tests require linux or darwin, running on %s", runtime.GOOS)
	}
}

func TestReadSecretFileRegularFile(t *testing.T) {
	const contents = "top secret value\n"

	t.Run("returns contents verbatim", func(t *testing.T) {
		path := writeSecretFile(t, 0o400, contents)
		data, err := ReadSecretFile(path)
		require.NoError(t, err)
		require.Equal(t, contents, string(data))
	})

	t.Run("rejects missing file", func(t *testing.T) {
		_, err := ReadSecretFile(filepath.Join(t.TempDir(), "missing"))
		require.Error(t, err)
	})

	t.Run("rejects directory", func(t *testing.T) {
		_, err := ReadSecretFile(t.TempDir())
		require.ErrorContains(t, err, "not a regular file")
	})

	t.Run("rejects symlink even to a canonical target", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("creating symlinks requires elevated privileges on windows")
		}
		target := writeSecretFile(t, 0o400, "linked secret")
		link := filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.Symlink(target, link))
		_, err := ReadSecretFile(link)
		require.ErrorContains(t, err, "is a symlink")
	})
}

func TestReadSecretFilePermissions(t *testing.T) {
	skipIfNoPermissionPolicy(t)
	const contents = "top secret value\n"

	t.Run("accepts 0400 owned by the process user (compose form)", func(t *testing.T) {
		path := writeSecretFile(t, 0o400, contents)
		data, err := ReadSecretFile(path)
		require.NoError(t, err)
		require.Equal(t, contents, string(data))
	})

	t.Run("accepts 0440 root:process-group (kubernetes form)", func(t *testing.T) {
		path := writeSecretFile(t, 0o644, contents)
		if err := os.Chown(path, 0, os.Getegid()); err != nil {
			t.Skipf("cannot chown to root: %v", err)
		}
		require.NoError(t, os.Chmod(path, 0o440))
		_, err := ReadSecretFile(path)
		require.NoError(t, err)
	})

	t.Run("accepts 0600 owned by the process user (compose form, owner-writable)", func(t *testing.T) {
		path := writeSecretFile(t, 0o600, contents)
		data, err := ReadSecretFile(path)
		require.NoError(t, err)
		require.Equal(t, contents, string(data))
	})

	t.Run("rejects 0411 (owner exec bit is not a canonical form)", func(t *testing.T) {
		path := writeSecretFile(t, 0o411, contents)
		_, err := ReadSecretFile(path)
		require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
	})

	t.Run("rejects world-readable 0644", func(t *testing.T) {
		path := writeSecretFile(t, 0o644, contents)
		_, err := ReadSecretFile(path)
		require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
	})

	t.Run("rejects world-accessible 0755", func(t *testing.T) {
		path := writeSecretFile(t, 0o755, contents)
		_, err := ReadSecretFile(path)
		require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
	})

	t.Run("rejects group-readable 0640", func(t *testing.T) {
		path := writeSecretFile(t, 0o640, contents)
		_, err := ReadSecretFile(path)
		require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
	})

	t.Run("rejects 0440 owned by the process user (kubernetes files are root-owned)", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("as root, root:root 0440 is the kubernetes form")
		}
		path := writeSecretFile(t, 0o440, contents)
		_, err := ReadSecretFile(path)
		require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
	})

	t.Run("rejects 0440 root:other-group", func(t *testing.T) {
		path := writeSecretFile(t, 0o644, contents)
		if err := os.Chown(path, 0, 1); err != nil {
			t.Skipf("cannot chown to root: %v", err)
		}
		require.NoError(t, os.Chmod(path, 0o440))
		_, err := ReadSecretFile(path)
		require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
	})

	t.Run("rejects 0400 owned by another user", func(t *testing.T) {
		path := writeSecretFile(t, 0o644, contents)
		if err := os.Chown(path, 1, 1); err != nil {
			t.Skipf("cannot chown to another user: %v", err)
		}
		require.NoError(t, os.Chmod(path, 0o400))
		_, err := ReadSecretFile(path)
		require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
	})

	t.Run("error does not leak file contents", func(t *testing.T) {
		const secret = "TOP-SECRET-VALUE-123"
		path := writeSecretFile(t, 0o644, secret)
		_, err := ReadSecretFile(path)
		require.Error(t, err)
		require.NotContains(t, err.Error(), secret)
	})
}

func TestGetAuthMnemonicFromFile(t *testing.T) {
	viper.Reset()
	viper.AutomaticEnv() // Reset drops the package init's AutomaticEnv registration
	t.Setenv(AUTH_MNEMONIC, "")
	t.Setenv(AUTH_MNEMONIC_FILE, writeSecretFile(t, 0o400, "mnemonic words\n"))
	mnemonic, err := GetAuthMnemonic()
	require.NoError(t, err)
	require.Equal(t, "mnemonic words", mnemonic.Value)
}

func TestGetAuthMnemonicFromFileRejectsInsecurePermissions(t *testing.T) {
	skipIfNoPermissionPolicy(t)
	viper.Reset()
	viper.AutomaticEnv() // Reset drops the package init's AutomaticEnv registration
	t.Setenv(AUTH_MNEMONIC, "")
	t.Setenv(AUTH_MNEMONIC_FILE, writeSecretFile(t, 0o644, "mnemonic words\n"))
	_, err := GetAuthMnemonic()
	require.ErrorContains(t, err, "does not conform with uid/gid/mode rules")
}

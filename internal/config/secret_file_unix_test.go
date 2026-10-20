// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build unix

package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeFileInfo struct {
	mode fs.FileMode
	stat syscall.Stat_t
	size int64
}

func (f fakeFileInfo) Name() string       { return "redacted" }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return &f.stat }

func foreignGroupID(t *testing.T) int {
	t.Helper()
	groups, err := os.Getgroups()
	require.NoError(t, err)
	for gid := 1; gid < 1<<16; gid++ {
		if gid == os.Getegid() {
			continue
		}
		if !slices.Contains(groups, gid) {
			return gid
		}
	}
	t.Fatal("could not find a group outside the process groups")
	return 0
}

func TestCheckSecretFilePermissions(t *testing.T) {
	euid := os.Geteuid()
	egid := os.Getegid()
	otherUID := euid + 1
	if otherUID == 0 {
		otherUID = 1
	}

	tests := []struct {
		name  string
		uid   int
		gid   int
		mode  fs.FileMode
		valid bool
	}{
		{name: "process 0400", uid: euid, gid: egid, mode: 0o400, valid: true},
		{name: "process 0600", uid: euid, gid: egid, mode: 0o600, valid: true},
		{name: "process 0644", uid: euid, gid: egid, mode: 0o644},
		{name: "process 0640", uid: euid, gid: egid, mode: 0o640, valid: euid == 0},
		{name: "process owner execute", uid: euid, gid: egid, mode: 0o500},
		{name: "process no read", uid: euid, gid: egid, mode: 0o200},
		{name: "root 0440 process group", uid: 0, gid: egid, mode: 0o440, valid: true},
		{name: "root 0640 process group", uid: 0, gid: egid, mode: 0o640, valid: true},
		{name: "root 0400", uid: 0, gid: egid, mode: 0o400, valid: euid == 0},
		{name: "root group write", uid: 0, gid: egid, mode: 0o460},
		{name: "unexpected owner", uid: otherUID, gid: egid, mode: 0o400},
		{name: "special mode bit", uid: euid, gid: egid, mode: 0o400 | fs.ModeSetuid},
	}

	if euid != 0 {
		tests = append(tests,
			struct {
				name  string
				uid   int
				gid   int
				mode  fs.FileMode
				valid bool
			}{
				name: "root 0440 foreign group",
				uid:  0,
				gid:  foreignGroupID(t),
				mode: 0o440,
			},
		)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSecretFilePermissions(fakeFileInfo{
				mode: tt.mode,
				stat: syscall.Stat_t{Uid: uint32(tt.uid), Gid: uint32(tt.gid)},
			})
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrInsecureSecretFile)
			}
		})
	}
}

func TestReadSecretFileFollowsKubernetesSymlinks(t *testing.T) {
	root := t.TempDir()
	version := filepath.Join(root, "..2026_01_01_00_00_00.000000000")
	require.NoError(t, os.Mkdir(version, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(version, "key"), []byte("secret"), 0o400))
	require.NoError(t, os.Chmod(filepath.Join(version, "key"), 0o400))
	require.NoError(t, os.Symlink(filepath.Base(version), filepath.Join(root, "..data")))
	require.NoError(t, os.Symlink(filepath.Join("..data", "key"), filepath.Join(root, "key")))

	contents, err := readSecretFile(filepath.Join(root, "key"))
	require.NoError(t, err)
	require.Equal(t, "secret", string(contents))
}

func TestReadSecretFileRejectsInsecureSymlinkTarget(t *testing.T) {
	target := writeSecretFile(t, 0o644, "secret")
	link := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.Symlink(target, link))

	_, err := readSecretFile(link)
	require.ErrorIs(t, err, ErrInsecureSecretFile)
}

func TestReadSecretFileRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	require.NoError(t, syscall.Mkfifo(path, 0o600))

	_, err := readSecretFile(path)
	require.ErrorIs(t, err, ErrInsecureSecretFile)
	require.ErrorContains(t, err, "not a regular file")
}

func TestReadSecretFilePermissionErrorDoesNotExposeInput(t *testing.T) {
	const pathMarker = "SYNTHETIC_INSECURE_FILE_PATH_4019283"
	const contentsMarker = "SYNTHETIC_INSECURE_FILE_CONTENT_8401932"
	dir := t.TempDir()
	path := filepath.Join(dir, pathMarker)
	require.NoError(t, os.WriteFile(path, []byte(contentsMarker), 0o644))
	require.NoError(t, os.Chmod(path, 0o644))

	_, err := readSecretFile(path)
	require.ErrorIs(t, err, ErrInsecureSecretFile)
	require.NotContains(t, err.Error(), pathMarker)
	require.NotContains(t, err.Error(), contentsMarker)
}

func TestFileGettersRejectInsecurePermissions(t *testing.T) {
	path := writeSecretFile(t, 0o644, "value")
	tests := []struct {
		name string
		get  func() error
	}{
		{AUTH_MNEMONIC, func() error { _, err := GetAuthMnemonic(); return err }},
		{AUTH_PRIVATE_KEY, func() error { _, err := GetAuthPrivateKey(); return err }},
		{PRT_AUTH_MNEMONIC, func() error { _, err := GetPrtAuthMnemonic(); return err }},
		{PRT_AUTH_PRIVATE_KEY, func() error { _, err := GetPrtAuthPrivateKey(); return err }},
		{DATABASE_CONNECTION, func() error { _, err := GetDatabaseConnection(); return err }},
		{BLOCKCHAIN_HTTP_ENDPOINT, func() error { _, err := GetBlockchainHttpEndpoint(); return err }},
		{BLOCKCHAIN_HTTP_AUTHORIZATION, func() error { _, err := GetBlockchainHttpAuthorization(); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetSecretConfig(t)
			t.Setenv(tt.name, "")
			t.Setenv(tt.name+"_FILE", path)

			err := tt.get()
			require.ErrorIs(t, err, ErrInsecureSecretFile)
		})
	}
}

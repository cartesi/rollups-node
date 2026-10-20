// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// ErrInsecureSecretFile marks a *_FILE value that was rejected by the secret
// file policy. Errors wrap it so callers can use errors.Is.
var ErrInsecureSecretFile = errors.New("insecure secret file")

// 1 MiB matches the maximum size of a Kubernetes Secret.
const maxSecretFileSize = 1 << 20

// readSecretFile opens path once, validates the opened descriptor, and returns
// its contents verbatim. Validation and reading use the same descriptor, so a
// path replaced after open cannot change which file is read.
//
// Symlinks are followed deliberately: Kubernetes Secret volumes expose each
// key as a symlink into the kubelet's versioned ..data directory. The target
// descriptor is what the policy validates.
//
// Errors identify the violated rule, never the path or the file contents.
func readSecretFile(path string) ([]byte, error) {
	if path == "" {
		return nil, ErrNotDefined
	}

	f, err := os.OpenFile(path, secretFileOpenFlags, 0)
	if err != nil {
		return nil, safeFileError(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, safeFileError(err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file", ErrInsecureSecretFile)
	}
	if info.Size() > maxSecretFileSize {
		return nil, fmt.Errorf("%w: file exceeds %d bytes", ErrInsecureSecretFile, maxSecretFileSize)
	}
	if err := checkSecretFilePermissions(info); err != nil {
		return nil, err
	}

	contents, err := io.ReadAll(io.LimitReader(f, maxSecretFileSize+1))
	if err != nil {
		return nil, safeFileError(err)
	}
	if len(contents) > maxSecretFileSize {
		return nil, fmt.Errorf("%w: file exceeds %d bytes", ErrInsecureSecretFile, maxSecretFileSize)
	}
	return contents, nil
}

// safeFileError removes the path from filesystem errors. An operator may
// accidentally put a secret value in a *_FILE variable instead of a filename.
func safeFileError(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

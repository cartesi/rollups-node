// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// anvilHomeDir is the private HOME of a test-owned Anvil (see anvilEnvironment).
const anvilHomeDir = "anvil-home"

// zstdWindowSize lets the encoder find the long repeats between the
// historical states of an Anvil dump (similar to "zstd --long=27").
const zstdWindowSize = 1 << 27

func sha256File(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func compressZstd(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644) //nolint:mnd
	if err != nil {
		return err
	}
	encoder, err := zstd.NewWriter(out,
		zstd.WithEncoderLevel(zstd.SpeedBetterCompression),
		zstd.WithWindowSize(zstdWindowSize))
	if err != nil {
		out.Close()
		return err
	}
	if _, err := io.Copy(encoder, in); err != nil {
		encoder.Close()
		out.Close()
		return fmt.Errorf("compressing %s: %w", src, err)
	}
	if err := encoder.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func decompressZstd(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	decoder, err := zstd.NewReader(in, zstd.WithDecoderMaxWindow(zstdWindowSize))
	if err != nil {
		return err
	}
	defer decoder.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644) //nolint:mnd
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, decoder); err != nil {
		out.Close()
		return fmt.Errorf("decompressing %s: %w", src, err)
	}
	return out.Close()
}

// copyTree copies a directory (for example, a stored machine) without
// following symbolic links.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700) //nolint:mnd
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported file type in %s: %s", src, path)
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

var storedHashPattern = regexp.MustCompile(`0x[0-9a-fA-F]{64}`)

// storedMachineHash runs cartesi-machine-stored-hash on a stored machine.
func storedMachineHash(dir string) (string, error) {
	bin, err := exec.LookPath("cartesi-machine-stored-hash")
	if err != nil {
		return "", fmt.Errorf("cartesi-machine-stored-hash not found on PATH: %w", err)
	}
	out, err := exec.Command(bin, dir).Output()
	if err != nil {
		return "", fmt.Errorf("cartesi-machine-stored-hash %s: %w", dir, err)
	}
	match := storedHashPattern.FindString(string(out))
	if match == "" {
		return "", fmt.Errorf("cartesi-machine-stored-hash printed no hash: %q", strings.TrimSpace(string(out)))
	}
	return strings.ToLower(match), nil
}

// toolVersion returns the first line printed by "tool --version", or "".
func toolVersion(tool string) string {
	out, err := exec.Command(tool, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line
}

// gitRevision returns the HEAD revision of dir and whether the tree is dirty.
func gitRevision(dir string) (string, bool, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false, fmt.Errorf("git rev-parse in %s: %w", dir, err)
	}
	status, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--untracked-files=no").Output()
	if err != nil {
		return "", false, fmt.Errorf("git status in %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), len(strings.TrimSpace(string(status))) > 0, nil
}

// anvilEnvironment gives a test-owned Anvil its own HOME below dir.
//
// Anvil 1.5.1 keeps the historical states of a loaded dump in a disk cache
// under $HOME/.foundry/anvil/tmp. When it cannot create that directory, it
// drops every historical state without an error, and reads below the head
// fail with BlockOutOfRangeError. A private HOME makes the cache writable and
// keeps its (large) files inside the run directory.
func anvilEnvironment(dir string) ([]string, error) {
	home := filepath.Join(dir, anvilHomeDir)
	if err := os.MkdirAll(home, 0o700); err != nil { //nolint:mnd
		return nil, err
	}
	return mergeEnvironment(os.Environ(), nil, map[string]string{"HOME": home}), nil
}

func isMissingOrEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	return errors.Is(err, os.ErrNotExist) || (err == nil && len(entries) == 0)
}

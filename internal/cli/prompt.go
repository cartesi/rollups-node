// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrNonInteractive is returned when a confirmation prompt is attempted
// on a non-interactive (piped/redirected) stdin without using --yes.
var ErrNonInteractive = errors.New(
	"cannot prompt for confirmation: stdin is not a terminal. Use --yes to skip prompts")

// ConfirmPrompt prints the given message followed by " [y/N]: " and reads user input from stdin.
// Returns true if the user answers "y" or "yes" (case-insensitive), false otherwise.
// Returns ErrNonInteractive if stdin is not a terminal (e.g. piped in a script),
// or a wrapped I/O error if reading from stdin fails.
func ConfirmPrompt(message string) (bool, error) {
	if !IsTerminal(os.Stdin) {
		return false, ErrNonInteractive
	}
	fmt.Printf("%s [y/N]: ", message)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("reading input: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

// IsTerminal reports whether the given file is connected to a terminal.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

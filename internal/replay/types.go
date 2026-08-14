// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package replay reconstructs and verifies Cartesi machines from persisted
// canonical input outcomes. Repository evidence access is defined by
// repository.ReplayRepository; machine lifecycle remains caller responsibility.
package replay

import (
	"errors"
	"fmt"
)

var (
	// ErrContradiction identifies a deterministic difference between a
	// persisted result and its replay.
	ErrContradiction = errors.New("replay contradicts persisted canonical result")

	// ErrExecution identifies a failure to reconstruct the requested range.
	ErrExecution = errors.New("failed to replay machine execution")

	// ErrInvalidOptions identifies an invalid replay request.
	ErrInvalidOptions = errors.New("invalid replay options")
)

// ContradictionError identifies the first deterministic difference between a
// persisted result and its replay. Byte values are represented by lengths and
// digests, so the diagnostic never exposes input, output, or report payloads.
type ContradictionError struct {
	Application string
	EpochIndex  *uint64
	InputIndex  uint64
	Field       string
	Expected    string
	Actual      string
}

func (e *ContradictionError) Error() string {
	epochIndex := "<unknown>"
	if e.EpochIndex != nil {
		epochIndex = fmt.Sprint(*e.EpochIndex)
	}
	return fmt.Sprintf(
		"%v: app=%q epoch=%s input=%d field=%s expected=%s actual=%s",
		ErrContradiction,
		e.Application,
		epochIndex,
		e.InputIndex,
		e.Field,
		e.Expected,
		e.Actual,
	)
}

func (e *ContradictionError) Unwrap() error { return ErrContradiction }

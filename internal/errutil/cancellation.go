// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package errutil provides error classification shared by node components.
package errutil

import (
	"context"
	"errors"
)

// IsOnlyCancellation reports whether every cause of err is context.Canceled.
// Unlike errors.Is, it does not hide other causes in a joined error. Nil is not
// a cancellation. Errors without children are checked with errors.Is. Callers
// must separately establish that the service is shutting down.
func IsOnlyCancellation(err error) bool {
	switch cause := err.(type) {
	case interface{ Unwrap() []error }:
		children := cause.Unwrap()
		if len(children) == 0 {
			return errors.Is(err, context.Canceled)
		}
		for _, child := range children {
			if !IsOnlyCancellation(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		if child := cause.Unwrap(); child != nil {
			return IsOnlyCancellation(child)
		}
	}
	return errors.Is(err, context.Canceled)
}

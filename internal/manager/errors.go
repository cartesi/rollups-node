// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"errors"
	"fmt"
)

var ErrApplicationFailureNotDurable = errors.New("application failure status is not durably confirmed")

// ApplicationFailurePersistenceError means local machine work is fenced but
// the repository has not confirmed the corresponding FAILED (or a stronger
// terminal/deleted) application state.
type ApplicationFailurePersistenceError struct {
	ApplicationID int64
	WriteErr      error
	ReadErr       error
}

func (e *ApplicationFailurePersistenceError) Error() string {
	if e.ReadErr != nil {
		return fmt.Sprintf(
			"%v: application=%d write_error=%v read_error=%v",
			ErrApplicationFailureNotDurable, e.ApplicationID, e.WriteErr, e.ReadErr,
		)
	}
	return fmt.Sprintf(
		"%v: application=%d write_error=%v durable status remains unconfirmed",
		ErrApplicationFailureNotDurable, e.ApplicationID, e.WriteErr,
	)
}

func (e *ApplicationFailurePersistenceError) Unwrap() []error {
	errList := []error{ErrApplicationFailureNotDurable}
	if e.WriteErr != nil {
		errList = append(errList, e.WriteErr)
	}
	if e.ReadErr != nil {
		errList = append(errList, e.ReadErr)
	}
	return errList
}

// IsOnlyApplicationFailurePersistenceErrors reports whether err is one or
// more application-local durability failures and contains no global failure.
// It deliberately examines joined top-level errors without descending into a
// persistence error's write/read causes.
func IsOnlyApplicationFailurePersistenceErrors(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*ApplicationFailurePersistenceError); ok {
		return true
	}

	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		children := wrapped.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !IsOnlyApplicationFailurePersistenceErrors(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return IsOnlyApplicationFailurePersistenceErrors(wrapped.Unwrap())
	default:
		return false
	}
}

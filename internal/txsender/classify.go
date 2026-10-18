// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/errutil"
)

// Keep shutdown cancellation pure. Joining a wait sentinel would make the
// service mistake cancellation of an in-flight RPC for an operational failure.
func inconclusive(err error) error {
	if errutil.IsOnlyCancellation(err) {
		return err
	}
	return errors.Join(ErrInconclusive, err)
}

// HardFailure reports a protected-state contradiction that stops publication
// until an operator restarts the service.
func HardFailure(err error) bool { return errors.Is(err, ErrFinality) || errors.Is(err, ErrContent) }

// PassInvalid reports an error that invalidates the rest of a publication pass.
func PassInvalid(err error) bool {
	return errors.Is(err, ErrStale) || errors.Is(err, ErrNonCanonical) ||
		HardFailure(err) || errors.Is(err, ErrUnsupported)
}

// WaitOnly reports whether every cause of err is an ordinary wait. A joined
// RPC or database failure remains an error. Callers may name extra waits.
func WaitOnly(err error, extra ...error) bool {
	switch cause := err.(type) {
	case nil:
		return false
	case *PublicationError:
		return cause.NonceLowOnly()
	case interface{ Unwrap() []error }:
		children := cause.Unwrap()
		for _, child := range children {
			if !WaitOnly(child, extra...) {
				return false
			}
		}
		return len(children) > 0
	case interface{ Unwrap() error }:
		return WaitOnly(cause.Unwrap(), extra...)
	}
	for _, wait := range append([]error{ErrHeld, ErrStale, ErrInconclusive, ErrIneligible}, extra...) {
		if err == wait {
			return true
		}
	}
	return false
}

// ShutdownOnly reports whether shutdown interrupted an otherwise ordinary wait.
// Every joined cause must be cancellation or a named wait; real failures and
// expired operation deadlines remain visible even when shutdown has started.
func ShutdownOnly(ctx context.Context, err error, extra ...error) bool {
	if ctx.Err() != context.Canceled || err == nil {
		return false
	}
	return shutdownCauseOnly(err, extra...)
}

func shutdownCauseOnly(err error, extra ...error) bool {
	if err == context.Canceled || WaitOnly(err, extra...) {
		return true
	}
	switch cause := err.(type) {
	case interface{ Unwrap() []error }:
		children := cause.Unwrap()
		for _, child := range children {
			if !shutdownCauseOnly(child, extra...) {
				return false
			}
		}
		return len(children) > 0
	case interface{ Unwrap() error }:
		return shutdownCauseOnly(cause.Unwrap(), extra...)
	default:
		return false
	}
}

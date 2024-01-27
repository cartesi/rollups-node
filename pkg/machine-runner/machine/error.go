// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import "errors"

const unreachable = "internal error: entered unreacheable code"

var (
	ErrReachedLimitCycles = errors.New("reached limit cycles")

	ErrFailed        = errors.New("machine failed")
	ErrHalted        = errors.New("machine halted")
	ErrYieldedSoftly = errors.New("machine yielded softly")

	// Load (isPrimed) errors
	ErrNotAtManualYield            = errors.New("not at manual yield")
	ErrLastInputWasRejected        = errors.New("last input was rejected")
	ErrLastInputYieldedAnException = errors.New("last input yielded an exception")

	// Fork errors
	ErrOrphanFork = errors.New("forked cartesi machine was left orphan")

	// Destroy() errors
	ErrRemoteShutdown = errors.New("could not shut down the remote machine")
	ErrMachineDestroy = errors.New("could not destroy the inner machine")
)

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

	// isPrimed() errors
	ErrNotAtManualYield            = errors.New("not at manual yield")
	ErrLastInputWasRejected        = errors.New("last input was rejected")
	ErrLastInputYieldedAnException = errors.New("last input yielded an exception")

	// Destroy() errors
	ErrRemoteShutdown = errors.New("could not shut down the machine")
	ErrBindingDestroy = errors.New("could not destroy the machine binding")
)

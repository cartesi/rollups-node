// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/node/model"
)

const unreachable = "internal error: entered unreacheable code"

var (
	ErrCartesiMachine = errors.New("cartesi machine internal error")

	// Misc.
	ErrException            = errors.New("last request yielded an exception")
	ErrHalted               = errors.New("machine halted")
	ErrProgress             = errors.New("machine yielded progress")
	ErrSoftYield            = errors.New("machine yielded softly")
	ErrCycleLimitExceeded   = errors.New("cycle limit exceeded")
	ErrOutputsLimitExceeded = errors.New("outputs length limit exceeded")
	// ErrPayloadLengthLimitExceeded = errors.New("payload length limit exceeded")

	ErrOrphanServer = errors.New("cartesi machine server was left orphan")

	// Load
	ErrNotAtManualYield = errors.New("not at manual yield")

	// Advance
	ErrHashLength = fmt.Errorf("hash does not have exactly %d bytes", model.HashLength)
)

func errOrphanServerWithAddress(address string) error {
	return fmt.Errorf("%w at address %s", ErrOrphanServer, address)
}

func errCartesiMachine(err error) error {
	return errors.Join(ErrCartesiMachine, err)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
)

var (
	ErrUnreachable = errors.New("internal error: entered unreachable code")

	ErrException                  = errors.New("last request yielded an exception")
	ErrHalted                     = errors.New("machine halted")
	ErrProgress                   = errors.New("machine yielded progress")
	ErrSoftYield                  = errors.New("machine yielded softly")
	ErrOutputsLimitExceeded       = errors.New("outputs limit exceeded")
	ErrCycleLimitExceeded         = errors.New("cycle limit exceeded")
	ErrPayloadLengthLimitExceeded = errors.New("payload length limit exceeded")

	// Load
	ErrNotAtManualYield = errors.New("not at manual yield")

	// Advance
	ErrHashLength = fmt.Errorf("hash does not have exactly %d bytes", model.HashLength)
)

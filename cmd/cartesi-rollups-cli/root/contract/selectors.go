// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"errors"
)

// errFound is a sentinel error used to short-circuit FindTransitions
// after the target item has been located.
var errFound = errors.New("found")

// errLimitReached is a sentinel error used to short-circuit FindTransitions
// after the configured event limit has been reached, avoiding unnecessary
// RPC calls for remaining transition blocks.
var errLimitReached = errors.New("limit reached")

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"errors"

	"github.com/cartesi/rollups-node/pkg/ethutil"
)

// tournamentFailedNoWinner is the 4-byte error selector for TournamentFailedNoWinner.
// When ArbitrationResult reverts with this selector, the tournament finished with all
// participants eliminated.
const tournamentFailedNoWinner = "0xb3045ef8"

// errFound is a sentinel error used to short-circuit FindTransitions
// after the target item has been located.
var errFound = errors.New("found")

// errLimitReached is a sentinel error used to short-circuit FindTransitions
// after the configured event limit has been reached, avoiding unnecessary
// RPC calls for remaining transition blocks.
var errLimitReached = errors.New("limit reached")

// matchesSelector delegates to ethutil.MatchesSelector.
func matchesSelector(data any, selector string) bool {
	return ethutil.MatchesSelector(data, selector)
}

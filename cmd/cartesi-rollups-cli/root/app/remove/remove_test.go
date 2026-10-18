// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package remove

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemovalPromptSkipDoesNotAcknowledgeAbandonment(t *testing.T) {
	oldYes, oldAllow := yesFlag, allowAbandonment
	t.Cleanup(func() { yesFlag, allowAbandonment = oldYes, oldAllow })
	for _, skipPrompt := range []bool{false, true} {
		yesFlag, allowAbandonment = skipPrompt, false
		require.ErrorContains(t, checkAcknowledgment(), "--allow-abandonment")
		allowAbandonment = true
		require.NoError(t, checkAcknowledgment())
	}
}

func TestRemovalRequiresAcknowledgmentBeforeOpeningDatabase(t *testing.T) {
	oldAllow, oldTimeout := allowAbandonment, removalTimeout
	t.Cleanup(func() { allowAbandonment, removalTimeout = oldAllow, oldTimeout })
	allowAbandonment, removalTimeout = false, defaultRemovalTimeout
	require.ErrorContains(t, run(Cmd, []string{"echo-dapp"}), "--allow-abandonment")
}

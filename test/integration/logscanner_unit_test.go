// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntegrationLogScanner(t *testing.T) {
	const anvilError = "BlockOutOfRangeError: block height is 3051 but requested was 3040"
	for _, tc := range []struct {
		name    string
		message string
		allow   bool
		matched bool
	}{
		{name: "Anvil error with allowance", message: anvilError, allow: true, matched: true},
		{name: "Anvil error without allowance", message: anvilError},
		{name: "shutdown cancellation", message: "context canceled", allow: true},
		{name: "query deadline", message: "context deadline exceeded", allow: true},
		{name: "database error", message: "connection refused", allow: true},
		{name: "provider error", message: "missing trie node", allow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().Truncate(time.Millisecond)
			line := now.Format(nodeLogTimeFmt) + ` ERR Tick service=claimer error="` + tc.message + `"`
			path := filepath.Join(t.TempDir(), "node.log")
			require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0600))
			t.Setenv("CARTESI_TEST_NODE_LOG_FILE", path)

			var checker LogChecker
			if tc.allow {
				checker.SetExpectedLogs(t, anvilBlockOutOfRangeAllowlist)
			}
			unexpected, unmatched := scanNodeLogsBetween(t, now, now, checker.expectedLogs)
			if tc.matched {
				require.Empty(t, unexpected)
			} else {
				require.Equal(t, []string{line}, unexpected)
			}
			if tc.allow && !tc.matched {
				require.Equal(t, []int{0}, unmatched)
			} else {
				require.Empty(t, unmatched)
			}
		})
	}
}

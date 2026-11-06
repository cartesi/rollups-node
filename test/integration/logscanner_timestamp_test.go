// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntegrationLogScannerTimezoneOffsets(t *testing.T) {
	instant := time.Date(2026, 11, 6, 21, 59, 51, 819000000, time.FixedZone("host", -3*60*60))
	for _, offset := range []int{0, -3 * 60 * 60, 5*60*60 + 30*60} {
		stamp := instant.In(time.FixedZone("node", offset)).Format(nodeLogTimeFmt)
		t.Run(stamp, func(t *testing.T) {
			line := stamp + " ERR expected diagnostic service=claimer"
			parsed, ok := parseLogTimestamp(line)
			require.True(t, ok)
			require.True(t, instant.Equal(parsed))
			level, ok := parseLogLevel(line)
			require.True(t, ok)
			require.Equal(t, LevelError, level)

			path := filepath.Join(t.TempDir(), "node.log")
			require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0600))
			t.Setenv("CARTESI_TEST_NODE_LOG_FILE", path)
			expected := []ExpectedLog{{Pattern: regexp.MustCompile("expected diagnostic"), Level: LevelError, Required: true}}
			unexpected, unmatched := scanNodeLogsBetween(t, instant.Add(-time.Second), instant.Add(time.Second), expected)
			require.Empty(t, unexpected)
			require.Empty(t, unmatched)
			unexpected, unmatched = scanNodeLogsBetween(t, instant.Add(time.Hour), instant.Add(2*time.Hour), expected)
			require.Empty(t, unexpected)
			require.Equal(t, []int{0}, unmatched)
		})
	}
}

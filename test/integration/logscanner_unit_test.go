// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lmittmann/tint"
	"github.com/stretchr/testify/require"
)

func TestIntegrationLogScanner(t *testing.T) {
	t.Run("PRT protection stop identity", testPrtProtectionStopLogs)
	t.Run("PRT protection stop reporter", testPrtProtectionStopReporter)
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

func testPrtProtectionStopReporter(t *testing.T) {
	const cause = "epoch 2: invalid root"
	app := &model.Application{
		Name: "stopped-prt-ABC", IApplicationAddress: common.HexToAddress("0xfc098287aa9dae54e2505e3793558f74745dc0c2"),
	}
	root := common.HexToAddress("0xa5012ed4755e2769361ba032b8612d9afe351f56")
	require.NotEqual(t, strings.ToLower(app.IApplicationAddress.Hex()), app.IApplicationAddress.Hex())
	require.NotEqual(t, strings.ToLower(root.Hex()), root.Hex())
	for _, color := range []bool{false, true} {
		for _, tc := range []struct {
			name, source string
			available    bool
		}{
			{name: "pinned", source: prtPinnedClockSource, available: true},
			{name: "stored", source: prtStoredClockSource, available: true},
			{name: "stored before projection", source: prtStoredClockSource},
		} {
			t.Run(fmt.Sprintf("%s/color=%t", tc.name, color), func(t *testing.T) {
				var output bytes.Buffer
				logger := slog.New(tint.NewTextHandler(&output, &tint.Options{NoColor: !color, TimeFormat: nodeLogTimeFmt})).
					With("service", "prt")
				clock := prtprotection.Clock{EpochIndex: 2, Root: root, Source: tc.source}
				if tc.source == prtPinnedClockSource {
					clock.Source += " at block 90 (" + common.Hash{}.Hex() + ")"
				}
				if tc.available {
					clock.ClosingBlock = new(uint64(123))
				} else {
					clock.Err = errors.New("root descriptor is not stored")
				}
				now := time.Now().Truncate(time.Millisecond)
				prtprotection.ReportClock(logger, app, clock, errors.New(cause))
				path := filepath.Join(t.TempDir(), "node.log")
				require.NoError(t, os.WriteFile(path, output.Bytes(), 0600))
				t.Setenv("CARTESI_TEST_NODE_LOG_FILE", path)
				expected := prtProtectionStopLog(app.Name, app.IApplicationAddress, 2, root, 123, cause, tc.source)
				unexpected, unmatched := scanNodeLogsBetween(t, now, time.Now(), []ExpectedLog{expected})
				require.Empty(t, unexpected, "the actual stop reporter must match the integration oracle")
				require.Empty(t, unmatched)
			})
		}
	}
}

func testPrtProtectionStopLogs(t *testing.T) {
	const application = "stopped-prt"
	const cause = "epoch 2: invalid root"
	address := common.HexToAddress("0x1001")
	root := common.HexToAddress("0x2002")
	now := time.Now().Truncate(time.Millisecond)
	line := now.Format(nodeLogTimeFmt) + fmt.Sprintf(
		` ERR PRT protection stopped service=prt application=%s address=%s epoch_index=2 root_tournament=%s cause="%s"`,
		application, address.Hex(), root.Hex(), cause)
	pinned := ` clock_source="pinned root descriptor at block 90 (` + common.Hash{}.Hex() + `)"`
	stored := ` clock_source="stored root descriptor"`
	available := ` clock_available=true join_close_block=123`
	missing := ` clock_available=false clock_error="root descriptor is not stored"`
	for _, tc := range []struct {
		name   string
		source string
		line   string
		valid  bool
	}{
		{name: "pinned exact end", line: line + pinned + available, valid: true},
		{name: "stored exact end", source: prtStoredClockSource, line: line + stored + available, valid: true},
		{name: "stored before projection", source: prtStoredClockSource, line: line + stored + missing, valid: true},
		{name: "wrong application", line: strings.Replace(line, "application="+application, "application="+application+"-other", 1) +
			pinned + available},
		{name: "wrong application case", line: strings.Replace(line, "application="+application,
			"application="+strings.ToUpper(application), 1) + pinned + available},
		{name: "wrong address", line: strings.Replace(line, "address="+address.Hex(), "address="+root.Hex(), 1) + pinned + available},
		{name: "wrong epoch", line: strings.Replace(line, "epoch_index=2", "epoch_index=1", 1) + pinned + available},
		{name: "wrong root", line: strings.Replace(line, "root_tournament="+root.Hex(), "root_tournament="+address.Hex(), 1) +
			pinned + available},
		{name: "wrong close", line: line + pinned + strings.Replace(available, "123", "124", 1)},
		{name: "stored wrong close", source: prtStoredClockSource, line: line + stored + strings.Replace(available, "123", "124", 1)},
		{name: "wrong source", line: line + stored + available},
		{name: "wrong source case", line: line +
			strings.Replace(pinned, prtPinnedClockSource, strings.ToUpper(prtPinnedClockSource), 1) + available},
		{name: "direct wrong source", source: prtStoredClockSource, line: line + pinned + available},
		{name: "observer missing clock", line: line + pinned + missing},
		{name: "wrong unavailable error", source: prtStoredClockSource, line: line + stored +
			strings.Replace(missing, "root descriptor is not stored", "connection refused", 1)},
		{name: "wrong cause", line: strings.Replace(line, cause, "epoch 2: another failure", 1) + pinned + available},
		{name: "unreported end", line: line + pinned + ` clock_available=true`},
		{name: "extra clock error", line: line + pinned + available + ` clock_error="connection refused"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.source
			if source == "" {
				source = prtPinnedClockSource
			}
			expected := prtProtectionStopLog(application, address, 2, root, 123, cause, source)
			require.True(t, expected.Required)
			path := filepath.Join(t.TempDir(), "node.log")
			require.NoError(t, os.WriteFile(path, []byte(tc.line+"\n"), 0600))
			t.Setenv("CARTESI_TEST_NODE_LOG_FILE", path)
			unexpected, unmatched := scanNodeLogsBetween(t, now, now, []ExpectedLog{expected})
			if tc.valid {
				require.Empty(t, unexpected)
				require.Empty(t, unmatched)
			} else {
				require.Equal(t, []string{tc.line}, unexpected)
				require.Equal(t, []int{0}, unmatched)
			}
		})
	}
	t.Run("both sources are required", func(t *testing.T) {
		expected := []ExpectedLog{
			prtProtectionStopLog(application, address, 2, root, 123, cause, prtStoredClockSource),
			prtProtectionStopLog(application, address, 2, root, 123, cause, prtPinnedClockSource),
		}
		path := filepath.Join(t.TempDir(), "node.log")
		t.Setenv("CARTESI_TEST_NODE_LOG_FILE", path)
		for _, source := range []struct {
			line    string
			missing int
		}{
			{line: line + stored + available, missing: 1},
			{line: line + pinned + available, missing: 0},
		} {
			require.NoError(t, os.WriteFile(path, []byte(source.line+"\n"), 0600))
			unexpected, unmatched := scanNodeLogsBetween(t, now, now, expected)
			require.Empty(t, unexpected)
			require.Equal(t, []int{source.missing}, unmatched)
		}
	})
}

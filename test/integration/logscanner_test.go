// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

// Package-level log scanner for detecting unexpected ERR entries in the
// node's log output during integration tests.
//
// Each test suite embeds a LogChecker and calls StartLogCapture() in
// SetupTest and CheckLogs() in TearDownTest. The scanner only examines
// log lines whose timestamps fall within that test's time window,
// so per-test allowlists remain precise even though the node is a
// single shared process.

package integration

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// nodeLogTimeFmt matches the tint logger's TimeFormat: RFC3339 with
// milliseconds and no timezone.
const nodeLogTimeFmt = "2006-01-02T15:04:05.000"

// ansiPattern strips ANSI escape sequences from log lines so that
// color-enabled output can be parsed reliably.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// AllowedError describes an error pattern that is expected in the node logs.
// Pattern is matched against the full (ANSI-stripped) log line.
// When Required is true, the test fails if the pattern is not found.
// When Required is false, a warning is logged if the pattern is not found.
type AllowedError struct {
	Pattern  *regexp.Regexp
	Reason   string // documentation: why this error is expected
	Required bool   // if true, absence of this error fails the test
}

// LogChecker is an embeddable helper for test suites that captures a time
// window and an allowlist, then scans node logs per test in TearDownTest.
type LogChecker struct {
	logStart      time.Time
	allowedErrors []AllowedError
}

// StartLogCapture records the current time as the beginning of this
// test's log window and resets allowed errors. Call this in SetupTest.
func (lc *LogChecker) StartLogCapture() {
	lc.logStart = time.Now()
	lc.allowedErrors = nil
}

// SetAllowedErrors configures the error patterns that are expected
// during this suite's execution.
func (lc *LogChecker) SetAllowedErrors(allowed ...AllowedError) {
	lc.allowedErrors = allowed
}

// CheckLogs scans the node log file for unexpected ERR lines that
// fall within this test's time window. Call this in TearDownTest.
// It also warns when an allowed error pattern was configured but
// never matched any log line, which may indicate a stale allowlist.
//
// Unexpected-error detection only looks at ERR-level lines, but
// allowed-pattern matching (both Required and non-Required) scans
// every line in the window regardless of log level. This lets tests
// correlate on events that are legitimately logged below ERR (for
// example, HTTP 4xx handlers that log at Info) without forcing the
// production code to log at a misleading level.
func (lc *LogChecker) CheckLogs(t testing.TB) {
	t.Helper()
	unexpected, unmatchedIdx := scanNodeLogsBetween(
		t, lc.logStart, time.Now(), lc.allowedErrors,
	)
	if len(unexpected) > 0 {
		t.Errorf("Found %d unexpected error(s) in node logs:", len(unexpected))
		for i, line := range unexpected {
			if i >= 20 {
				t.Errorf("  ... and %d more", len(unexpected)-20)
				break
			}
			t.Errorf("  %s", line)
		}
	}
	for _, idx := range unmatchedIdx {
		a := lc.allowedErrors[idx]
		if a.Required {
			t.Errorf("Required error pattern never matched: %s (reason: %s)",
				a.Pattern.String(), a.Reason)
		} else {
			t.Logf("WARNING: allowed error pattern never matched: %s (reason: %s)",
				a.Pattern.String(), a.Reason)
		}
	}
}

// scanNodeLogsBetween reads the node log file, collects ERR lines in
// the [from, to+grace] window that don't match any allowed pattern,
// and tracks which allowed patterns matched any line (any level) in
// that window. A 2-second grace buffer is applied at the end to catch
// errors from async operations triggered during the test. No grace is
// applied at the start — lines before StartLogCapture belong to the
// previous test.
//
// Unexpected collection is intentionally ERR-only: tests regress when
// the node starts logging real errors. But allowed-pattern matching
// spans every level so that tests can correlate on events logged at
// Info or Warn (for example, client-error HTTP paths that shouldn't
// be ERR in production). Non-ERR lines never count as unexpected.
func scanNodeLogsBetween(
	t testing.TB,
	from, to time.Time,
	allowed []AllowedError,
) (unexpected []string, unmatchedIdx []int) {
	t.Helper()

	logFile := os.Getenv("CARTESI_TEST_NODE_LOG_FILE")
	if logFile == "" {
		t.Log("CARTESI_TEST_NODE_LOG_FILE not set, skipping node log error scan")
		return nil, nil
	}

	f, err := os.Open(logFile)
	if err != nil {
		t.Logf("WARNING: could not open node log file %s: %v", logFile, err)
		return nil, nil
	}
	defer f.Close()

	const grace = 2 * time.Second
	windowStart := from
	windowEnd := to.Add(grace)

	matched := make([]bool, len(allowed))
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // handle long lines (stack traces, JSON)
	for scanner.Scan() {
		line := stripANSI(scanner.Text())

		ts, tsOK := parseLogTimestamp(line)
		if tsOK && (ts.Before(windowStart) || ts.After(windowEnd)) {
			continue
		}

		// Allowed-pattern matching spans every level.
		if idx := matchedAllowedIdx(line, allowed); idx >= 0 {
			matched[idx] = true
			continue
		}

		// Only ERR lines that didn't match become "unexpected".
		// Timestamp-unparseable lines are conservatively included.
		if !isErrLine(line) {
			continue
		}
		unexpected = append(unexpected, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading node log file: %v", err)
	}

	for i, m := range matched {
		if !m {
			unmatchedIdx = append(unmatchedIdx, i)
		}
	}
	return unexpected, unmatchedIdx
}

// isErrLine checks whether a (ANSI-stripped) log line contains the ERR
// level marker. The tint logger format places the level right after the
// timestamp: "2006-01-02T15:04:05.000 ERR ..."
func isErrLine(line string) bool {
	return strings.Contains(line, " ERR ")
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func parseLogTimestamp(line string) (time.Time, bool) {
	// The tint timestamp is always the first 23 characters:
	// "2006-01-02T15:04:05.000"
	if len(line) < len(nodeLogTimeFmt) {
		return time.Time{}, false
	}
	ts, err := time.ParseInLocation(nodeLogTimeFmt, line[:len(nodeLogTimeFmt)], time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

// matchedAllowedIdx returns the index of the first allowed pattern that
// matches line, or -1 if none match.
func matchedAllowedIdx(line string, allowed []AllowedError) int {
	for i, a := range allowed {
		if a.Pattern.MatchString(line) {
			return i
		}
	}
	return -1
}

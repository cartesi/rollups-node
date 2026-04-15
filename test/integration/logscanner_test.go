// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

// Package-level log scanner for correlating on expected log entries and
// detecting unexpected ERR entries in the node's log output during
// integration tests.
//
// Each test suite embeds a LogChecker and calls StartLogCapture() in
// SetupTest and CheckLogs() in TearDownTest. The scanner only examines
// log lines whose timestamps fall within that test's time window,
// so per-test expectations remain precise even though the node is a
// single shared process.

package integration

import (
	"bufio"
	"fmt"
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

// LogLevel is one of the tint logger's level tokens. Values match the
// exact substrings tint emits so scanner matching is a plain substring
// check. LogLevel("") is an invalid/unknown level and is rejected by
// SetExpectedLogs; every ExpectedLog must declare one of the typed
// constants below.
type LogLevel string

const (
	LevelError LogLevel = "ERR"
	LevelWarn  LogLevel = "WRN"
	LevelInfo  LogLevel = "INF"
	LevelDebug LogLevel = "DBG"
)

// ExpectedLog describes a log entry a test expects or tolerates.
//
// Pattern is matched against the full (ANSI-stripped) log line.
// Level narrows the match to lines of exactly that level — matching is
// strict, so a pattern declared at LevelInfo will not match an ERR line
// even if the text is identical. This prevents broad patterns at one
// level from accidentally silencing real regressions at another level,
// and keeps the staleness warning meaningful per-level.
//
// When Required is true, the test fails if no line in the window matches
// both the pattern and the level. When Required is false, a warning is
// logged on TearDown instead.
//
// Separately, when Level is LevelError, any matching ERR line is excluded
// from the "unexpected errors" list. Entries at other levels never affect
// the unexpected-error collection — non-ERR lines can never be unexpected.
type ExpectedLog struct {
	Pattern  *regexp.Regexp
	Level    LogLevel
	Reason   string // documentation: why this log entry is expected
	Required bool   // if true, absence of a matching line fails the test
}

// LogChecker is an embeddable helper for test suites that captures a time
// window and an expectation list, then scans node logs per test in
// TearDownTest.
type LogChecker struct {
	logStart     time.Time
	expectedLogs []ExpectedLog
}

// StartLogCapture records the current time as the beginning of this
// test's log window and resets the expectation list. Call this in SetupTest.
func (lc *LogChecker) StartLogCapture() {
	lc.logStart = time.Now()
	lc.expectedLogs = nil
}

// SetExpectedLogs configures the log entries this test expects or tolerates.
// Every entry must declare an explicit Level; an entry with the zero value
// fails the test loudly at setup time rather than being silently permissive.
func (lc *LogChecker) SetExpectedLogs(t testing.TB, expected ...ExpectedLog) {
	t.Helper()
	for i, e := range expected {
		if e.Level == "" {
			t.Fatalf("SetExpectedLogs: entry %d (%q) must declare a Level",
				i, e.Pattern.String())
		}
		if !isKnownLevel(e.Level) {
			t.Fatalf("SetExpectedLogs: entry %d (%q) has unknown Level %q",
				i, e.Pattern.String(), e.Level)
		}
	}
	lc.expectedLogs = expected
}

func isKnownLevel(l LogLevel) bool {
	switch l {
	case LevelError, LevelWarn, LevelInfo, LevelDebug:
		return true
	}
	return false
}

// CheckLogs scans the node log file for unexpected ERR lines that fall
// within this test's time window. Call this in TearDownTest. It also
// reports expected log entries that were configured but never matched
// a line at their declared level, which usually means either the
// allowlist is stale or the production code no longer emits what the
// test thought it did.
//
// Unexpected-error detection looks only at ERR-level lines. Expected-log
// matching is strict per level: an ExpectedLog at LevelInfo only counts
// as "matched" when an Info line matches its pattern. This preserves
// per-level staleness signals and prevents broad patterns from
// accidentally silencing real regressions.
func (lc *LogChecker) CheckLogs(t testing.TB) {
	t.Helper()
	unexpected, unmatchedIdx := scanNodeLogsBetween(
		t, lc.logStart, time.Now(), lc.expectedLogs,
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
		e := lc.expectedLogs[idx]
		msg := fmt.Sprintf("pattern %q at level %s never matched a log line (reason: %s)",
			e.Pattern.String(), e.Level, e.Reason)
		if e.Required {
			t.Errorf("Required expected log never matched: %s", msg)
		} else {
			t.Logf("WARNING: expected log never matched: %s", msg)
		}
	}
}

// scanNodeLogsBetween reads the node log file, collects ERR lines in the
// [from, to+grace] window that don't match any expected entry at
// LevelError, and tracks which expected entries matched a line at their
// declared level. A 2-second grace buffer is applied at the end to catch
// errors from async operations triggered during the test. No grace is
// applied at the start — lines before StartLogCapture belong to the
// previous test.
//
// Matching is strict per level: an ExpectedLog entry only counts as
// "matched" when a line's parsed level equals the entry's Level and the
// line text matches the Pattern. This separates the "did my code path
// run" question from the "did the node log a real problem" question.
//
// Lines without a recognisable tint level token are skipped. Such lines
// are almost always stack-trace continuations or plain-text stderr
// output from child processes (Cartesi Machine emulator, anvil) —
// promoting them to ERR would flood the unexpected list with benign
// chatter. This matches the old scanner's isErrLine filter.
func scanNodeLogsBetween(
	t testing.TB,
	from, to time.Time,
	expected []ExpectedLog,
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

	matched := make([]bool, len(expected))
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // handle long lines (stack traces, JSON)
	for scanner.Scan() {
		line := stripANSI(scanner.Text())

		ts, tsOK := parseLogTimestamp(line)
		if tsOK && (ts.Before(windowStart) || ts.After(windowEnd)) {
			continue
		}

		lineLevel, levelOK := parseLogLevel(line)
		if !levelOK {
			// No tint level token on this line. This is almost
			// always a continuation/multiline segment (stack trace
			// or a quoted error with embedded newlines) or plain
			// stderr output from a child process such as the
			// Cartesi Machine emulator C library. Skip it — we have
			// no basis to call it an error, and promoting it to
			// ERR would flag benign emulator chatter as a
			// regression. Matches the old scanner's !isErrLine
			// filter.
			continue
		}

		if idx := matchedExpectedIdx(line, lineLevel, expected); idx >= 0 {
			matched[idx] = true
			continue
		}

		if lineLevel == LevelError {
			unexpected = append(unexpected, line)
		}
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

// parseLogLevel extracts the tint level token from a log line. The tint
// logger emits the level immediately after the timestamp, flanked by
// spaces: "2006-01-02T15:04:05.000 ERR message...". Returns false when
// the line is too short or no known level token is present at the
// expected position.
func parseLogLevel(line string) (LogLevel, bool) {
	// Fast path: look for " LVL " at position len(timestamp).
	// The timestamp is always exactly len(nodeLogTimeFmt) characters.
	const levelTokenLen = 3
	start := len(nodeLogTimeFmt) + 1 // skip timestamp + single space
	end := start + levelTokenLen
	if len(line) < end+1 || line[start-1] != ' ' || line[end] != ' ' {
		// Fallback: substring scan. Handles lines with unexpected
		// leading whitespace or an extra separator.
		for _, lvl := range []LogLevel{LevelError, LevelWarn, LevelInfo, LevelDebug} {
			if strings.Contains(line, " "+string(lvl)+" ") {
				return lvl, true
			}
		}
		return "", false
	}
	tok := LogLevel(line[start:end])
	if !isKnownLevel(tok) {
		return "", false
	}
	return tok, true
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

// matchedExpectedIdx returns the index of the first expected entry whose
// Pattern matches line AND whose Level equals lineLevel. Returns -1 when
// no entry matches.
func matchedExpectedIdx(line string, lineLevel LogLevel, expected []ExpectedLog) int {
	for i, e := range expected {
		if e.Level != lineLevel {
			continue
		}
		if e.Pattern.MatchString(line) {
			return i
		}
	}
	return -1
}

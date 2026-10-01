// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Time estimates. They only set expectations in the progress log; nothing
// waits for them. A capture records how long its harness ran, and later
// estimates prefer that value.
var knownHarnessDurations = map[string]time.Duration{
	scenarioEchoSimple:     2 * time.Minute, //nolint:mnd // measured
	scenarioHoneypotStfAll: 9*time.Minute + 20*time.Second,
}

const (
	unknownHarnessDuration = 5 * time.Minute
	// replayDuration covers loading the dump, indexing, and verifying.
	replayDuration     = time.Minute
	harnessLogInterval = 30 * time.Second
	maxLogLineRunes    = 110
	logTailBytes       = 8 << 10
)

// continuationDurations are measured at the default mining rate (10 blocks
// per second); both wait out the case's claim staging period.
var continuationDurations = map[string]time.Duration{
	contRollupsAlone:    4 * time.Minute, //nolint:mnd // measured
	contOutputExecution: 5 * time.Minute, //nolint:mnd // measured
}

// harnessEstimate returns how long the harness of program/scenario takes,
// and where the number comes from. Earlier captures of the scenario in
// casesDir (kept, set aside, or --fresh) record their own duration.
func harnessEstimate(scenario, casesDir string) (time.Duration, string) {
	program, name, _ := strings.Cut(scenario, "/")
	base := filepath.Join(casesDir, program+"-"+name)
	dirs := []string{base}
	for _, pattern := range []string{base + failedCaseMarker + "*", base + "-2*"} {
		matches, _ := filepath.Glob(pattern)
		dirs = append(dirs, matches...)
	}
	for _, dir := range dirs {
		if m, err := readManifest(dir); err == nil && m.Provenance.HarnessSeconds > 0 {
			return time.Duration(m.Provenance.HarnessSeconds) * time.Second, "as the last capture"
		}
	}
	if known, ok := knownHarnessDurations[scenario]; ok {
		return known, "typical"
	}
	return unknownHarnessDuration, "a guess: no earlier capture"
}

// aboutText renders an estimate, for example "about 9 min" or "about 40 s".
func aboutText(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("about %d s", int(d.Seconds()))
	}
	return fmt.Sprintf("about %d min", int((d + time.Minute/2).Minutes()))
}

// elapsedText compares the time spent so far with an estimate.
func elapsedText(elapsed, estimate time.Duration) string {
	elapsed = elapsed.Round(time.Second)
	if elapsed > estimate {
		return fmt.Sprintf("%s so far, longer than the %s expected", elapsed, aboutText(estimate)[len("about "):])
	}
	return fmt.Sprintf("%s of %s", elapsed, aboutText(estimate))
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// lastLogLine returns the last non-empty line of a log, without terminal
// escapes and shortened for one progress line.
func lastLogLine(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() > logTailBytes {
		if _, err := file.Seek(-logTailBytes, io.SeekEnd); err != nil {
			return ""
		}
	}
	tail, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	lines := bytes.Split(tail, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(ansiEscape.ReplaceAllString(string(lines[i]), ""))
		line = strings.TrimSpace(strings.ReplaceAll(line, "\r", " "))
		if line == "" {
			continue
		}
		if runes := []rune(line); len(runes) > maxLogLineRunes {
			line = string(runes[:maxLogLineRunes-1]) + "…"
		}
		return line
	}
	return ""
}

// heartbeat logs a progress line every interval until the returned function
// is called.
func heartbeat(interval time.Duration, line func() string) (stop func()) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				logger.Info(line())
			}
		}
	}()
	return func() { close(done) }
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Output conventions: stdout carries the result (text by default, JSON with
// --json); stderr carries a short progress log, one line per stage step.
var (
	outputJSON bool
	verbose    bool
)

const maxListedItems = 5

func configureLogger() {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	info, err := os.Stderr.Stat()
	color := err == nil && info.Mode()&os.ModeCharDevice != 0
	logger = slog.New(&lineHandler{mu: &sync.Mutex{}, w: os.Stderr, level: level, color: color})
}

// lineHandler writes one short line per record: "15:04:05 message k=v".
// Only warnings, errors, and debug lines carry a level.
type lineHandler struct {
	mu    *sync.Mutex
	w     io.Writer
	level slog.Level
	color bool
	attrs []slog.Attr
}

const (
	ansiDim    = "\x1b[2m"
	ansiBold   = "\x1b[1m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiReset  = "\x1b[0m"
)

func (h *lineHandler) Enabled(_ context.Context, level slog.Level) bool { return level >= h.level }

func (h *lineHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(h.paint(ansiDim, r.Time.Format(time.TimeOnly)))
	b.WriteByte(' ')
	switch {
	case r.Level >= slog.LevelError:
		b.WriteString(h.paint(ansiRed, "ERROR") + " ")
	case r.Level >= slog.LevelWarn:
		b.WriteString(h.paint(ansiYellow, "WARN") + " ")
	case r.Level < slog.LevelInfo:
		b.WriteString(h.paint(ansiDim, "debug") + " ")
	}
	b.WriteString(r.Message)
	writeAttr := func(a slog.Attr) bool {
		value := a.Value.String()
		if strings.ContainsAny(value, " \t\"") {
			value = fmt.Sprintf("%q", value)
		}
		b.WriteString(" " + h.paint(ansiDim, a.Key+"=") + value)
		return true
	}
	for _, a := range h.attrs {
		writeAttr(a)
	}
	r.Attrs(writeAttr)
	b.WriteByte('\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *lineHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &clone
}

func (h *lineHandler) WithGroup(string) slog.Handler { return h }

func (h *lineHandler) paint(color, text string) string {
	if !h.color {
		return text
	}
	return color + text + ansiReset
}

// displayPath shortens a path below the working directory to a relative one.
func displayPath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return path
	}
	return rel
}

// step logs one progress line of a stage, for example "replay: node started".
func step(stage, format string, args ...any) {
	logger.Info(stage + ": " + fmt.Sprintf(format, args...))
}

// banner separates the scenarios of a suite in the progress log.
func banner(format string, args ...any) {
	title := fmt.Sprintf(format, args...)
	const width = 72
	logger.Info("━━ " + title + " " + strings.Repeat("━", max(3, width-len(title)))) //nolint:mnd
}

type textWriter interface {
	writeText(w io.Writer)
}

// emit writes a command result: JSON with --json, text otherwise.
func emit(w io.Writer, value any) error {
	if outputJSON {
		return writeJSON(w, value)
	}
	if text, ok := value.(textWriter); ok {
		text.writeText(w)
		return nil
	}
	return writeJSON(w, value)
}

// textStyle renders the text reports: a colored mark per status, section
// titles, and details wrapped under their column. Colors only on a terminal,
// and never with NO_COLOR.
type textStyle struct {
	color bool
	width int
}

const (
	reportWidth    = 110
	maxReportWidth = 140
	minDetailWidth = 40
	markColumn     = 5 // "  ✔  "
)

func newTextStyle(w io.Writer) textStyle {
	st := textStyle{width: reportWidth}
	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > minDetailWidth {
		st.width = min(columns, maxReportWidth)
	}
	if f, ok := w.(*os.File); ok && os.Getenv("NO_COLOR") == "" {
		info, err := f.Stat()
		st.color = err == nil && info.Mode()&os.ModeCharDevice != 0
	}
	return st
}

func (st textStyle) paint(code, text string) string {
	if !st.color || text == "" {
		return text
	}
	return code + text + ansiReset
}

// mark is the one-character status mark: ✔ pass, ✘ fail, ○ not tested;
// blank when a row has no check.
func (st textStyle) mark(status checkStatus) string {
	switch status {
	case checkPass:
		return st.paint(ansiGreen, "✔")
	case checkFail:
		return st.paint(ansiRed+ansiBold, "✘")
	case checkNotTested:
		return st.paint(ansiYellow, "○")
	}
	return " "
}

func (st textStyle) verdict(passed bool) string {
	if passed {
		return st.paint(ansiGreen+ansiBold, "PASS")
	}
	return st.paint(ansiRed+ansiBold, "FAIL")
}

// tally is the header's count of checks, which also explains the marks.
func (st textStyle) tally(counts map[checkStatus]int) string {
	parts := make([]string, 0, len(counts))
	for _, entry := range []struct {
		status checkStatus
		label  string
	}{{checkPass, "passed"}, {checkFail, "failed"}, {checkNotTested, "not tested"}} {
		if counts[entry.status] > 0 || entry.status == checkFail {
			parts = append(parts, fmt.Sprintf("%s %d %s", st.mark(entry.status), counts[entry.status], entry.label))
		}
	}
	return strings.Join(parts, "   ")
}

// heading is the first line of a report: a bold title and the verdict.
func (st textStyle) heading(w io.Writer, title, verdict string) {
	fmt.Fprintf(w, "%s  %s\n", st.paint(ansiBold, title), verdict)
}

// field is one "  label   value" line of a report header.
func (st textStyle) field(w io.Writer, label, value string) {
	fmt.Fprintf(w, "  %s %s\n", st.paint(ansiDim, pad(label, 9)), value) //nolint:mnd // label column
}

// section starts a section: a blank line, the bold title, and a rule.
func (st textStyle) section(w io.Writer, title, after string) {
	fmt.Fprintln(w)
	line := st.paint(ansiBold, title)
	if after != "" {
		line += "  " + after
	}
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, st.paint(ansiDim, strings.Repeat("─", min(st.width, textWidth(title)))))
}

// row writes "  ✔  name  detail" with the detail wrapped under its column;
// a name wider than the column puts the detail on the next lines.
func (st textStyle) row(w io.Writer, status checkStatus, name string, width int, detail string) {
	width = max(width, 0)
	column := markColumn + width + 2 //nolint:mnd // two spaces after the name
	head := "  " + st.mark(status) + "  "
	lines := wrapWords(detail, max(st.width-column, minDetailWidth))
	if textWidth(name) > width {
		column = markColumn + 2 //nolint:mnd // indent under the name
		fmt.Fprintln(w, head+st.nameText(status, name))
		lines = wrapWords(detail, max(st.width-column, minDetailWidth))
		for _, line := range lines {
			fmt.Fprintln(w, strings.Repeat(" ", column)+line)
		}
		return
	}
	first := ""
	if len(lines) > 0 {
		first = "  " + lines[0]
	}
	fmt.Fprintln(w, strings.TrimRight(head+st.nameText(status, pad(name, width))+first, " "))
	for _, line := range lines[min(1, len(lines)):] {
		fmt.Fprintln(w, strings.Repeat(" ", column)+line)
	}
}

func (st textStyle) nameText(status checkStatus, name string) string {
	if status == checkFail {
		return st.paint(ansiRed, name)
	}
	return name
}

// items lists the reasons of a failed check under its detail column.
func (st textStyle) items(w io.Writer, width int, items []string) {
	indent := strings.Repeat(" ", markColumn+width+2) //nolint:mnd // the detail column
	for i, item := range items {
		if i == maxListedItems {
			fmt.Fprintf(w, "%s%s\n", indent, st.paint(ansiDim, fmt.Sprintf("… %d more", len(items)-maxListedItems)))
			return
		}
		for j, line := range wrapWords(item, max(st.width-len(indent)-2, minDetailWidth)) { //nolint:mnd // "- "
			prefix := "  "
			if j == 0 {
				prefix = st.paint(ansiDim, "- ")
			}
			fmt.Fprintln(w, indent+prefix+line)
		}
	}
}

// note writes a dim paragraph under a section, indented like the rows.
func (st textStyle) note(w io.Writer, text string) {
	const indent = 2
	for _, line := range wrapWords(text, max(st.width-indent, minDetailWidth)) {
		fmt.Fprintln(w, strings.Repeat(" ", indent)+st.paint(ansiDim, line))
	}
}

// wrapWords breaks text into lines of at most width characters, at spaces;
// a longer word, such as a hash, keeps its own line.
func wrapWords(text string, width int) []string {
	var lines []string
	var line strings.Builder
	length := 0
	for _, word := range strings.Fields(text) {
		n := textWidth(word)
		if length > 0 && length+1+n > width {
			lines = append(lines, line.String())
			line.Reset()
			length = 0
		}
		if length > 0 {
			line.WriteByte(' ')
			length++
		}
		line.WriteString(word)
		length += n
	}
	if length > 0 {
		lines = append(lines, line.String())
	}
	return lines
}

// textWidth counts runes, which is the terminal width of the report's text.
func textWidth(text string) int { return utf8.RuneCountInString(text) }

func pad(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-textWidth(text)))
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value, suffix := float64(n), "B"
	for _, s := range []string{"kB", "MB", "GB", "TB"} {
		if value < unit {
			break
		}
		value /= unit
		suffix = s
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}

func humanSeconds(seconds int) string {
	return (time.Duration(seconds) * time.Second).String()
}

func writeItems(w io.Writer, indent string, items []string) {
	for i, item := range items {
		if i == maxListedItems {
			fmt.Fprintf(w, "%s… %d more\n", indent, len(items)-maxListedItems)
			return
		}
		fmt.Fprintf(w, "%s- %s\n", indent, item)
	}
}

// coverageText is the one-line form, for example
// "computation agreement tested, tournament indexing not tested".
func coverageText(coverage []coverageItem) string {
	parts := make([]string, 0, len(coverage))
	for _, item := range coverage {
		parts = append(parts, item.Name+" "+testedText(item.Tested))
	}
	return strings.Join(parts, ", ")
}

func testedText(tested bool) string {
	if tested {
		return "tested"
	}
	return "not tested"
}

func (r *preflightReport) writeText(w io.Writer) {
	fmt.Fprintf(w, "check %s: %s\n", r.Capability, map[bool]string{true: "OK", false: "MISSING PREREQUISITES"}[r.OK])
	for _, item := range r.Items {
		status := "ok"
		switch {
		case !item.OK && item.Required:
			status = "MISSING"
		case !item.OK:
			status = "warn"
		}
		fmt.Fprintf(w, "  %-8s %-28s %s\n", status, item.Name, item.Detail)
	}
}

func (m *caseManifest) writeText(w io.Writer) {
	golden := "no"
	if m.Golden {
		golden = "yes"
	}
	r := m.Reference
	fmt.Fprintf(w, "case %s (%s/%s)\n", m.Case, m.Provenance.Program, m.Provenance.Scenario)
	fmt.Fprintf(w, "  golden      %s (harness exit %d, %s", golden, m.Provenance.HarnessExitCode,
		valueOr(m.Provenance.ExitCodeSource, exitCodeObserved))
	if m.Provenance.HarnessSeconds > 0 {
		fmt.Fprintf(w, ", ran %s", humanSeconds(m.Provenance.HarnessSeconds))
	}
	fmt.Fprintln(w, ")")
	fmt.Fprintf(w, "  dave tree   %s", shortHash(m.Provenance.DaveRevision))
	if m.Provenance.DaveDirty {
		fmt.Fprint(w, " (modified)")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  chain       %d, head %d\n", m.Chain.ChainID, m.Chain.Head)
	fmt.Fprintf(w, "  application %s, deployed at block %d\n", m.Application.Address, m.Application.DeployBlock)
	fmt.Fprintf(w, "  reference   %d sealed, %d staged epochs; %d inputs; %d tournaments; %d stage claims\n",
		len(r.SealedEpochs), len(r.StagedEpochs), r.InputCount, r.Tournaments, len(r.Claims))
	fmt.Fprintf(w, "  sling node  %s: %d joins, %d sentry claims\n", r.Signer, r.SignerJoins, r.SignerSentries)
	fmt.Fprintf(w, "  dump        %s -> %s (%s)\n", humanBytes(m.Artifacts.Dump.RawSize), humanBytes(m.Artifacts.Dump.Size),
		m.Artifacts.Dump.Path)
	if len(m.Notes) > 0 {
		fmt.Fprintln(w, "  notes")
		writeItems(w, "    ", m.Notes)
	}
}

func shortHash(value string) string {
	if len(value) > 12 { //nolint:mnd
		return value[:12]
	}
	if value == "" {
		return textUnknown
	}
	return value
}

func (r *verifyReport) writeText(w io.Writer) {
	st := newTextStyle(w)
	st.heading(w, "verify "+r.Application, st.verdict(r.Passed)+st.paint(ansiDim, fmt.Sprintf("  at block %d", r.Head)))
	r.writeChecks(w, st)
}

// writeSection writes the verification as a section of a larger report.
func (r *verifyReport) writeSection(w io.Writer, st textStyle, title, subtitle string) {
	st.section(w, title, st.verdict(r.Passed)+st.paint(ansiDim, fmt.Sprintf("  %s, at block %d", subtitle, r.Head)))
	r.writeChecks(w, st)
}

// writeChecks writes the checks, the coverage, and the gaps.
func (r *verifyReport) writeChecks(w io.Writer, st textStyle) {
	width := 0
	for _, check := range r.Checks {
		width = max(width, textWidth(check.Name))
	}
	for _, item := range r.Coverage {
		width = max(width, textWidth(item.Name))
	}
	for _, check := range r.Checks {
		st.row(w, check.Status, check.Name, width, check.Detail)
		if check.Status == checkFail {
			st.items(w, width, check.Items)
		}
	}
	if len(r.Coverage) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  "+st.paint(ansiDim, "coverage"))
	}
	for _, item := range r.Coverage {
		status := checkNotTested
		if item.Tested {
			status = checkPass
		}
		st.row(w, status, item.Name, width, testedText(item.Tested)+": "+item.Detail)
	}
	if len(r.Gaps) > 0 {
		st.note(w, "Incomplete at the deadline:")
		st.items(w, width, r.Gaps)
	}
}

func (r *continuationReport) writeText(w io.Writer) {
	st := newTextStyle(w)
	st.section(w, "continuation "+r.Name, st.verdict(r.Passed))
	width := nameWidth(r.Steps)
	for _, s := range r.Steps {
		st.row(w, s.Status, s.Name, width, s.Detail)
	}
	if r.Error != "" {
		st.row(w, checkFail, "error", width, r.Error)
	}
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (r *replayReport) writeText(w io.Writer) {
	fmt.Fprintf(w, "replay %s: %s\n", r.Case, passFail(r.Passed))
	if p := r.Provenance; p != nil {
		golden := "golden"
		if !r.Golden {
			golden = "NOT golden"
		}
		fmt.Fprintf(w, "  case       %s/%s, %s, Dave tree %s, harness exit %d\n", p.Program, p.Scenario, golden,
			shortHash(p.DaveRevision), p.HarnessExitCode)
	}
	fmt.Fprintf(w, "  run dir    %s\n", displayPath(r.RunDir))
	fmt.Fprintf(w, "  database   %s\n", r.Database)
	if r.Error != "" {
		fmt.Fprintf(w, "  error      %s\n", r.Error)
	}
	st := newTextStyle(w)
	if r.Verify != nil {
		r.Verify.writeSection(w, st, "Verification", "the rollups node against the chain and the Sling node's evidence")
	}
	for _, c := range r.Continuations {
		c.writeText(w)
	}
	if r.FinalVerify != nil {
		r.FinalVerify.writeSection(w, st, "Verification after the continuations", "the same checks")
	}
}

func (r *runSlingResult) writeText(w io.Writer) {
	fmt.Fprintf(w, "run-sling: exit code %d\n", r.ExitCode)
	fmt.Fprintf(w, "  binary     %s (sha256 %s)\n", r.SlingBinary, shortHash(r.SHA256))
	fmt.Fprintf(w, "  signer     %s\n", r.Signer)
	fmt.Fprintf(w, "  state dir  %s\n", r.StateDir)
}

func (r *suiteReport) writeText(w io.Writer) {
	passed := 0
	for _, entry := range r.Scenarios {
		if entry.Passed {
			passed++
		}
	}
	fmt.Fprintf(w, "suite: %d of %d scenarios passed\n", passed, len(r.Scenarios))
	untested := false
	for _, entry := range r.Scenarios {
		for _, item := range entry.Coverage {
			untested = untested || (item.Name == coverageAgreement && !item.Tested)
		}
		detail := coverageText(entry.Coverage)
		if entry.Error != "" {
			detail = entry.Error
		}
		capture := "captured"
		if entry.Reused {
			capture = "reused"
		}
		fmt.Fprintf(w, "  %s  %-36s %8s  %-8s  %s\n", passFail(entry.Passed), entry.Scenario, humanSeconds(entry.Seconds), capture, detail)
		if !entry.Passed {
			writeItems(w, "        ", entry.Failed)
			if entry.RunDir != "" {
				fmt.Fprintf(w, "        run dir %s\n", displayPath(entry.RunDir))
			}
		}
	}
	if untested {
		fmt.Fprintln(w, "\ncomputation agreement is not tested when no epoch that matches the Sling node changes the")
		fmt.Fprintln(w, "machine state; honeypot rejects the plain harness inputs, so its epochs end where they start.")
	}
}

// scenarioList is the output of "suite --list".
type scenarioList struct {
	Smoke      []string `json:"smoke"`
	All        []string `json:"all"`
	OutOfScope []string `json:"out_of_scope"`
}

func (l *scenarioList) writeText(w io.Writer) {
	fmt.Fprintf(w, "smoke (default):\n")
	writeAll(w, l.Smoke)
	fmt.Fprintf(w, "all (every in-scope pair; echo/stf_all and honeypot/stf_revert are not in Dave's justfile):\n")
	writeAll(w, l.All)
	fmt.Fprintf(w, "out of scope (yield breaks the outputs-root rule):\n")
	writeAll(w, l.OutOfScope)
	fmt.Fprintln(w, "\nSelect with --scenarios smoke|all or a list such as echo/simple,honeypot/stf_all.")
}

func writeAll(w io.Writer, items []string) {
	for _, item := range items {
		fmt.Fprintf(w, "  %s\n", item)
	}
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	scenarioEchoSimple     = "echo/simple"
	scenarioHoneypotStfAll = "honeypot/stf_all"
)

// Scenario sets from the plan (section 14). yield is out of scope.
//
// Two pairs in all are not in Dave's justfile: echo/stf_all runs the
// state-transition disputes on accepted inputs, and honeypot/stf_revert
// replaces yield/stf_revert (honeypot also rejects every plain input, so the
// dispute lands on the revert restore).
var (
	smokeScenarios = []string{scenarioEchoSimple, scenarioHoneypotStfAll}
	allScenarios   = []string{
		scenarioEchoSimple, "echo/multi_sybil", "echo/sealed_leaf_timeout_winner", "echo/sealed_leaf_timeout_both",
		"echo/kill_catchup", "echo/kill_catchup_batched", "echo/kill_settle", "echo/kill_commitment_build",
		"echo/kill_mid_match", "echo/kill_join", "echo/chaos", "echo/stf_all",
		"honeypot/simple", "honeypot/simple_no_input", scenarioHoneypotStfAll, "honeypot/stf_revert", "honeypot/big_input",
		"honeypot/gc_match", "honeypot/gc_tournament", "honeypot/bad_commitment", "honeypot/deposit_withdrawal",
	}
	// The yield pairs of the pinned justfile; yield breaks the outputs-root rule.
	outOfScopeScenarios = []string{"yield/simple", "yield/simple_no_input", "yield/stf_all", "yield/stf_revert",
		"yield/big_input", "yield/gc_match", "yield/gc_tournament", "yield/bad_commitment"}
)

// defaultSuiteTestInstance keeps the harness off port 8545 (our devnet).
const defaultSuiteTestInstance = "18555"

type suiteOptions struct {
	replayOptions
	list           bool
	daveRoot       string
	scenarios      string
	casesDir       string
	fresh          bool
	testInstance   string
	then           string
	harnessTimeout time.Duration
}

type suiteEntry struct {
	Scenario    string         `json:"scenario"`
	CaseDir     string         `json:"case_dir"`
	Reused      bool           `json:"reused_capture"`
	Golden      bool           `json:"golden"`
	HarnessExit int            `json:"harness_exit_code"`
	RunDir      string         `json:"run_dir,omitempty"`
	Coverage    []coverageItem `json:"coverage,omitempty"`
	Failed      []string       `json:"failed,omitempty"`
	Passed      bool           `json:"passed"`
	Error       string         `json:"error,omitempty"`
	Seconds     int            `json:"seconds"`
}

type suiteReport struct {
	Scenarios []*suiteEntry `json:"scenarios"`
	Passed    bool          `json:"passed"`
}

func newSuiteCommand() *cobra.Command {
	opts := &suiteOptions{}
	cmd := &cobra.Command{
		Use:   "suite --dave-root DIR [--scenarios smoke|all|program/scenario,...]",
		Short: "Capture, replay, and verify a list of scenarios without interaction",
		Long: `suite runs, for each program/scenario in order:

  1. capture into <cases-dir>/<program>-<scenario>, or reuse that case when it
     is golden (--fresh captures again into a new, time-stamped directory). A
     failed earlier capture is kept as <dir>.failed-<time> and captured again;
  2. replay into a fresh rollups node and verify it (computation agreement is
     required for echo, whose inputs change state; honeypot rejects the
     harness inputs);
  3. optionally, the rollups-only continuations (--then); output-execution
     runs on honeypot cases only.

It never stops at the first failure. It prints one summary and exits 1 when
any scenario failed. Each replay writes its own report.json.

Sets: smoke = echo/simple,honeypot/stf_all; all = every in-scope pair (21
scenarios, about 70 minutes when captured).

The harness runs its Anvil on --test-instance (18555), so the devnet on 8545
can keep running.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.list {
				return emit(cmd.OutOrStdout(), &scenarioList{Smoke: smokeScenarios, All: allScenarios,
					OutOfScope: outOfScopeScenarios})
			}
			return runSuite(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.list, "list", false, "list the scenario sets and pairs, then exit")
	flags.StringVar(&opts.daveRoot, "dave-root", os.Getenv("DAVE_ROOT"), "prepared Dave tree (default: $DAVE_ROOT)")
	flags.StringVar(&opts.scenarios, "scenarios", "smoke", "smoke, all, or a comma-separated list of program/scenario")
	flags.StringVar(&opts.casesDir, "cases-dir", filepath.Join("_interop", "cases"), "directory of the case directories")
	flags.BoolVar(&opts.fresh, "fresh", false, "capture again even when a case exists")
	flags.StringVar(&opts.testInstance, "test-instance", defaultSuiteTestInstance,
		"harness TEST_INSTANCE: the harness Anvil port, so that it does not collide with the devnet on 8545")
	flags.DurationVar(&opts.harnessTimeout, "harness-timeout", defaultHarnessTimeout, "deadline of each harness run")
	flags.StringVar(&opts.then, "then", "", "rollups-only continuations after each replay: rollups-continues-alone, output-execution")
	flags.IntVar(&opts.anvilPort, "anvil-port", defaultAnvilPort, "port of the replay Anvil")
	flags.IntVar(&opts.nodePortBase, "node-port-base", defaultNodePortBase, "node telemetry port; API +11, inspect +12")
	flags.StringVar(&opts.dbAdmin, "db-admin", defaultDatabaseURL(), "Postgres URL used to create run databases")
	flags.BoolVar(&opts.dropDB, "drop-db", true, "drop the database of each passing replay")
	flags.DurationVar(&opts.wait, "wait", 10*time.Minute, "deadline for the completion predicates of each replay") //nolint:mnd
	flags.StringVar(&opts.nodeBin, "node-bin", "./cartesi-rollups-node", "rollups node binary")
	flags.StringVar(&opts.cliBin, "cli-bin", "./cartesi-rollups-cli", "rollups CLI binary")
	flags.Uint64Var(&opts.mineStep, "mine-step", 10, "blocks mined per step while cases run") //nolint:mnd
	flags.DurationVar(&opts.mineInterval, "mine-interval", time.Second, "time between mining steps")
	flags.DurationVar(&opts.continuationTimeout, "continuation-timeout", 20*time.Minute, //nolint:mnd
		"deadline of each rollups-only continuation")
	flags.StringVar(&opts.depositWei, "deposit-amount", "1000", "token amount for output-execution")
	addAnvilFlags(flags, &opts.replayOptions)
	return cmd
}

func expandScenarios(value string) ([]string, error) {
	var list []string
	switch value {
	case "smoke":
		list = smokeScenarios
	case "all":
		list = allScenarios
	default:
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				list = append(list, item)
			}
		}
	}
	for _, item := range list {
		program, scenario, ok := strings.Cut(item, "/")
		if !ok || program == "" || scenario == "" {
			return nil, fmt.Errorf("invalid scenario %q (use program/scenario)", item)
		}
		if program == programYield {
			return nil, errors.New("the yield program breaks the outputs-root rule; it is out of scope")
		}
	}
	if len(list) == 0 {
		return nil, errors.New("no scenarios selected")
	}
	return list, nil
}

func runSuite(ctx context.Context, out io.Writer, opts *suiteOptions) error {
	scenarios, err := expandScenarios(opts.scenarios)
	if err != nil {
		return withExitCode(exitUsage, err)
	}
	if opts.daveRoot == "" {
		return withExitCode(exitPrerequisites, errors.New("set --dave-root or DAVE_ROOT"))
	}
	if opts.dbAdmin == "" {
		return withExitCode(exitPrerequisites, errors.New("no Postgres URL: set CARTESI_DATABASE_CONNECTION or --db-admin"))
	}
	var continuations []string
	for _, name := range strings.Split(opts.then, ",") {
		if name = strings.TrimSpace(name); name != "" {
			continuations = append(continuations, name)
		}
	}
	ctx, stopNotify := signal.NotifyContext(ctx, stopSignals...)
	defer stopNotify()

	plans := planSuite(opts, scenarios, continuations)
	var total time.Duration
	for _, plan := range plans {
		total += plan.total
	}
	started := time.Now()
	step("suite", "%d scenarios, %s in total (finishing near %s):", len(plans), aboutText(total),
		started.Add(total).Format("15:04"))
	for _, plan := range plans {
		step("suite", "  %-34s %s", plan.scenario, plan.text)
	}

	report := &suiteReport{Passed: true}
	for i, plan := range plans {
		item := plan.scenario
		if ctx.Err() != nil {
			break
		}
		banner("[%d/%d] %s, %s", i+1, len(plans), item, aboutText(plan.total))
		entry := runSuiteScenario(ctx, opts, item, continuations, plan.harness)
		report.Scenarios = append(report.Scenarios, entry)
		report.Passed = report.Passed && entry.Passed
		result := fmt.Sprintf("%s %s after %s", item, passFail(entry.Passed), humanSeconds(entry.Seconds))
		if entry.Error != "" {
			result += ": " + entry.Error
		}
		step("result", "%s", result)
	}
	if ctx.Err() != nil {
		report.Passed = false
	}
	banner("summary after %s", time.Since(started).Round(time.Second))
	if err := emit(out, report); err != nil {
		return err
	}
	if !report.Passed {
		return withExitCode(exitFailure, errors.New("at least one scenario failed; see the summary"))
	}
	return nil
}

// scenarioPlan is what the suite expects to do for one scenario, and how long
// that takes.
type scenarioPlan struct {
	scenario string
	harness  time.Duration // zero when the case is reused
	total    time.Duration
	text     string
}

func planSuite(opts *suiteOptions, scenarios, continuations []string) []scenarioPlan {
	plans := make([]scenarioPlan, 0, len(scenarios))
	for _, item := range scenarios {
		program, scenario, _ := strings.Cut(item, "/")
		plan := scenarioPlan{scenario: item, total: replayDuration}
		m, err := readManifest(filepath.Join(opts.casesDir, program+"-"+scenario))
		if err == nil && m.Golden && !opts.fresh {
			plan.text = "reuse the case; replay " + aboutText(replayDuration)
		} else {
			estimate, source := harnessEstimate(item, opts.casesDir)
			plan.harness, plan.total = estimate, plan.total+estimate
			plan.text = fmt.Sprintf("capture %s (%s); replay %s", aboutText(estimate), source, aboutText(replayDuration))
		}
		for _, name := range continuations {
			if name == contOutputExecution && program != programHoneypot {
				continue
			}
			plan.total += continuationDurations[name]
			plan.text += fmt.Sprintf("; %s %s", name, aboutText(continuationDurations[name]))
		}
		plans = append(plans, plan)
	}
	return plans
}

func runSuiteScenario(ctx context.Context, opts *suiteOptions, item string, continuations []string,
	harness time.Duration,
) *suiteEntry {
	started := time.Now()
	program, scenario, _ := strings.Cut(item, "/")
	entry := &suiteEntry{Scenario: item, CaseDir: filepath.Join(opts.casesDir, program+"-"+scenario)}
	defer func() { entry.Seconds = int(time.Since(started).Seconds()) }()
	fail := func(err error) *suiteEntry {
		entry.Error = err.Error()
		return entry
	}

	if opts.fresh {
		entry.CaseDir += "-" + time.Now().UTC().Format("20060102T150405Z")
	}
	m, aside, err := prepareCaseDir(entry.CaseDir, time.Now())
	switch {
	case err != nil:
		return fail(fmt.Errorf("capture: %w", err))
	case m != nil:
		entry.Reused = true
		step("capture", "reusing the case in %s (captured %s; --fresh captures again)", entry.CaseDir,
			m.CreatedAt.Local().Format("2006-01-02 15:04"))
		if rev, _, err := gitRevision(opts.daveRoot); err == nil && m.Provenance.DaveRevision != "" && rev != m.Provenance.DaveRevision {
			logger.Warn(fmt.Sprintf("the case was captured with Dave %s, but the Dave tree is at %s; --fresh captures again",
				shortHash(m.Provenance.DaveRevision), shortHash(rev)))
		}
	default:
		if aside != "" {
			step("capture", "an earlier capture did not produce a golden case; kept it as %s", aside)
		}
		m, err = executeCapture(ctx, &captureOptions{daveRoot: opts.daveRoot, program: program, scenario: scenario,
			out: entry.CaseDir, testInstance: opts.testInstance, harnessTimeout: opts.harnessTimeout, estimate: harness}, io.Discard)
		if err != nil {
			return fail(fmt.Errorf("capture: %w", err))
		}
	}
	entry.Golden, entry.HarnessExit = m.Golden, m.Provenance.HarnessExitCode
	if m.Provenance.HarnessExitCode != 0 {
		return fail(fmt.Errorf("the harness exited with code %d; see %s", m.Provenance.HarnessExitCode,
			filepath.Join(entry.CaseDir, "harness.log")))
	}
	if !m.Golden {
		return fail(fmt.Errorf("the case is not golden: %s", strings.Join(m.Notes, "; ")))
	}

	replay := opts.replayOptions
	replay.caseDir = entry.CaseDir
	replay.runDir = ""
	replay.dbName = ""
	replay.keep = false
	replay.requireAgreement = program == programEcho
	replay.continuations = nil
	for _, name := range continuations {
		if name == contOutputExecution && program != programHoneypot {
			continue
		}
		replay.continuations = append(replay.continuations, name)
	}
	replay.submission = len(replay.continuations) > 0
	if len(replay.continuations) > 0 {
		step("replay", "rollups-only continuations after verification: %s", strings.Join(replay.continuations, ", "))
	}
	result, err := executeReplay(ctx, &replay, io.Discard)
	if result != nil {
		entry.RunDir = result.RunDir
		entry.Passed = result.Passed
		if result.Verify != nil {
			entry.Coverage = result.Verify.Coverage
			entry.Failed = append(entry.Failed, result.Verify.Gaps...)
			for _, check := range result.Verify.Checks {
				if check.Status == checkFail {
					entry.Failed = append(entry.Failed, check.Name+": "+check.Detail)
				}
			}
		}
		for _, c := range result.Continuations {
			if !c.Passed {
				entry.Failed = append(entry.Failed, "continuation "+c.Name+": "+c.Error)
			}
		}
		if result.FinalVerify != nil {
			for _, gap := range result.FinalVerify.Gaps {
				entry.Failed = append(entry.Failed, "after the continuations: "+gap)
			}
			for _, check := range result.FinalVerify.Checks {
				if check.Status == checkFail {
					entry.Failed = append(entry.Failed, "after the continuations: "+check.Name+": "+check.Detail)
				}
			}
		}
	}
	if err != nil {
		entry.Passed = false
		return fail(fmt.Errorf("replay: %w", err))
	}
	return entry
}

// failedCaseMarker names a case directory set aside by the suite.
const failedCaseMarker = ".failed-"

// prepareCaseDir decides what to do with a case directory. It returns the
// manifest of a golden case to reuse. Otherwise the directory is free for a
// new capture: a directory left by a failed capture (no valid manifest, or a
// case that is not golden) is renamed to <dir>.failed-<time>, never deleted,
// and that name is returned.
func prepareCaseDir(dir string, now time.Time) (*caseManifest, string, error) {
	m, err := readManifest(dir)
	if err == nil && m.Golden {
		return m, "", nil
	}
	if isMissingOrEmpty(dir) {
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
		return nil, "", nil
	}
	aside := dir + failedCaseMarker + now.UTC().Format("20060102T150405Z")
	if err := os.Rename(dir, aside); err != nil {
		return nil, "", fmt.Errorf("setting aside the failed capture %s: %w", dir, err)
	}
	return nil, aside, nil
}

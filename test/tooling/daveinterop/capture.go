// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

const (
	harnessDir      = "test/e2e/rollups"
	deploymentsDir  = "cartesi-rollups/contracts/deployments/31337"
	programsDir     = "test/programs"
	slingBinaryPath = "target/debug/cartesi-rollups-prt-node"
	caseDumpRaw     = "anvil-state.json"
	caseDumpZstd    = "anvil-state.json.zst"
	caseTemplateDir = "template"
)

type captureOptions struct {
	daveRoot     string
	program      string
	scenario     string
	out          string
	caseName     string
	testInstance string
	keepRaw      bool
	app          string
	fromDump     string
	slingLog     string
	harnessLog   string
	// harnessTimeout bounds one harness run.
	harnessTimeout time.Duration
	// estimate is the expected harness duration, for the progress log; zero
	// means "look it up".
	estimate time.Duration
}

// defaultHarnessTimeout is about three times the longest scenario we know
// (honeypot/stf_all takes about 9 minutes).
const defaultHarnessTimeout = 30 * time.Minute

func newCaptureCommand() *cobra.Command {
	opts := &captureOptions{}
	cmd := &cobra.Command{
		Use:   "capture --dave-root DIR --program P --scenario S --out DIR",
		Short: "Run a Dave harness scenario and record its chain as a case",
		Long: `capture runs "just rollups-tests::test <program> <scenario>" in a prepared Dave
tree with ANVIL_DUMP_PATH set, keeps the harness exit status, copies the
harness's Sling node log and the program template, loads the dump into a temporary Anvil to read the
chain facts, compresses the dump with zstd, and writes manifest.json.

A run whose harness failed is kept for diagnosis but is not golden.

With --from-dump, capture skips the harness and imports an existing dump
(for example, one recorded by hand). The provenance then records exit code -1
unless the dump comes with a harness log that you vouch for with --harness-log.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stopNotify := signal.NotifyContext(cmd.Context(), stopSignals...)
			defer stopNotify()
			m, err := executeCapture(ctx, opts, cmd.OutOrStdout())
			if err == nil && m.Provenance.HarnessExitCode != 0 {
				err = withExitCode(exitFailure, fmt.Errorf("the harness exited with code %d; the case is kept for diagnosis only",
					m.Provenance.HarnessExitCode))
			}
			return err
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.daveRoot, "dave-root", os.Getenv("DAVE_ROOT"), "prepared Dave tree (default: $DAVE_ROOT)")
	flags.StringVar(&opts.program, "program", "", "harness program: echo or honeypot (yield breaks the outputs-root rule)")
	flags.StringVar(&opts.scenario, "scenario", "", "harness scenario, for example stf_all")
	flags.StringVar(&opts.out, "out", "", "new case directory")
	flags.StringVar(&opts.caseName, "case", "", "case name (default: <program>-<scenario>)")
	flags.StringVar(&opts.testInstance, "test-instance", "", "harness TEST_INSTANCE (Anvil port; default 8545)")
	flags.BoolVar(&opts.keepRaw, "keep-raw", false, "keep the uncompressed dump next to the .zst")
	flags.StringVar(&opts.app, "app", "", "application address when the chain has several Dave apps")
	flags.StringVar(&opts.fromDump, "from-dump", "", "import this dump instead of running the harness")
	flags.StringVar(&opts.slingLog, "sling-log", "", "Sling node log to import with --from-dump")
	flags.StringVar(&opts.harnessLog, "harness-log", "", "harness log to import with --from-dump")
	flags.DurationVar(&opts.harnessTimeout, "harness-timeout", defaultHarnessTimeout, "deadline of the harness run")
	_ = cmd.MarkFlagRequired("program")
	_ = cmd.MarkFlagRequired("scenario")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

// executeCapture runs (or imports) one harness scenario and writes a case.
// The manifest is also written to out. A harness failure is recorded in the
// manifest (not golden), not returned as an error.
func executeCapture(ctx context.Context, opts *captureOptions, out io.Writer) (*caseManifest, error) {
	m, err := captureCase(ctx, opts, out)
	if err != nil {
		var exitErr *exitError
		if !errors.As(err, &exitErr) {
			err = withExitCode(exitFailure, err)
		}
	}
	return m, err
}

func captureCase(ctx context.Context, opts *captureOptions, stdout io.Writer) (*caseManifest, error) {
	if opts.program == programYield {
		return nil, withExitCode(exitUsage, errors.New("the yield program breaks the outputs-root rule; it is out of scope"))
	}
	if opts.daveRoot == "" {
		return nil, withExitCode(exitPrerequisites, errors.New("set --dave-root or DAVE_ROOT"))
	}
	daveRoot, err := filepath.Abs(opts.daveRoot)
	if err != nil {
		return nil, err
	}
	out, err := filepath.Abs(opts.out)
	if err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 {
		return nil, withExitCode(exitUsage, fmt.Errorf("%s is not empty; a case is never overwritten", out))
	}
	if err := os.MkdirAll(out, 0o755); err != nil { //nolint:mnd
		return nil, err
	}
	name := opts.caseName
	if name == "" {
		name = opts.program + "-" + opts.scenario
	}
	m := &caseManifest{Schema: manifestSchemaVersion, Case: name, CreatedAt: time.Now().UTC()}
	m.Provenance.Program, m.Provenance.Scenario, m.Provenance.TestInstance = opts.program, opts.scenario, opts.testInstance
	m.Provenance.AnvilVersion = toolVersion("anvil")
	if rev, dirty, err := gitRevision("."); err == nil {
		m.Provenance.RollupsRevision, m.Provenance.RollupsDirty = rev, dirty
	}
	// An imported dump may come from any Dave tree: its provenance is unknown.
	if opts.fromDump != "" {
		m.Notes = append(m.Notes, "imported dump: the Dave revision and Sling binary that produced it are unknown")
	} else if rev, dirty, err := gitRevision(daveRoot); err == nil {
		m.Provenance.DaveRevision, m.Provenance.DaveDirty = rev, dirty
	} else {
		m.Notes = append(m.Notes, "Dave revision unknown: "+err.Error())
	}
	if sum, _, err := sha256File(filepath.Join(daveRoot, slingBinaryPath)); err == nil && opts.fromDump == "" {
		m.Provenance.SlingBinarySHA256 = sum
	}

	rawDump := filepath.Join(out, caseDumpRaw)
	harnessLog := filepath.Join(out, "harness.log")
	daveLog := filepath.Join(out, "dave.log")
	if opts.fromDump == "" {
		m.Provenance.HarnessCommand = fmt.Sprintf("just rollups-tests::test %s %s", opts.program, opts.scenario)
		started := time.Now()
		code, err := runHarness(ctx, daveRoot, opts, rawDump, harnessLog)
		if err != nil {
			// Keep the directory only when the harness left a log in it.
			if isMissingOrEmpty(out) {
				_ = os.Remove(out)
			}
			return m, err
		}
		m.Provenance.HarnessExitCode, m.Provenance.ExitCodeSource = code, exitCodeObserved
		m.Provenance.HarnessSeconds = int(time.Since(started).Seconds())
		logSuffix := ""
		if opts.testInstance != "" {
			logSuffix = "-" + opts.testInstance
		}
		// The harness writes to a fixed log path; a log older than this run
		// belongs to an earlier run.
		slingLog := filepath.Join(daveRoot, harnessDir, "dave"+logSuffix+".log")
		if info, err := os.Stat(slingLog); err != nil {
			m.Notes = append(m.Notes, "Sling log not captured: "+err.Error())
		} else if info.ModTime().Before(started) {
			m.Notes = append(m.Notes, "Sling log not captured: "+slingLog+" was not written by this run")
		} else if err := copyFile(slingLog, daveLog, 0o644); err != nil { //nolint:mnd
			m.Notes = append(m.Notes, "Sling log not captured: "+err.Error())
		}
	} else {
		m.Provenance.HarnessCommand = "imported: " + opts.fromDump
		m.Provenance.HarnessExitCode, m.Provenance.ExitCodeSource = -1, exitCodeNone
		if opts.harnessLog != "" {
			m.Provenance.HarnessExitCode, m.Provenance.ExitCodeSource = 0, exitCodeOperator
			if err := copyFile(opts.harnessLog, harnessLog, 0o644); err != nil { //nolint:mnd
				return m, err
			}
			m.Notes = append(m.Notes, "harness exit status taken from the operator (--harness-log), not observed")
		}
		if err := copyFile(opts.fromDump, rawDump, 0o644); err != nil { //nolint:mnd
			return m, err
		}
		if opts.slingLog != "" {
			if err := copyFile(opts.slingLog, daveLog, 0o644); err != nil { //nolint:mnd
				return m, err
			}
		}
	}
	if _, err := os.Stat(harnessLog); err == nil {
		m.Artifacts.HarnessLog = "harness.log"
	}
	if _, err := os.Stat(daveLog); err == nil {
		m.Artifacts.DaveLog = "dave.log"
		claims, err := parseSlingClaimsFile(daveLog)
		if err != nil {
			return m, err
		}
		m.Reference.ClaimSource = "Sling stage log (the Sling node's own claim, not an oracle)"
		m.Reference.Claims = claims
	}
	if _, err := os.Stat(rawDump); err != nil {
		writeCaptureFailure(stdout, out, m, "no chain dump was written")
		return m, withExitCode(exitFailure, fmt.Errorf("the harness wrote no dump (exit code %d); see %s",
			m.Provenance.HarnessExitCode, harnessLog))
	}

	if err := readDeployments(daveRoot, m); err != nil {
		return m, withExitCode(exitPrerequisites, err)
	}
	if err := copyTree(filepath.Join(daveRoot, programsDir, opts.program, "machine-image"),
		filepath.Join(out, caseTemplateDir)); err != nil {
		return m, withExitCode(exitPrerequisites, fmt.Errorf("copying the %s template: %w", opts.program, err))
	}
	m.Application.TemplateDir = caseTemplateDir

	inconsistencies, err := readChainFacts(ctx, rawDump, opts.app, m)
	if err != nil {
		return m, err
	}
	if hash, err := storedMachineHash(filepath.Join(out, caseTemplateDir)); err != nil {
		m.Notes = append(m.Notes, "template hash not checked: "+err.Error())
	} else if common.HexToHash(hash) != m.Application.TemplateHash {
		return m, fmt.Errorf("copied template hash %s differs from the application template hash %s", hash, m.Application.TemplateHash)
	}

	rawSum, rawSize, err := sha256File(rawDump)
	if err != nil {
		return m, err
	}
	zstdDump := filepath.Join(out, caseDumpZstd)
	if err := compressZstd(rawDump, zstdDump); err != nil {
		return m, err
	}
	sum, size, err := sha256File(zstdDump)
	if err != nil {
		return m, err
	}
	m.Artifacts.Dump = caseFile{Path: caseDumpZstd, SHA256: sum, Size: size, RawSHA256: rawSum, RawSize: rawSize}
	step("capture", "dump compressed: %s -> %s", humanBytes(rawSize), humanBytes(size))
	if !opts.keepRaw {
		if err := os.Remove(rawDump); err != nil {
			return m, err
		}
	}

	m.Notes = append(m.Notes, inconsistencies...)
	m.Golden = m.Provenance.ExitCodeSource == exitCodeObserved && m.Provenance.HarnessExitCode == 0 &&
		m.Artifacts.DaveLog != "" && len(inconsistencies) == 0
	if !m.Golden {
		m.Notes = append(m.Notes, "not golden: a golden case needs an observed harness exit code 0, the Sling log, "+
			"and consistent Sling evidence")
	}
	if err := writeManifest(out, m); err != nil {
		return m, err
	}
	step("capture", "case written to %s", displayPath(out))
	if err := emit(stdout, m); err != nil {
		return m, err
	}
	return m, nil
}

// runHarness runs the stock scenario in its own process group. Its output goes
// to the harness log (and to stderr with --verbose); its exit code is returned
// unchanged. On timeout or interruption the whole group is stopped, and so is
// anything the harness leaves running when it exits.
func runHarness(ctx context.Context, daveRoot string, opts *captureOptions, dumpPath, logPath string) (int, error) {
	if _, err := exec.LookPath("just"); err != nil {
		return -1, withExitCode(exitPrerequisites, fmt.Errorf("just not found on PATH: %w", err))
	}
	if _, err := os.Stat(filepath.Join(daveRoot, programsDir, opts.program, "machine-image")); err != nil {
		return -1, withExitCode(exitPrerequisites,
			fmt.Errorf("the %s image is not built in %s (build it in the Dave tree first): %w", opts.program, daveRoot, err))
	}
	if opts.testInstance == "" {
		if err := requirePortFree(8545); err != nil { //nolint:mnd // harness default port
			return -1, withExitCode(exitPrerequisites,
				fmt.Errorf("the harness needs port 8545 (stop the devnet or use --test-instance): %w", err))
		}
	} else if port, err := strconv.Atoi(opts.testInstance); err != nil || requirePortFree(port) != nil {
		return -1, withExitCode(exitPrerequisites, fmt.Errorf("--test-instance %s must be a free port", opts.testInstance))
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return -1, err
	}
	defer logFile.Close()
	timeout := opts.harnessTimeout
	if timeout <= 0 {
		timeout = defaultHarnessTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"rollups-tests::test", opts.program, opts.scenario}
	cmd := exec.CommandContext(runCtx, "just", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = childStopTimeout
	cmd.Dir = daveRoot
	cmd.Env = append(os.Environ(), "ANVIL_DUMP_PATH="+dumpPath)
	if opts.testInstance != "" {
		cmd.Env = append(cmd.Env, "TEST_INSTANCE="+opts.testInstance)
	}
	var writer io.Writer = logFile
	if verbose {
		writer = io.MultiWriter(logFile, os.Stderr)
	}
	cmd.Stdout, cmd.Stderr = writer, writer
	step("capture", "running the Dave harness: just %s (log %s)", strings.Join(args, " "), displayPath(logPath))
	estimate, source := opts.estimate, "the suite estimate"
	if estimate == 0 {
		estimate, source = harnessEstimate(opts.program+"/"+opts.scenario, filepath.Dir(filepath.Dir(dumpPath)))
	}
	started := time.Now()
	step("capture", "the harness plays a full Dave dispute on its own Anvil: %s (%s); it logs nothing here, "+
		"so a progress line follows every %s", aboutText(estimate), source, harnessLogInterval)
	stopHeartbeat := heartbeat(harnessLogInterval, func() string {
		line := fmt.Sprintf("capture: harness running, %s", elapsedText(time.Since(started), estimate))
		if last := lastLogLine(logPath); last != "" {
			line += "; last line: " + last
		}
		return line
	})
	err = cmd.Run()
	stopHeartbeat()
	if cmd.Process != nil {
		stopGroup("harness", cmd.Process.Pid)
	}
	switch {
	case ctx.Err() != nil:
		return -1, fmt.Errorf("the harness was interrupted after %s", time.Since(started).Round(time.Second))
	case runCtx.Err() != nil:
		return -1, fmt.Errorf("the harness did not finish within %s (--harness-timeout); see %s", timeout, logPath)
	}
	if cmd.ProcessState != nil {
		step("capture", "the harness exited with code %d after %s", cmd.ProcessState.ExitCode(), time.Since(started).Round(time.Second))
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return -1, fmt.Errorf("running the harness: %w", err)
	}
	return cmd.ProcessState.ExitCode(), nil
}

func readDeployments(daveRoot string, m *caseManifest) error {
	read := func(contract string) (common.Address, error) { return readDeployment(daveRoot, contract) }
	var err error
	if m.Deployments.InputBox, err = read("InputBox"); err != nil {
		return err
	}
	if m.Deployments.DaveAppFactory, err = read("DaveAppFactory"); err != nil {
		return err
	}
	if m.Deployments.Erc20Portal, err = read("Erc20Portal"); err != nil {
		return err
	}
	m.Deployments.TestFungibleToken, err = read("TestFungibleToken")
	return err
}

func readDeployment(daveRoot, name string) (common.Address, error) {
	data, err := os.ReadFile(filepath.Join(daveRoot, deploymentsDir, name+".txt"))
	if err != nil {
		return common.Address{}, fmt.Errorf("reading the %s deployment: %w", name, err)
	}
	return parseAddress(name, strings.TrimSpace(string(data)))
}

// readChainFacts loads the dump into a temporary Anvil and records the facts
// that replay checks and verify compares against.
func readChainFacts(ctx context.Context, dump, requestedApp string, m *caseManifest) ([]string, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(filepath.Dir(dump), "capture-anvil.log")
	scratch, err := os.MkdirTemp("", "daveinterop-capture-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	env, err := anvilEnvironment(scratch)
	if err != nil {
		return nil, err
	}
	anvil, err := startChild("anvil (capture)", logPath, filepath.Dir(dump), env, "anvil",
		anvilArgs(dump, port, 1, defaultPersistedStates)...)
	if err != nil {
		return nil, withExitCode(exitPrerequisites, err)
	}
	defer anvil.stop()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := waitForPort(ctx, addr, anvil, anvilStartTimeout); err != nil {
		return nil, err
	}
	c, err := dialChain(ctx, "http://"+addr)
	if err != nil {
		return nil, err
	}
	defer c.close()
	step("capture", "reading chain facts from the dump")

	if m.Chain.ChainID, err = c.chainID(ctx); err != nil {
		return nil, err
	}
	header, err := c.head(ctx)
	if err != nil {
		return nil, err
	}
	head := header.Number.Uint64()
	m.Chain.Head, m.Chain.HeadHash, m.Chain.SlotsInAnEpoch = head, header.Hash(), 1
	if head >= defaultPersistedStates {
		return nil, fmt.Errorf("the dump has %d blocks; Anvil keeps only %d historical states", head, defaultPersistedStates)
	}
	if err := c.waitForHistory(ctx, "capture", head, anvilStartTimeout); err != nil {
		return nil, err
	}

	apps, err := c.daveApps(ctx, m.Deployments.DaveAppFactory, head)
	if err != nil {
		return nil, err
	}
	app, err := selectDaveApp(apps, requestedApp)
	if err != nil {
		return nil, err
	}
	m.Application.Address, m.Application.Consensus, m.Application.DeployBlock = app.App, app.Consensus, app.DeployBlock
	if m.Application.TemplateHash, err = c.templateHash(ctx, app.App, head); err != nil {
		return nil, err
	}
	if _, err := c.inputCount(ctx, m.Deployments.InputBox, app.App, app.DeployBlock); err != nil {
		return nil, fmt.Errorf("the dump lacks the historical state at the deployment block: %w", err)
	}
	if m.Reference.InputCount, err = c.inputCount(ctx, m.Deployments.InputBox, app.App, head); err != nil {
		return nil, err
	}
	if m.Reference.SealedEpochs, err = c.sealedEpochs(ctx, app.Consensus, head); err != nil {
		return nil, err
	}
	staged, err := c.stagedEpochs(ctx, app.Consensus, head)
	if err != nil {
		return nil, err
	}
	for _, index := range sortedKeys(staged) {
		m.Reference.StagedEpochs = append(m.Reference.StagedEpochs, staged[index])
	}
	roots := make(map[common.Address]uint64, len(m.Reference.SealedEpochs))
	for _, sealed := range m.Reference.SealedEpochs {
		roots[sealed.Tournament] = sealed.Index
	}
	tree, err := c.tournamentTree(ctx, roots, head)
	if err != nil {
		return nil, err
	}
	m.Reference.Tournaments = len(tree)

	// The harness runs the Sling node as test account 7 (Dave.wallet_address, also its sentry).
	m.Reference.Signer = defaultSlingSigner(m.Chain.ChainID)
	if m.Reference.Signer != (common.Address{}) {
		for _, sealed := range m.Reference.SealedEpochs {
			joins, err := c.joins(ctx, sealed.Tournament, head)
			if err != nil {
				return nil, err
			}
			for _, join := range joins {
				if join.Submitter == m.Reference.Signer {
					m.Reference.SignerJoins++
				}
			}
		}
		sentries, err := c.sentryClaims(ctx, app.Consensus, m.Reference.Signer, head)
		if err != nil {
			return nil, err
		}
		m.Reference.SignerSentries = len(sentries)
		if m.Reference.SignerJoins+m.Reference.SignerSentries == 0 {
			m.Notes = append(m.Notes, "the assumed Sling node signer "+m.Reference.Signer.Hex()+" made no joins or sentry claims")
		}
	}
	return slingEvidenceProblems(ctx, c, m, staged, head)
}

// slingEvidenceProblems cross-checks the Sling node's stage log with the chain:
// a finished root must have the claimed commitment as winner, and an epoch
// that the Sling node staged must have a claim in its log.
func slingEvidenceProblems(ctx context.Context, c *chain, m *caseManifest, staged map[uint64]stagedEpoch, head uint64,
) ([]string, error) {
	var problems []string
	claims := m.Reference.claimMap()
	for _, sealed := range m.Reference.SealedEpochs {
		standing, err := c.rootStanding(ctx, sealed.Tournament, head)
		if err != nil {
			return nil, err
		}
		if claim, ok := claims[sealed.Index]; ok && standing.HasWinner && claim != standing.Winner {
			problems = append(problems, fmt.Sprintf("epoch %d: the Sling stage log claims %s, but the root winner is %s",
				sealed.Index, claim, standing.Winner))
		}
	}
	if m.Reference.Signer == (common.Address{}) || m.Artifacts.DaveLog == "" {
		return problems, nil
	}
	for _, index := range sortedKeys(staged) {
		sender, err := c.txSender(ctx, staged[index].Tx)
		if err != nil {
			return nil, err
		}
		if _, ok := claims[index]; sender == m.Reference.Signer && !ok {
			problems = append(problems, fmt.Sprintf("epoch %d: the Sling node staged it, but its log has no claim", index))
		}
	}
	return problems, nil
}

func writeCaptureFailure(stdout io.Writer, out string, m *caseManifest, reason string) {
	m.Notes = append(m.Notes, reason)
	_ = writeJSONFile(filepath.Join(out, "capture-failure.json"), m)
	_ = emit(stdout, m)
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cartesi/rollups-node/internal/model"
)

const (
	anvilStartTimeout = 3 * time.Minute // loading a large dump takes a while
	nodeStartTimeout  = time.Minute
	cliTimeout        = 5 * time.Minute

	jsonrpcPortOffset = 11
	inspectPortOffset = 12
)

type replayOptions struct {
	caseDir          string
	runDir           string
	anvilPort        int
	nodePortBase     int
	dbAdmin          string
	dbName           string
	dropDB           bool
	submission       bool
	keep             bool
	wait             time.Duration
	nodeBin          string
	cliBin           string
	requireAgreement bool
	rollupsPrtSigner string
	// persistedStates bounds how many historical states Anvil keeps on disk.
	persistedStates uint64
	// The --then continuations, and the miner that they and live use.
	continuations       []string
	mineStep            uint64
	mineInterval        time.Duration
	continuationTimeout time.Duration
	depositWei          string
}

type replayReport struct {
	Case       string          `json:"case"`
	Golden     bool            `json:"golden"`
	Provenance *caseProvenance `json:"provenance,omitempty"`
	RunDir     string          `json:"run_dir"`
	Database   string          `json:"database"`
	RPC        string          `json:"rpc"`
	API        string          `json:"api"`
	Submission bool            `json:"submission"`
	Verify     *verifyReport   `json:"verify,omitempty"`
	Passed     bool            `json:"passed"`
	Error      string          `json:"error,omitempty"`
	// Continuations are the --then results; FinalVerify checks the rollups
	// node again after them.
	Continuations []*continuationReport `json:"continuations,omitempty"`
	FinalVerify   *verifyReport         `json:"final_verify,omitempty"`
}

const (
	contRollupsAlone    = "rollups-continues-alone"
	contOutputExecution = "output-execution"
)

func newReplayCommand() *cobra.Command {
	opts := &replayOptions{}
	cmd := &cobra.Command{
		Use:   "replay --case DIR",
		Short: "Load a captured chain, attach a fresh rollups node, and verify it",
		Long: `replay loads a captured harness chain into a test-owned Anvil (mining off),
creates a fresh database, registers the captured application, starts the
rollups node, waits for positive completion predicates, and verifies the node
against the chain and the Sling node's evidence in the manifest.

With --then, it continues on the same chain after verification: it mines
blocks, runs the rollups-only continuations (rollups-continues-alone,
output-execution), stops mining, and verifies the node again at the new head.
The continuations need claim submission, and the node cannot change that on an
existing database, so --then turns it on from the start. The first
verification is not affected: with mining off, nothing the node sends is
mined before the continuations start.

Nothing outside the run directory, the new database, and the selected ports
is touched. Use --keep to leave the chain and node running for inspection.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(opts.continuations) > 0 && !cmd.Flags().Changed("submission") {
				opts.submission = true
			}
			report, err := executeReplay(cmd.Context(), opts, cmd.OutOrStdout())
			return replayExit(report, err)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.caseDir, "case", "", "case directory with manifest.json (required)")
	flags.StringVar(&opts.runDir, "run-dir", "", "run directory (default: <case>/runs/<timestamp>)")
	flags.IntVar(&opts.anvilPort, "anvil-port", defaultAnvilPort, "port of the test-owned Anvil")
	flags.IntVar(&opts.nodePortBase, "node-port-base", defaultNodePortBase,
		"node telemetry port; JSON-RPC API uses +11 and inspect +12")
	flags.StringVar(&opts.dbAdmin, "db-admin", defaultDatabaseURL(),
		"Postgres URL used to create the run database (default: $CARTESI_DATABASE_CONNECTION)")
	flags.StringVar(&opts.dbName, "db-name", "", "name of the new database (default: interop_<case>_<time>)")
	flags.BoolVar(&opts.dropDB, "drop-db", false, "drop the run database after a passing run")
	flags.BoolVar(&opts.submission, "submission", false, "enable claim submission in the rollups node (default true with --then)")
	flags.BoolVar(&opts.keep, "keep", false, "keep Anvil and the node running until interrupted")
	flags.DurationVar(&opts.wait, "wait", 10*time.Minute, "deadline for the completion predicates") //nolint:mnd
	flags.StringVar(&opts.nodeBin, "node-bin", "./cartesi-rollups-node", "rollups node binary")
	flags.StringVar(&opts.cliBin, "cli-bin", "./cartesi-rollups-cli", "rollups CLI binary")
	flags.BoolVar(&opts.requireAgreement, "require-agreement", false, "fail when no compared epoch changed state")
	flags.StringSliceVar(&opts.continuations, "then", nil,
		"continuations to run after verification: rollups-continues-alone, output-execution")
	flags.Uint64Var(&opts.mineStep, "mine-step", 10, "blocks mined per step while continuations run") //nolint:mnd
	flags.DurationVar(&opts.mineInterval, "mine-interval", time.Second, "time between mining steps")
	flags.DurationVar(&opts.continuationTimeout, "continuation-timeout", 20*time.Minute, "deadline of each continuation") //nolint:mnd
	flags.StringVar(&opts.rollupsPrtSigner, "rollups-prt-signer", "",
		"rollups PRT signer address (default: derived from CARTESI_PRT_AUTH_MNEMONIC and its index)")
	flags.StringVar(&opts.depositWei, "deposit-amount", "1000", "token amount for output-execution")
	addAnvilFlags(flags, opts)
	_ = cmd.MarkFlagRequired("case")
	return cmd
}

// session is one run of the tool: a test-owned chain and its nodes.
type session struct {
	// stage labels the progress lines of shared steps, such as "replay".
	stage    string
	opts     *replayOptions
	manifest *caseManifest
	runDir   string
	anvil    *child
	node     *child
	sling    *child
	// expandedDump is a decompressed copy of the case dump, removed on stop.
	expandedDump string
	chain        *chain
	api          *nodeAPI
	rpcURL       string
	apiURL       string
	dbURL        string
	dbName       string
	appName      string
	env          []string

	// The miner goroutine reports why it stopped through minerErr.
	minerMu  sync.Mutex
	minerErr error
}

func (s *session) stageName() string {
	if s.stage == "" {
		return "replay"
	}
	return s.stage
}

// replayExit maps a replay result to the command's exit code.
func replayExit(report *replayReport, err error) error {
	if err != nil {
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			return err
		}
		return withExitCode(exitFailure, err)
	}
	if !report.Passed {
		return withExitCode(exitFailure, errors.New("replay checks failed; see the report"))
	}
	return nil
}

// executeReplay runs one replay and returns its report. The report is also
// written to out (before --keep waits) and to <run-dir>/report.json. A
// returned error means the replay could not run; failed checks are only in
// the report.
func executeReplay(ctx context.Context, opts *replayOptions, out io.Writer) (*replayReport, error) {
	for _, name := range opts.continuations {
		if name != contRollupsAlone && name != contOutputExecution {
			return nil, withExitCode(exitUsage, fmt.Errorf("unknown continuation %q", name))
		}
	}
	if len(opts.continuations) > 0 && !opts.submission {
		return nil, withExitCode(exitUsage, errors.New("--then needs claim submission; drop --submission=false"))
	}
	if opts.dbAdmin == "" {
		return nil, withExitCode(exitPrerequisites, errors.New("no Postgres URL: set CARTESI_DATABASE_CONNECTION or --db-admin"))
	}
	ctx, stopNotify := signal.NotifyContext(ctx, stopSignals...)
	defer stopNotify()

	s := &session{opts: opts}
	report := &replayReport{Submission: opts.submission}
	err := s.start(ctx, report)
	if err == nil {
		step("verify", "waiting for the rollups node to index the chain, then comparing")
		report.Verify, err = verifyUntil(ctx, s.verifyTarget(s.manifest.Chain.Head), opts.wait)
		if err == nil {
			step("verify", "%s (%s)", passFail(report.Verify.Passed), coverageText(report.Verify.Coverage))
		}
	}
	if err == nil && len(opts.continuations) > 0 {
		if !report.Verify.Passed {
			logger.Warn("verification failed; running the continuations anyway")
		}
		report.Continuations, report.FinalVerify, err = s.continueReplay(ctx)
	}
	report.Passed = err == nil && report.Verify != nil && report.Verify.Passed
	for _, c := range report.Continuations {
		report.Passed = report.Passed && c.Passed
	}
	if len(opts.continuations) > 0 {
		report.Passed = report.Passed && report.FinalVerify != nil && report.FinalVerify.Passed
	}
	if err != nil {
		report.Error = err.Error()
	}
	if s.runDir != "" {
		if writeErr := writeJSONFile(filepath.Join(s.runDir, "report.json"), report); writeErr != nil {
			logger.Warn("writing report", "error", writeErr)
		}
	}
	if printErr := emit(out, report); printErr != nil {
		logger.Warn("printing the report", "error", printErr)
	}

	if opts.keep && err == nil {
		logger.Info("keeping the chain and node running; press Ctrl-C to stop",
			"rpc", s.rpcURL, "api", s.apiURL, "database", s.dbName, "app", s.appName)
		<-ctx.Done()
	}
	s.stop()
	if report.Passed {
		s.removeSnapshots()
	}
	if report.Passed && opts.dropDB && s.dbName != "" {
		if dropErr := dropDatabase(context.Background(), opts.dbAdmin, s.dbName); dropErr != nil {
			logger.Warn("dropping the run database", "error", dropErr)
		}
	}
	return report, err
}

func (s *session) start(ctx context.Context, report *replayReport) error {
	opts := s.opts
	// The node and Anvil run with the run directory as working directory, so
	// every path handed to them must be absolute.
	if err := absolutePaths(&opts.caseDir, &opts.runDir); err != nil {
		return err
	}
	m, err := readManifest(opts.caseDir)
	if err != nil {
		return withExitCode(exitPrerequisites, err)
	}
	s.manifest = m
	report.Case, report.Golden, report.Provenance = m.Case, m.Golden, &m.Provenance
	if !m.Golden {
		logger.Warn("the case is not golden; results are diagnostic only", "harness_exit_code", m.Provenance.HarnessExitCode)
	}

	s.runDir = opts.runDir
	if s.runDir == "" {
		s.runDir = filepath.Join(opts.caseDir, "runs", time.Now().UTC().Format("20060102T150405Z"))
	}
	if _, err := os.Stat(s.runDir); err == nil {
		return fmt.Errorf("run directory %s exists; each attempt needs a new one", s.runDir)
	}
	if err := os.MkdirAll(filepath.Join(s.runDir, "snapshots"), 0o755); err != nil { //nolint:mnd
		return err
	}
	report.RunDir = s.runDir

	for _, port := range []int{opts.anvilPort, opts.nodePortBase, opts.nodePortBase + jsonrpcPortOffset,
		opts.nodePortBase + inspectPortOffset} {
		if err := requirePortFree(port); err != nil {
			return withExitCode(exitPrerequisites, err)
		}
	}
	if err := resolveBinaries(&opts.nodeBin, &opts.cliBin); err != nil {
		return err
	}
	if _, err := exec.LookPath("anvil"); err != nil {
		return withExitCode(exitPrerequisites, fmt.Errorf("anvil not found: %w", err))
	}

	stateFile, err := s.prepareDump()
	if err != nil {
		return err
	}
	step("replay", "loading the chain into Anvil on port %d", opts.anvilPort)
	if err := s.startAnvil(ctx, stateFile, true); err != nil {
		return err
	}
	if err := s.checkChain(ctx); err != nil {
		return err
	}
	step("replay", "chain %d at block %d matches the case; history readable", m.Chain.ChainID, m.Chain.Head)
	templateDir, err := s.prepareTemplate()
	if err != nil {
		return err
	}
	if err := s.createDatabase(ctx); err != nil {
		return err
	}
	report.Database = s.dbName
	report.RPC = s.rpcURL
	if err := s.buildEnvironment(); err != nil {
		return err
	}
	s.appName = "interop-" + strings.ReplaceAll(sanitizeName(m.Case), "_", "-")
	if _, err := s.runCLI(ctx, "app", "register", "--prt", "-n", s.appName, "-a", m.Application.Address.Hex(),
		"-t", templateDir); err != nil {
		return fmt.Errorf("registering the application: %w", err)
	}
	step("replay", "registered %s as %s (claim submission %t)", m.Application.Address, s.appName, opts.submission)
	if err := s.startNode(ctx); err != nil {
		return err
	}
	step("replay", "rollups node started; API %s; log %s", s.apiURL, displayPath(filepath.Join(s.runDir, "node.log")))
	report.API = s.apiURL
	return nil
}

// prepareDump verifies the compressed dump and expands it into the run directory.
func (s *session) prepareDump() (string, error) {
	m := s.manifest
	dump := filepath.Join(s.opts.caseDir, m.Artifacts.Dump.Path)
	sum, _, err := sha256File(dump)
	if err != nil {
		return "", withExitCode(exitPrerequisites, fmt.Errorf("reading dump: %w", err))
	}
	if sum != m.Artifacts.Dump.SHA256 {
		return "", fmt.Errorf("dump checksum %s does not match manifest %s", sum, m.Artifacts.Dump.SHA256)
	}
	if !strings.HasSuffix(dump, ".zst") {
		return dump, nil
	}
	stateFile := filepath.Join(s.runDir, "anvil-state.json")
	logger.Debug("expanding the dump", "from", dump)
	s.expandedDump = stateFile // removed on stop, even when partly written
	if err := decompressZstd(dump, stateFile); err != nil {
		return "", err
	}
	rawSum, _, err := sha256File(stateFile)
	if err != nil {
		return "", err
	}
	if m.Artifacts.Dump.RawSHA256 != "" && rawSum != m.Artifacts.Dump.RawSHA256 {
		return "", fmt.Errorf("expanded dump checksum %s does not match manifest %s", rawSum, m.Artifacts.Dump.RawSHA256)
	}
	return stateFile, nil
}

// startAnvil starts the test-owned Anvil. With waitHistory, it also waits
// until the dump's historical states are readable.
func (s *session) startAnvil(ctx context.Context, stateFile string, waitHistory bool) error {
	slots := s.manifest.Chain.SlotsInAnEpoch
	if slots == 0 {
		slots = 1
	}
	env, err := anvilEnvironment(s.runDir)
	if err != nil {
		return err
	}
	s.anvil, err = startChild("anvil", filepath.Join(s.runDir, "anvil.log"), s.runDir, env, "anvil",
		anvilArgs(stateFile, s.opts.anvilPort, slots, s.opts.persistedStates)...)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", s.opts.anvilPort)
	if err := waitForPort(ctx, addr, s.anvil, anvilStartTimeout); err != nil {
		return err
	}
	s.rpcURL = "http://" + addr
	if s.chain, err = dialChain(ctx, s.rpcURL); err != nil || !waitHistory {
		return err
	}
	header, err := s.chain.head(ctx)
	if err != nil {
		return err
	}
	head := header.Number.Uint64()
	if head >= s.opts.persistedStates {
		return fmt.Errorf("the chain has %d blocks; raise --anvil-persisted-states (%d)", head, s.opts.persistedStates)
	}
	return s.chain.waitForHistory(ctx, "replay", head, anvilStartTimeout)
}

// anvilArgs is the command line of a test-owned Anvil: mining off, and enough
// retention to keep every historical state (see defaultPersistedStates).
func anvilArgs(stateFile string, port, slotsInAnEpoch int, persistedStates uint64) []string {
	return []string{"--load-state", stateFile, "--port", strconv.Itoa(port),
		"--slots-in-an-epoch", strconv.Itoa(slotsInAnEpoch), "--no-mining",
		"--max-persisted-states", strconv.FormatUint(persistedStates, 10)}
}

// checkChain confirms that the loaded chain is the captured one and that its
// historical state is available at the deployment block.
func (s *session) checkChain(ctx context.Context) error {
	m := s.manifest
	chainID, err := s.chain.chainID(ctx)
	if err != nil {
		return err
	}
	if chainID != m.Chain.ChainID {
		return fmt.Errorf("loaded chain id %d, manifest %d", chainID, m.Chain.ChainID)
	}
	header, err := s.chain.head(ctx)
	if err != nil {
		return err
	}
	if header.Number.Uint64() != m.Chain.Head || header.Hash() != m.Chain.HeadHash {
		return fmt.Errorf("loaded head %d %s, manifest %d %s", header.Number, header.Hash(), m.Chain.Head, m.Chain.HeadHash)
	}
	for _, block := range []uint64{m.Application.DeployBlock, m.Chain.Head} {
		if _, err := s.chain.inputCount(ctx, m.Deployments.InputBox, m.Application.Address, block); err != nil {
			return fmt.Errorf("captured history is incomplete (fixture failure): %w", err)
		}
	}
	count, err := s.chain.inputCount(ctx, m.Deployments.InputBox, m.Application.Address, m.Chain.Head)
	if err != nil {
		return err
	}
	if count != m.Reference.InputCount {
		return fmt.Errorf("chain has %d inputs, manifest %d", count, m.Reference.InputCount)
	}
	// The head hash already fixes the chain; these catch a manifest whose
	// reference facts were not read from this chain.
	sealed, err := s.chain.sealedEpochs(ctx, m.Application.Consensus, m.Chain.Head)
	if err != nil {
		return err
	}
	if !slices.Equal(sealed, m.Reference.SealedEpochs) {
		return fmt.Errorf("the chain has %d sealed epochs that differ from the %d in the manifest", len(sealed),
			len(m.Reference.SealedEpochs))
	}
	staged, err := s.chain.stagedEpochs(ctx, m.Application.Consensus, m.Chain.Head)
	if err != nil {
		return err
	}
	stagedList := make([]stagedEpoch, 0, len(staged))
	for _, index := range sortedKeys(staged) {
		stagedList = append(stagedList, staged[index])
	}
	if !slices.Equal(stagedList, m.Reference.StagedEpochs) {
		return fmt.Errorf("the chain has %d staged epochs that differ from the %d in the manifest", len(stagedList),
			len(m.Reference.StagedEpochs))
	}
	return nil
}

func (s *session) prepareTemplate() (string, error) {
	src := filepath.Join(s.opts.caseDir, s.manifest.Application.TemplateDir)
	dst := filepath.Join(s.runDir, "template")
	if err := copyTree(src, dst); err != nil {
		return "", fmt.Errorf("copying the template: %w", err)
	}
	hash, err := storedMachineHash(dst)
	if err != nil {
		logger.Warn("cannot check the template hash", "error", err)
		return dst, nil
	}
	if common.HexToHash(hash) != s.manifest.Application.TemplateHash {
		return "", fmt.Errorf("template hash %s does not match manifest %s", hash, s.manifest.Application.TemplateHash)
	}
	return dst, nil
}

var nonNameCharacters = regexp.MustCompile(`[^a-z0-9_]+`)

func sanitizeName(value string) string {
	return strings.Trim(nonNameCharacters.ReplaceAllString(strings.ToLower(value), "_"), "_")
}

// runDatabaseName names the database of one run: interop_<case>_<time>,
// shortened to the Postgres identifier limit. Shortening cuts the case name,
// never the prefix.
func runDatabaseName(caseName string, now time.Time) string {
	const maxIdentifier = 63
	prefix, suffix := interopDatabasePrefix, "_"+now.UTC().Format("20060102t150405")
	name := sanitizeName(caseName)
	if room := maxIdentifier - len(prefix) - len(suffix); len(name) > room {
		name = strings.TrimRight(name[:room], "_")
	}
	return prefix + name + suffix
}

func (s *session) createDatabase(ctx context.Context) error {
	s.dbName = s.opts.dbName
	if s.dbName == "" {
		s.dbName = runDatabaseName(s.manifest.Case, time.Now())
	}
	url, err := createDatabase(ctx, s.opts.dbAdmin, s.dbName)
	if err != nil {
		return err
	}
	s.dbURL = url
	step(s.stageName(), "created database %s", s.dbName)
	return nil
}

// buildEnvironment derives the node and CLI environment from ours. Run-specific
// values always override the operator's environment so that nothing points at
// another chain, database, or port.
func (s *session) buildEnvironment() error {
	m := s.manifest
	overrides := map[string]string{
		"CARTESI_DATABASE_CONNECTION":                s.dbURL,
		"CARTESI_BLOCKCHAIN_HTTP_ENDPOINT":           s.rpcURL,
		"CARTESI_BLOCKCHAIN_ID":                      strconv.FormatUint(m.Chain.ChainID, 10),
		"CARTESI_BLOCKCHAIN_DEFAULT_BLOCK":           "latest",
		"CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED":   strconv.FormatBool(s.opts.submission),
		"CARTESI_SNAPSHOTS_DIR":                      filepath.Join(s.runDir, "snapshots"),
		"CARTESI_NODE_TELEMETRY_ADDRESS":             fmt.Sprintf("127.0.0.1:%d", s.opts.nodePortBase),
		"CARTESI_JSONRPC_API_ADDRESS":                fmt.Sprintf("127.0.0.1:%d", s.opts.nodePortBase+jsonrpcPortOffset),
		"CARTESI_INSPECT_ADDRESS":                    fmt.Sprintf("127.0.0.1:%d", s.opts.nodePortBase+inspectPortOffset),
		"CARTESI_JSONRPC_API_URL":                    fmt.Sprintf("http://127.0.0.1:%d/rpc", s.opts.nodePortBase+jsonrpcPortOffset),
		"CARTESI_LOG_COLOR":                          flagFalse,
		"CARTESI_CONTRACTS_INPUT_BOX_ADDRESS":        m.Deployments.InputBox.Hex(),
		"CARTESI_CONTRACTS_DAVE_APP_FACTORY_ADDRESS": m.Deployments.DaveAppFactory.Hex(),
	}
	defaults := map[string]string{}
	if m.Chain.ChainID == anvilChainID {
		defaults = map[string]string{
			"CARTESI_AUTH_KIND": authKindMnemonic, "CARTESI_AUTH_MNEMONIC": testMnemonic,
			"CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX": strconv.Itoa(defaultCLIIndex),
			"CARTESI_PRT_AUTH_KIND":               authKindMnemonic, envPrtMnemonic: testMnemonic,
			"CARTESI_PRT_AUTH_MNEMONIC_ACCOUNT_INDEX": strconv.Itoa(defaultRollupsPrtIndex),
		}
	}
	s.env = mergeEnvironment(os.Environ(), defaults, overrides)
	s.apiURL = overrides["CARTESI_JSONRPC_API_URL"]
	s.api = newNodeAPI(s.apiURL)
	return nil
}

// mergeEnvironment applies defaults (only when unset) and then overrides.
func mergeEnvironment(base []string, defaults, overrides map[string]string) []string {
	values := map[string]string{}
	var order []string
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, seen := values[key]; !seen {
			order = append(order, key)
		}
		values[key] = value
	}
	for key, value := range defaults {
		if values[key] == "" {
			if _, seen := values[key]; !seen {
				order = append(order, key)
			}
			values[key] = value
		}
	}
	for key, value := range overrides {
		if _, seen := values[key]; !seen {
			order = append(order, key)
		}
		values[key] = value
	}
	env := make([]string, 0, len(order))
	for _, key := range order {
		env = append(env, key+"="+values[key])
	}
	return env
}

func lookupEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == key {
			return v
		}
	}
	return ""
}

// runCLI runs the rollups CLI with the session environment. Its stderr goes to
// cli.log; its stdout is returned.
func (s *session) runCLI(ctx context.Context, args ...string) (string, error) {
	return s.runTool(ctx, s.opts.cliBin, "cli.log", s.env, args...)
}

// runTool runs a rollups tool. Its stderr goes to the log file; its stdout is
// returned.
func (s *session) runTool(ctx context.Context, bin, logName string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()
	logFile, err := os.OpenFile(filepath.Join(s.runDir, logName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644) //nolint:mnd
	if err != nil {
		return "", err
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "\n$ %s %s\n", bin, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(logFile, &stderr)
	out, err := cmd.Output()
	fmt.Fprintf(logFile, "%s\n", out)
	if err != nil {
		// The last line of the CLI's error output usually names the cause,
		// for example a decoded revert.
		reason := err.Error()
		if last := lastLine(stderr.String()); last != "" {
			reason = last
		}
		return string(out), fmt.Errorf("%s %s: %s (see %s)", filepath.Base(bin), strings.Join(args, " "), reason, logName)
	}
	return string(out), nil
}

// continueReplay runs the continuations while mining, stops mining, and
// verifies the node again at the new head.
func (s *session) continueReplay(ctx context.Context) ([]*continuationReport, *verifyReport, error) {
	reports := s.runContinuations(ctx, s.opts.continuations)
	if err := s.minerError(); err != nil {
		return reports, nil, err
	}
	header, err := s.chain.head(ctx)
	if err != nil {
		return reports, nil, err
	}
	head := header.Number.Uint64()
	step("verify", "mining stopped at block %d; checking the rollups node again", head)
	target := s.verifyTarget(head)
	target.requireAgreement = false
	final, err := verifyUntil(ctx, target, s.opts.wait)
	if err == nil {
		step("verify", "after the continuations: %s", passFail(final.Passed))
	}
	return reports, final, err
}

// lastLine returns the last non-empty line of text.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func (s *session) startNode(ctx context.Context) error {
	var err error
	s.node, err = startChild("node", filepath.Join(s.runDir, "node.log"), s.runDir, s.env, s.opts.nodeBin)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", s.opts.nodePortBase+jsonrpcPortOffset)
	return waitForPort(ctx, addr, s.node, nodeStartTimeout)
}

// lookPathAbs resolves a binary name or path to an absolute path.
func lookPathAbs(bin string) (string, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

// resolveBinaries turns binary names or relative paths into absolute paths.
func resolveBinaries(bins ...*string) error {
	for _, bin := range bins {
		path, err := lookPathAbs(*bin)
		if err != nil {
			return withExitCode(exitPrerequisites, fmt.Errorf("%s not found (run make build?): %w", *bin, err))
		}
		*bin = path
	}
	return nil
}

// absolutePaths replaces each non-empty path with its absolute form.
func absolutePaths(paths ...*string) error {
	for _, path := range paths {
		if *path == "" {
			continue
		}
		abs, err := filepath.Abs(*path)
		if err != nil {
			return err
		}
		*path = abs
	}
	return nil
}

func (s *session) verifyTarget(atBlock uint64) *verifyTarget {
	m := s.manifest
	rollupsSigner, err := s.rollupsPrtSigner()
	if err != nil {
		logger.Warn("the rollups PRT signer is unknown; not checking that the Sling signer differs", "error", err)
	}
	return &verifyTarget{
		chain: s.chain, api: s.api, app: s.appName, appAddress: m.Application.Address,
		consensus: m.Application.Consensus, inputBox: m.Deployments.InputBox, claims: m.Reference.claimMap(),
		expectedStatus: model.ApplicationStatus_OK, requireAgreement: s.opts.requireAgreement, atBlock: atBlock,
		slingSigner: m.Reference.Signer, rollupsSigner: rollupsSigner, watch: s.checkParticipants,
	}
}

// checkParticipants returns an error when a process of the run has exited or
// the miner has stopped.
func (s *session) checkParticipants() error {
	if err := s.minerError(); err != nil {
		return err
	}
	for _, participant := range []*child{s.anvil, s.node, s.sling} {
		if participant == nil {
			continue
		}
		if err := participant.errIfExited(); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) stop() {
	if s.node != nil {
		s.node.stop()
	}
	if s.anvil != nil {
		s.anvil.stop()
	}
	if s.chain != nil {
		s.chain.close()
	}
	// The Anvil state cache, the expanded dump, and the template copies are
	// only useful to running processes, and they are large. The compressed
	// dump and the template stay in the case.
	temporary := []string{s.expandedDump}
	if s.runDir != "" {
		temporary = append(temporary, filepath.Join(s.runDir, anvilHomeDir))
		copies, _ := filepath.Glob(filepath.Join(s.runDir, "template*"))
		temporary = append(temporary, copies...)
	}
	s.removeAll(temporary...)
}

// removeSnapshots removes the machine snapshots of a passing run. A failing
// run keeps them for diagnosis.
func (s *session) removeSnapshots() {
	if s.runDir != "" {
		s.removeAll(filepath.Join(s.runDir, "snapshots"))
	}
}

func (s *session) removeAll(paths ...string) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			logger.Warn("removing a temporary file", "path", path, "error", err)
		}
	}
}

func writeJSONFile(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return writeJSON(file, value)
}

// defaultPersistedStates keeps the full history of a replay that mines a few
// thousand blocks. The rollups node reads counters back to the deployment block
// (for example, when the first output is executed). Anvil evicts older states
// beyond this bound, and those reads then fail with BlockOutOfRangeError.
// Each state takes about 0.6 MB of disk in the run directory while Anvil runs.
const defaultPersistedStates = 20000

func addAnvilFlags(flags *pflag.FlagSet, opts *replayOptions) {
	flags.Uint64Var(&opts.persistedStates, "anvil-persisted-states", defaultPersistedStates,
		"historical states Anvil keeps on disk (--max-persisted-states)")
}

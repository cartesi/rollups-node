// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/model"
)

const (
	scenarioSmoke = "smoke"

	orderConcurrent   = "concurrent"
	orderRollupsFirst = "rollups-first"
	orderSlingFirst   = "sling-first"

	partyRollups = "rollups"
	partySling   = "sling"

	defaultInputSenderIx = 3
	minimumLiveInputs    = 3 // echo rejects input number 2; at least one input is accepted
	saltBytes            = 32

	// liveSlingSleepSeconds is the Sling node's polling interval in live runs.
	liveSlingSleepSeconds = 1
)

// livePolling makes the rollups node poll as often as the Sling node, so
// that the races between them (join, stage, accept) are fair. The defaults
// are 3 s (12 s for the EVM reader).
var livePolling = map[string]string{
	"CARTESI_EVM_READER_POLLING_INTERVAL": "1",
	"CARTESI_ADVANCER_POLLING_INTERVAL":   "1",
	"CARTESI_VALIDATOR_POLLING_INTERVAL":  "1",
	"CARTESI_CLAIMER_POLLING_INTERVAL":    "1",
	"CARTESI_PRT_POLLING_INTERVAL":        "1",
}

type liveOptions struct {
	replayOptions
	scenario         string
	daveRoot         string
	slingBin         string
	program          string
	template         string
	requireAgreement string
	anvilState       string
	order            string
	inputs           int
	stagingPeriod    uint64
	slingSentry      bool
	slingIndex       int
	timeout          time.Duration
}

type liveJoin struct {
	Party  string      `json:"party"`
	Block  uint64      `json:"block"`
	Tx     common.Hash `json:"transaction_hash"`
	Commit common.Hash `json:"commitment"`
}

type liveReport struct {
	RunDir        string         `json:"run_dir"`
	Scenario      string         `json:"scenario"`
	Order         string         `json:"order"`
	Program       string         `json:"program"`
	Application   common.Address `json:"application"`
	Database      string         `json:"database"`
	RollupsSigner common.Address `json:"rollups_signer"`
	SlingSigner   common.Address `json:"sling_signer"`
	SlingSentry   bool           `json:"sling_sentry"`
	InputEpoch    uint64         `json:"input_epoch"`
	InputTxs      []common.Hash  `json:"input_transactions"`
	Joins         []liveJoin     `json:"input_epoch_joins"`
	OrderChecked  checkResult    `json:"order_check"`
	// Epochs is the epochs table: the plan, the chain, and the rollups node.
	Epochs []liveEpoch `json:"epochs,omitempty"`
	// Steps are the scenario's checks, in sections (see stepGroups).
	Steps  []continuationStep `json:"steps,omitempty"`
	Verify *verifyReport      `json:"verify,omitempty"`
	Passed bool               `json:"passed"`
	Error  string             `json:"error,omitempty"`
}

func newLiveCommand() *cobra.Command {
	opts := &liveOptions{}
	cmd := &cobra.Command{
		Use:   "live [--scenario smoke|full] [--order concurrent|rollups-first|sling-first]",
		Short: "Run the rollups node and the Sling node together on a test-owned chain",
		Long: `live starts a test-owned Anvil from the Dave devnet bundle (mining off; this
command mines), deploys a PRT application with the rollups CLI, starts the
rollups node and the Sling node in the selected order, sends inputs, waits
until their epochs are accepted, freezes the chain, and verifies the rollups
node against the chain and the Sling node's own evidence: its joins, its
sentry claims, and its stage log. Both nodes poll every second, so the races
between them are fair.

Scenarios:
  smoke  one epoch of plain inputs (default). --program echo (default) or
         honeypot selects the Dave test program; --template DIR any other.
         echo accepts the inputs, so computation agreement is required;
         honeypot rejects them.
  full   the test dapp of "make interop-live-dapp" (test/dapps/interop-live;
         a payload that starts with "reject" is rejected) and five epochs:
         all accepted, accepted and rejected, all rejected, no inputs, and all
         accepted with a fake commitment that must lose by timeout. Then the
         application is funded, every voucher (1 gwei to the input sender) is
         executed and every notice validated, and the balances, gas included,
         and the bonds of every root tournament are checked. About 10 minutes.

Orders (the first epoch with inputs):
  concurrent     start both nodes before the inputs
  rollups-first  start the Sling node only after the rollups node joined
  sling-first    start the rollups node only after the Sling node joined

Only one participant can join a given commitment. With rollups-first, the
Sling node's evidence for that epoch comes from its sentry claim
(--sling-sentry, the default) or its stage log.

Signers (test mnemonic, chain 31337): CLI and deployer 0, input sender 3,
funder 4, fake commitment 5, rollups node PRT 6, Sling node 7.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.scenario != scenarioSmoke && cmd.Flags().Changed("program") {
				return withExitCode(exitUsage, fmt.Errorf("--program selects a Dave program for --scenario smoke; "+
					"--scenario %s uses its own test dapp (or --template)", opts.scenario))
			}
			if opts.scenario != scenarioSmoke && cmd.Flags().Changed("inputs") {
				return withExitCode(exitUsage, fmt.Errorf("--inputs applies to --scenario smoke; %s plans its own inputs",
					opts.scenario))
			}
			return runLive(cmd.Context(), cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.scenario, "scenario", scenarioSmoke, "smoke or full")
	flags.StringVar(&opts.daveRoot, "dave-root", os.Getenv("DAVE_ROOT"), "prepared Dave tree for the devnet bundle and the program images")
	flags.StringVar(&opts.slingBin, "sling-bin", defaultSlingBin(),
		"Sling node executable (default: $DAVE_NODE_BIN, or its build in the Dave tree)")
	flags.StringVar(&opts.program, "program", programEcho,
		"smoke: Dave test program, echo or honeypot (<dave-root>/test/programs/<program>/machine-image)")
	flags.StringVar(&opts.template, "template", "",
		"application template directory (smoke: overrides --program; full: default "+defaultLiveDapp+")")
	flags.StringVar(&opts.requireAgreement, "require-agreement", "auto",
		"fail when computation agreement is not tested: true, false, or auto (true for echo and full)")
	flags.StringVar(&opts.anvilState, "anvil-state", "", "devnet state (default: <dave-root>/cartesi-rollups/contracts/state.json)")
	flags.StringVar(&opts.order, "order", orderConcurrent, "start order: concurrent, rollups-first, or sling-first")
	flags.IntVar(&opts.inputs, "inputs", 5, "smoke: number of inputs")                                 //nolint:mnd
	flags.Uint64Var(&opts.stagingPeriod, "claim-staging-period", 50, "claim staging period in blocks") //nolint:mnd
	flags.BoolVar(&opts.slingSentry, "sling-sentry", true, "make the Sling node's signer the application's only sentry")
	flags.IntVar(&opts.slingIndex, "sling-index", defaultSlingSignerIx, "test-mnemonic index of the Sling node's signer")
	flags.DurationVar(&opts.timeout, "timeout", 20*time.Minute, "deadline for each epoch to be sealed or accepted") //nolint:mnd
	flags.StringVar(&opts.runDir, "run-dir", "", "run directory (default: _interop/live/<time>)")
	flags.IntVar(&opts.anvilPort, "anvil-port", defaultAnvilPort, "port of the test-owned Anvil")
	flags.IntVar(&opts.nodePortBase, "node-port-base", defaultNodePortBase, "node telemetry port; API +11, inspect +12")
	flags.StringVar(&opts.dbAdmin, "db-admin", defaultDatabaseURL(), "Postgres URL used to create the run database")
	flags.StringVar(&opts.dbName, "db-name", "", "name of the new database (default: interop_live_<time>)")
	flags.BoolVar(&opts.submission, "submission", true, "enable claim submission in the rollups node")
	flags.BoolVar(&opts.keep, "keep", false, "keep Anvil and both nodes running until interrupted (the chain stays frozen)")
	flags.DurationVar(&opts.wait, "wait", 5*time.Minute, "deadline for the verification predicates") //nolint:mnd
	flags.StringVar(&opts.nodeBin, "node-bin", "./cartesi-rollups-node", "rollups node binary")
	flags.StringVar(&opts.cliBin, "cli-bin", "./cartesi-rollups-cli", "rollups CLI binary")
	flags.Uint64Var(&opts.mineStep, "mine-step", 5, "blocks mined per step") //nolint:mnd
	addAnvilFlags(flags, &opts.replayOptions)
	flags.DurationVar(&opts.mineInterval, "mine-interval", time.Second, "time between mining steps")
	return cmd
}

//nolint:gocyclo,funlen // one linear procedure is easier to follow than scattered helpers
func runLive(ctx context.Context, cmd *cobra.Command, opts *liveOptions) error {
	switch opts.scenario {
	case scenarioSmoke, scenarioFull:
	default:
		return withExitCode(exitUsage, fmt.Errorf("unknown scenario %q (smoke or full)", opts.scenario))
	}
	switch opts.order {
	case orderConcurrent, orderRollupsFirst, orderSlingFirst:
	default:
		return withExitCode(exitUsage, fmt.Errorf("unknown order %q", opts.order))
	}
	switch opts.requireAgreement {
	case "auto", flagTrue, flagFalse:
	default:
		return withExitCode(exitUsage, fmt.Errorf("--require-agreement must be true, false, or auto, not %q", opts.requireAgreement))
	}
	if opts.order == orderRollupsFirst && !opts.submission {
		return withExitCode(exitUsage, errors.New("--order rollups-first needs claim submission: "+
			"the rollups node must join first"))
	}
	if opts.scenario == scenarioSmoke && opts.inputs < minimumLiveInputs {
		return withExitCode(exitUsage, fmt.Errorf("--inputs must be at least %d", minimumLiveInputs))
	}
	if opts.slingBin == "" && opts.daveRoot != "" {
		opts.slingBin = filepath.Join(opts.daveRoot, slingBinaryPath)
	}
	if opts.slingBin == "" || opts.daveRoot == "" {
		return withExitCode(exitPrerequisites, errors.New("set --dave-root or DAVE_ROOT (and --sling-bin or DAVE_NODE_BIN "+
			"for a Sling build outside the Dave tree)"))
	}
	if opts.dbAdmin == "" {
		return withExitCode(exitPrerequisites, errors.New("no Postgres URL: set CARTESI_DATABASE_CONNECTION or --db-admin"))
	}
	switch {
	case opts.scenario == scenarioFull && opts.template == "":
		opts.template, opts.program = defaultLiveDapp, filepath.Base(defaultLiveDapp)
		if !fileExists(opts.template) {
			return withExitCode(exitPrerequisites, fmt.Errorf("%s is missing; build it with make interop-live-dapp", opts.template))
		}
	case opts.template != "":
		opts.program = "custom template"
	case opts.program == programYield:
		return withExitCode(exitUsage, errors.New("the yield program breaks the outputs-root rule; it is out of scope"))
	default:
		opts.template = filepath.Join(opts.daveRoot, programsDir, opts.program, "machine-image")
	}
	if opts.anvilState == "" {
		opts.anvilState = filepath.Join(opts.daveRoot, "cartesi-rollups", "contracts", "state.json")
	}
	if opts.runDir == "" {
		opts.runDir = filepath.Join("_interop", "live", time.Now().UTC().Format("20060102T150405Z"))
	}
	if err := absolutePaths(&opts.slingBin, &opts.daveRoot, &opts.template, &opts.anvilState, &opts.runDir); err != nil {
		return err
	}
	for _, path := range []string{opts.slingBin, opts.template, opts.anvilState} {
		if _, err := os.Stat(path); err != nil {
			return withExitCode(exitPrerequisites, err)
		}
	}
	ctx, stopNotify := signal.NotifyContext(ctx, stopSignals...)
	defer stopNotify()

	s := &session{opts: &opts.replayOptions, stage: "live"}
	report := &liveReport{Scenario: opts.scenario, Order: opts.order, Program: opts.program, SlingSentry: opts.slingSentry}
	err := s.runLive(ctx, opts, report)
	report.Passed = err == nil && report.Verify != nil && report.Verify.Passed && report.OrderChecked.Status != checkFail
	for _, step := range report.Steps {
		report.Passed = report.Passed && step.Status != checkFail
	}
	for _, epoch := range report.Epochs {
		report.Passed = report.Passed && epoch.Status != checkFail
	}
	if err != nil {
		report.Error = err.Error()
	}
	if s.runDir != "" {
		_ = writeJSONFile(filepath.Join(s.runDir, "report.json"), report)
	}
	if printErr := emit(cmd.OutOrStdout(), report); printErr != nil {
		return printErr
	}
	if opts.keep && err == nil {
		logger.Info("keeping the chain and both nodes running; press Ctrl-C to stop",
			"rpc", s.rpcURL, "api", s.apiURL, "database", s.dbName)
		<-ctx.Done()
	}
	if s.sling != nil {
		s.sling.stop()
	}
	s.stop()
	if report.Passed {
		s.removeSnapshots()
		s.removeAll(filepath.Join(s.runDir, "sling-state"))
	}
	if err != nil {
		return withExitCode(exitFailure, err)
	}
	if !report.Passed {
		return withExitCode(exitFailure, errors.New("live checks failed; see the report"))
	}
	return nil
}

// liveRun is what the shared setup of a live run hands to its scenario.
type liveRun struct {
	opts          *liveOptions
	report        *liveReport
	chainID       uint64
	app           common.Address
	consensus     common.Address
	rollupsSigner common.Address
	sling         *testAccount
	sender        *testAccount
	startSling    func() error
	startRollups  func() error
	stopMiner     func()
	// full holds what the full scenario did, for its checks.
	full *fullRun
}

func (s *session) runLive(ctx context.Context, opts *liveOptions, report *liveReport) error {
	run, err := s.setupLive(ctx, opts, report)
	if run != nil {
		defer run.stopMiner()
	}
	if err != nil {
		return err
	}
	var head uint64
	switch opts.scenario {
	case scenarioFull:
		head, err = s.liveFull(ctx, run)
	default:
		head, err = s.liveSmoke(ctx, run)
	}
	if err != nil {
		// Show how far the scenario got.
		if ctx.Err() == nil {
			if header, headErr := s.chain.head(ctx); headErr == nil {
				s.recordEpochs(ctx, run, header.Number.Uint64())
				if run.full != nil {
					report.Steps = append(report.Steps, s.fullPlanSteps(ctx, run, header.Number.Uint64())...)
				}
			}
		}
		return err
	}
	// After verification, which waits for the rollups node to catch up.
	err = s.liveVerify(ctx, run, head)
	s.recordEpochs(ctx, run, head)
	if err != nil {
		return err
	}
	if run.full != nil {
		report.Steps = append(report.Steps, s.fullPlanSteps(ctx, run, head)...)
	}
	return nil
}

// recordEpochs fills the epochs table of the report; a read error becomes a
// failed step.
func (s *session) recordEpochs(ctx context.Context, run *liveRun, head uint64) {
	epochs, err := s.epochTable(ctx, run, head, run.epochExpectations())
	if err != nil {
		run.report.Steps = append(run.report.Steps, continuationStep{Name: "epochs table", Status: checkFail, Detail: err.Error()})
		return
	}
	run.report.Epochs = epochs
}

// setupLive starts the chain and the miner, deploys the application, and
// starts the node or nodes that the order starts first. The returned run is
// non-nil once the miner runs, even with an error; stop it.
//
//nolint:gocyclo,funlen // see runLive
func (s *session) setupLive(ctx context.Context, opts *liveOptions, report *liveReport) (*liveRun, error) {
	s.runDir = opts.runDir
	report.RunDir = s.runDir
	if _, err := os.Stat(s.runDir); err == nil {
		return nil, fmt.Errorf("run directory %s exists", s.runDir)
	}
	if err := os.MkdirAll(filepath.Join(s.runDir, "snapshots"), 0o755); err != nil { //nolint:mnd
		return nil, err
	}
	for _, port := range []int{opts.anvilPort, opts.nodePortBase, opts.nodePortBase + jsonrpcPortOffset,
		opts.nodePortBase + inspectPortOffset} {
		if err := requirePortFree(port); err != nil {
			return nil, withExitCode(exitPrerequisites, err)
		}
	}
	if err := resolveBinaries(&s.opts.nodeBin, &s.opts.cliBin); err != nil {
		return nil, err
	}

	s.manifest = &caseManifest{Case: "live_" + opts.scenario + "_" + strings.ReplaceAll(opts.order, "-", "_"),
		Chain: caseChain{SlotsInAnEpoch: 1}}
	s.manifest.Provenance.Program = opts.program
	if err := readDeployments(opts.daveRoot, s.manifest); err != nil {
		return nil, withExitCode(exitPrerequisites, err)
	}
	step("live", "starting Anvil on port %d from %s", opts.anvilPort, displayPath(opts.anvilState))
	if err := s.startAnvil(ctx, opts.anvilState, false); err != nil {
		return nil, err
	}
	chainID, err := s.chain.chainID(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireTestChain(chainID); err != nil {
		return nil, err
	}
	s.manifest.Chain.ChainID = chainID

	// This command owns the chain: it mines until the scenario freezes it.
	minerCtx, cancelMiner := context.WithCancel(ctx)
	var miner sync.WaitGroup
	miner.Add(1)
	go func() {
		defer miner.Done()
		s.mine(minerCtx)
	}()
	var once sync.Once
	run := &liveRun{opts: opts, report: report, chainID: chainID, stopMiner: func() {
		once.Do(func() {
			cancelMiner()
			miner.Wait()
		})
	}}

	if err := s.createDatabase(ctx); err != nil {
		return run, err
	}
	report.Database = s.dbName
	if err := s.buildEnvironment(); err != nil {
		return run, err
	}
	s.env = mergeEnvironment(s.env, nil, livePolling)
	rollupsTemplate := filepath.Join(s.runDir, "template-rollups")
	slingTemplate := filepath.Join(s.runDir, "template-sling")
	for _, dst := range []string{rollupsTemplate, slingTemplate} {
		if err := copyTree(opts.template, dst); err != nil {
			return run, fmt.Errorf("copying the template: %w", err)
		}
	}
	if run.rollupsSigner, err = s.rollupsPrtSigner(); err != nil {
		return run, err
	}
	if run.sling, err = deriveTestAccount(testMnemonic, uint32(opts.slingIndex)); err != nil { //nolint:gosec // small index
		return run, err
	}
	if run.sling.Address == run.rollupsSigner {
		return run, withExitCode(exitUsage, errors.New("the Sling node's signer and the rollups PRT signer must differ"))
	}
	if run.sender, err = deriveTestAccount(testMnemonic, defaultInputSenderIx); err != nil {
		return run, err
	}
	report.RollupsSigner, report.SlingSigner = run.rollupsSigner, run.sling.Address

	// Deploy and register with the rollups CLI.
	s.appName = "interop-live-" + opts.scenario + "-" + opts.order
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return run, err
	}
	deployArgs := []string{"deploy", "application", s.appName, rollupsTemplate, "--prt", "--salt", hex.EncodeToString(salt),
		"--claim-staging-period", strconv.FormatUint(opts.stagingPeriod, 10), "--json"}
	if opts.slingSentry {
		deployArgs = append(deployArgs, "--sentries", run.sling.Address.Hex())
	}
	out, err := s.runCLI(ctx, deployArgs...)
	if err != nil {
		return run, fmt.Errorf("deploying the application: %w", err)
	}
	var deployed struct {
		Application common.Address `json:"iapplication_address"`
		Consensus   common.Address `json:"iconsensus_address"`
	}
	if err := json.Unmarshal([]byte(out), &deployed); err != nil {
		return run, fmt.Errorf("parsing the deploy output %q: %w", out, err)
	}
	if deployed.Application == (common.Address{}) || deployed.Consensus == (common.Address{}) {
		return run, fmt.Errorf("the deploy output %q has no application or consensus address", out)
	}
	run.app, run.consensus = deployed.Application, deployed.Consensus
	s.manifest.Application.Address, s.manifest.Application.Consensus = deployed.Application, deployed.Consensus
	report.Application = deployed.Application
	step("live", "deployed %s (%s) with the rollups CLI; Sling signer %s is the sentry: %t",
		deployed.Application, opts.program, run.sling.Address, opts.slingSentry)

	keyFile := filepath.Join(s.runDir, "sling-signer.key")
	if err := os.WriteFile(keyFile, []byte("0x"+hex.EncodeToString(run.sling.key)), 0o600); err != nil { //nolint:mnd
		return run, err
	}
	slingState := filepath.Join(s.runDir, "sling-state")
	if err := os.MkdirAll(slingState, 0o700); err != nil { //nolint:mnd
		return run, err
	}
	run.startSling = func() error {
		var err error
		s.sling, err = startChild("sling node", filepath.Join(s.runDir, "sling.log"), s.runDir, slingEnvironment(), opts.slingBin,
			slingArgs(deployed.Application, slingTemplate, s.rpcURL, chainID, slingState, liveSlingSleepSeconds, keyFile)...)
		if err == nil {
			step("live", "Sling node started (log %s)", displayPath(filepath.Join(s.runDir, "sling.log")))
		}
		return err
	}
	run.startRollups = func() error {
		if err := s.startNode(ctx); err != nil {
			return err
		}
		step("live", "rollups node started (log %s)", displayPath(filepath.Join(s.runDir, "node.log")))
		return nil
	}
	switch opts.order {
	case orderConcurrent:
		if err := run.startRollups(); err != nil {
			return run, err
		}
		err = run.startSling()
	case orderRollupsFirst:
		err = run.startRollups()
	case orderSlingFirst:
		err = run.startSling()
	}
	return run, err
}

// liveSmoke sends plain inputs into one epoch, waits until that epoch is
// accepted, freezes the chain, and returns its head.
func (s *session) liveSmoke(ctx context.Context, run *liveRun) (uint64, error) {
	opts, report := run.opts, run.report
	for i := range opts.inputs {
		tx, err := s.sendInput(ctx, run, fmt.Sprintf("interop input %d", i))
		if err != nil {
			return 0, err
		}
		report.InputTxs = append(report.InputTxs, tx)
	}
	step("live", "sent %d inputs from %s", opts.inputs, run.sender.Address)

	liveCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	var inputEpoch sealedEpoch
	err := s.waitForLive(liveCtx, "the input epoch to be sealed", func(_ uint64, sealed map[uint64]sealedEpoch) (bool, error) {
		for _, epoch := range sealed {
			if epoch.Upper >= uint64(opts.inputs) && epoch.Lower < uint64(opts.inputs) { //nolint:gosec // checked positive
				inputEpoch = epoch
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return 0, err
	}
	report.InputEpoch = inputEpoch.Index
	step("live", "input epoch %d is sealed (tournament %s)", inputEpoch.Index, inputEpoch.Tournament)
	if err := s.startSecondNode(liveCtx, run, inputEpoch.Tournament); err != nil {
		return 0, err
	}
	err = s.waitForLive(liveCtx, "acceptance of the input epoch", func(_ uint64, sealed map[uint64]sealedEpoch) (bool, error) {
		_, accepted := sealed[inputEpoch.Index+1]
		return accepted, nil
	})
	if err != nil {
		return 0, err
	}
	head, err := s.freezeLive(ctx, run)
	if err != nil {
		return 0, err
	}
	step("live", "input epoch %d is accepted; chain frozen at block %d", inputEpoch.Index, head)
	return head, s.recordJoinOrder(ctx, run, inputEpoch.Tournament, head)
}

// sendInput adds one input from the input sender and waits for its receipt.
func (s *session) sendInput(ctx context.Context, run *liveRun, payload string) (common.Hash, error) {
	tx, err := s.chain.addInput(ctx, s.manifest.Deployments.InputBox, run.app, run.sender, []byte(payload))
	if err != nil {
		return common.Hash{}, err
	}
	if _, err := s.chain.waitReceipt(ctx, tx, receiptTimeout); err != nil {
		return common.Hash{}, err
	}
	return tx, nil
}

// startSecondNode starts the node that the order holds back, once the first
// one joined the tournament. With concurrent, both run already.
func (s *session) startSecondNode(ctx context.Context, run *liveRun, tournament common.Address) error {
	if run.opts.order == orderConcurrent {
		return nil
	}
	first, firstName, startSecond := run.rollupsSigner, "the rollups node", run.startSling
	if run.opts.order == orderSlingFirst {
		first, firstName, startSecond = run.sling.Address, "the Sling node", run.startRollups
	}
	err := s.waitForLive(ctx, firstName+" to join", func(head uint64, _ map[uint64]sealedEpoch) (bool, error) {
		joins, err := s.chain.joins(ctx, tournament, head)
		if err != nil {
			return false, err
		}
		for _, join := range joins {
			if join.Submitter == first {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	return startSecond()
}

// freezeLive stops the miner and returns the head, which stays fixed.
func (s *session) freezeLive(ctx context.Context, run *liveRun) (uint64, error) {
	run.stopMiner()
	header, err := s.chain.head(ctx)
	if err != nil {
		return 0, err
	}
	return header.Number.Uint64(), nil
}

// recordJoinOrder reports the joins of the first epoch with inputs and checks
// them against the order.
func (s *session) recordJoinOrder(ctx context.Context, run *liveRun, tournament common.Address, head uint64) error {
	joins, err := s.chain.joins(ctx, tournament, head)
	if err != nil {
		return err
	}
	sort.Slice(joins, func(i, j int) bool { return joins[i].Block < joins[j].Block })
	for _, join := range joins {
		run.report.Joins = append(run.report.Joins, liveJoin{Party: run.party(join.Submitter), Block: join.Block, Tx: join.Tx,
			Commit: join.Commitment})
	}
	run.report.OrderChecked = checkJoinOrder(run.opts.order, run.report.Joins)
	return nil
}

// party names a signer of the run.
func (run *liveRun) party(signer common.Address) string {
	switch signer {
	case run.rollupsSigner:
		return partyRollups
	case run.sling.Address:
		return partySling
	}
	if run.full != nil && run.full.fake != nil && signer == run.full.fake.Signer {
		return partyFake
	}
	return signer.Hex()
}

// liveVerify compares the rollups node with the frozen chain and the Sling
// node's evidence.
func (s *session) liveVerify(ctx context.Context, run *liveRun, head uint64) error {
	opts, report := run.opts, run.report
	requireAgreement := opts.scenario != scenarioSmoke || opts.program == programEcho
	switch opts.requireAgreement {
	case flagTrue:
		requireAgreement = true
	case flagFalse:
		requireAgreement = false
	}
	var claims []epochClaim
	if slingLog := filepath.Join(s.runDir, "sling.log"); fileExists(slingLog) {
		var err error
		if claims, err = parseSlingClaimsFile(slingLog); err != nil {
			return err
		}
	}
	target := &verifyTarget{chain: s.chain, api: s.api, app: s.appName, appAddress: run.app,
		consensus: run.consensus, inputBox: s.manifest.Deployments.InputBox, claims: claimsByEpoch(claims),
		expectedStatus: model.ApplicationStatus_OK, requireAgreement: requireAgreement, atBlock: head,
		slingSigner: run.sling.Address, rollupsSigner: run.rollupsSigner, watch: s.checkParticipants}
	step("verify", "comparing the rollups node with the chain and the Sling evidence")
	var err error
	report.Verify, err = verifyUntil(ctx, target, opts.wait)
	if err == nil {
		step("verify", "%s (%s)", passFail(report.Verify.Passed), coverageText(report.Verify.Coverage))
	}
	return err
}

// waitForLive polls the sealed epochs at the head until cond holds. It fails
// when either node exits (see waitFor).
func (s *session) waitForLive(ctx context.Context, what string, cond func(uint64, map[uint64]sealedEpoch) (bool, error)) error {
	return s.waitFor(ctx, what, func() (bool, error) {
		header, err := s.chain.head(ctx)
		if err != nil {
			return false, err
		}
		head := header.Number.Uint64()
		sealed, err := s.chain.sealedEpochs(ctx, s.manifest.Application.Consensus, head)
		if err != nil {
			return false, err
		}
		return cond(head, sealedByIndex(sealed))
	})
}

// checkJoinOrder checks that the first joiner of the input epoch matches the
// requested order. Only one participant can join a given commitment.
func checkJoinOrder(order string, joins []liveJoin) checkResult {
	result := checkResult{Name: "join_order", Status: checkPass}
	if len(joins) == 0 {
		result.Status, result.Detail = checkFail, "nobody joined the input epoch"
		return result
	}
	result.Detail = "first join by " + joins[0].Party
	switch order {
	case orderRollupsFirst:
		if joins[0].Party != partyRollups {
			result.Status = checkFail
		}
	case orderSlingFirst:
		if joins[0].Party != partySling {
			result.Status = checkFail
		}
	default:
		result.Status = checkNotTested
		result.Detail += " (concurrent start; order not controlled)"
	}
	if len(joins) > 1 {
		result.Items = append(result.Items, fmt.Sprintf("%d joins; a second join means different commitments", len(joins)))
		result.Status = checkFail
	}
	return result
}

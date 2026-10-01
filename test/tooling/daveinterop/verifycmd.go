// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/model"
)

type verifyOptions struct {
	rpc              string
	api              string
	app              string
	inputBox         string
	caseDir          string
	slingLog         string
	wait             time.Duration
	expectedStatus   string
	requireAgreement bool
	atBlock          uint64
	refSigner        string
}

func newVerifyCommand() *cobra.Command {
	opts := &verifyOptions{}
	cmd := &cobra.Command{
		Use:   "verify --app NAME_OR_ADDRESS",
		Short: "Compare a running rollups node with the chain and the Sling node's evidence",
		Long: `verify reads the chain with the contract bindings and the rollups node through its
JSON-RPC API, waits for positive completion predicates (cursors, inputs,
epochs, tournaments), and then compares:

  - application status, input count, and input statuses;
  - every sealed epoch: bounds, tournament, and computed commitment;
  - every staged epoch: final machine state and outputs root;
  - every accepted epoch: the node records CLAIM_ACCEPTED;
  - commitments and final states against the Sling node: its stage-log
    claims (--sling-log or --case), and its own joins and sentry claims on chain
    (--sling-signer);
  - commitments against finished root winners (divergence check only);
  - the full tournament tree, in both directions;
  - computation agreement: at least one epoch that matches the Sling node's
    evidence and changed state (--require-agreement makes "not tested" fail).

It works on a live chain too (for example, both nodes on our devnet).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVerify(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.rpc, "rpc", envOr("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://127.0.0.1:8545"), "chain JSON-RPC endpoint")
	flags.StringVar(&opts.api, "api", envOr("CARTESI_JSONRPC_API_URL", "http://127.0.0.1:10011/rpc"), "rollups node JSON-RPC API")
	flags.StringVar(&opts.app, "app", "", "application name or address in the rollups node (required)")
	flags.StringVar(&opts.inputBox, "input-box", envOr("CARTESI_CONTRACTS_INPUT_BOX_ADDRESS", defaultInputBox), "InputBox address")
	flags.StringVar(&opts.caseDir, "case", "", "case directory whose manifest supplies the Sling stage claims")
	flags.StringVar(&opts.slingLog, "sling-log", "", "Sling node log with stage claims")
	flags.DurationVar(&opts.wait, "wait", 0, "wait up to this long for the completion predicates")
	flags.StringVar(&opts.expectedStatus, "expect-status", string(model.ApplicationStatus_OK), "expected application status")
	flags.BoolVar(&opts.requireAgreement, "require-agreement", false, "fail when no compared epoch changed state")
	flags.Uint64Var(&opts.atBlock, "at-block", 0, "pin chain reads to this block (default: head)")
	flags.StringVar(&opts.refSigner, "sling-signer", "",
		`Sling node signer whose joins and sentry claims are evidence (default: test account 7 on chain 31337; "none" disables)`)
	_ = cmd.MarkFlagRequired("app")
	return cmd
}

// rollupsSignerFromEnv derives the rollups node's PRT signer from the node
// settings, when they use a mnemonic on the local test chain. It returns the
// zero address when the signer cannot be known.
func rollupsSignerFromEnv(chainID uint64) common.Address {
	s := &session{opts: &replayOptions{}, env: os.Environ()}
	if chainID == anvilChainID {
		s.env = mergeEnvironment(s.env, map[string]string{envPrtMnemonic: testMnemonic}, nil)
	}
	signer, err := s.rollupsPrtSigner()
	if err != nil {
		return common.Address{}
	}
	return signer
}

func runVerify(cmd *cobra.Command, opts *verifyOptions) error {
	ctx := cmd.Context()
	if opts.caseDir != "" && opts.slingLog != "" {
		return withExitCode(exitUsage, errors.New("use --case or --sling-log, not both"))
	}
	inputBox, err := parseAddress("--input-box", opts.inputBox)
	if err != nil {
		return withExitCode(exitUsage, err)
	}
	var claims []epochClaim
	switch {
	case opts.caseDir != "":
		m, err := readManifest(opts.caseDir)
		if err != nil {
			return withExitCode(exitPrerequisites, err)
		}
		claims = m.Reference.Claims
	case opts.slingLog != "":
		if claims, err = parseSlingClaimsFile(opts.slingLog); err != nil {
			return withExitCode(exitPrerequisites, err)
		}
	}

	c, err := dialChain(ctx, opts.rpc)
	if err != nil {
		return withExitCode(exitPrerequisites, err)
	}
	defer c.close()
	chainID, err := c.chainID(ctx)
	if err != nil {
		return withExitCode(exitPrerequisites, err)
	}
	var slingSigner common.Address
	switch opts.refSigner {
	case "":
		slingSigner = defaultSlingSigner(chainID)
	case textNone:
	default:
		if slingSigner, err = parseAddress("--sling-signer", opts.refSigner); err != nil {
			return withExitCode(exitUsage, err)
		}
	}
	api := newNodeAPI(opts.api)
	app, err := api.application(ctx, opts.app)
	if err != nil {
		return withExitCode(exitPrerequisites, fmt.Errorf("reading the application from %s: %w", opts.api, err))
	}
	if app.ConsensusType != model.Consensus_PRT {
		return withExitCode(exitUsage, fmt.Errorf("%s uses %s consensus; verify supports PRT (DaveConsensus) applications", opts.app,
			app.ConsensusType))
	}
	target := &verifyTarget{
		chain: c, api: api, app: opts.app, appAddress: app.IApplicationAddress, consensus: app.IConsensusAddress,
		inputBox: inputBox, claims: claimsByEpoch(claims), expectedStatus: model.ApplicationStatus(opts.expectedStatus),
		requireAgreement: opts.requireAgreement, atBlock: opts.atBlock, slingSigner: slingSigner,
		rollupsSigner: rollupsSignerFromEnv(chainID),
	}
	report, err := verifyUntil(ctx, target, opts.wait)
	if err != nil {
		return withExitCode(exitFailure, err)
	}
	if err := emit(cmd.OutOrStdout(), report); err != nil {
		return err
	}
	if !report.Passed {
		return withExitCode(exitFailure, errors.New("verification failed"))
	}
	return nil
}

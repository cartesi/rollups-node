// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

type runSlingOptions struct {
	slingBin      string
	app           string
	template      string
	rpc           string
	chainID       uint64
	stateDir      string
	keyFile       string
	mnemonicIndex int
	sleepSeconds  uint64
	resume        bool
}

type runSlingResult struct {
	Command     []string `json:"command"`
	SlingBinary string   `json:"sling_binary"`
	SHA256      string   `json:"sha256"`
	Signer      string   `json:"signer,omitempty"`
	StateDir    string   `json:"state_dir"`
	ExitCode    int      `json:"exit_code"`
}

func newRunSlingCommand() *cobra.Command {
	opts := &runSlingOptions{}
	cmd := &cobra.Command{
		Use:   "run-sling --app ADDRESS --template DIR --state-dir DIR (--key-file FILE | --mnemonic-index N)",
		Short: "Attach the honest Sling node to an existing chain (foreground)",
		Long: `run-sling starts the Sling node (--sling-bin) for one application on
an existing chain. It does not deploy, reset, or start a chain, and it never
builds it. It runs in the foreground; stop it with Ctrl-C. Its exit code is
passed through.

The signer is a key file, or an index of the public test mnemonic (chain 31337
only; the derived key is written to <state-dir>/signer.key with mode 0600). The
key never appears in the command line. Use a signer different from the
rollups node's PRT signer (index 6 by default); index 7 matches the Dave harness.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSling(cmd.Context(), cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.slingBin, "sling-bin", defaultSlingBin(),
		"Sling node executable (default: $DAVE_NODE_BIN, or its build in $DAVE_ROOT)")
	flags.StringVar(&opts.app, "app", "", "application address (required)")
	flags.StringVar(&opts.template, "template", "", "application template machine directory (required)")
	flags.StringVar(&opts.rpc, "rpc", envOr("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://127.0.0.1:8545"), "JSON-RPC endpoint")
	flags.Uint64Var(&opts.chainID, "chain-id", 0, "chain id (default: read from --rpc)")
	flags.StringVar(&opts.stateDir, "state-dir", "", "Sling state directory, one per participant (required)")
	flags.StringVar(&opts.keyFile, "key-file", "", "file with the signer private key")
	flags.IntVar(&opts.mnemonicIndex, "mnemonic-index", -1, "derive the signer from the test mnemonic (chain 31337 only)")
	flags.Uint64Var(&opts.sleepSeconds, "sleep-duration-seconds", 1, "Sling node polling interval")
	flags.BoolVar(&opts.resume, "resume", false, "reuse a non-empty state directory")
	_ = cmd.MarkFlagRequired("app")
	_ = cmd.MarkFlagRequired("template")
	_ = cmd.MarkFlagRequired("state-dir")
	return cmd
}

// slingArgs is the Sling node command line for one application. The key is
// read from a file, so it never appears on the command line.
func slingArgs(app common.Address, template, rpcURL string, chainID uint64, stateDir string, sleepSeconds uint64,
	keyFile string,
) []string {
	return []string{
		"--app-address", app.Hex(),
		"--machine-path", template,
		"--web3-rpc-url", rpcURL,
		"--web3-chain-id", strconv.FormatUint(chainID, 10),
		"--state-dir", stateDir,
		"--sleep-duration-seconds", strconv.FormatUint(sleepSeconds, 10),
		"pk", "--web3-private-key-file", keyFile,
	}
}

// slingEnvironment is ours plus RUST_LOG=info when unset: without it the
// Sling node logs nothing, including the stage claims that verify reads.
func slingEnvironment() []string {
	return mergeEnvironment(os.Environ(), map[string]string{"RUST_LOG": "info"}, nil)
}

func runSling(ctx context.Context, cmd *cobra.Command, opts *runSlingOptions) error {
	if opts.slingBin == "" {
		return withExitCode(exitPrerequisites, errors.New("set --sling-bin or DAVE_NODE_BIN"))
	}
	if (opts.keyFile == "") == (opts.mnemonicIndex < 0) {
		return withExitCode(exitUsage, errors.New("use exactly one of --key-file and --mnemonic-index"))
	}
	app, err := parseAddress("--app", opts.app)
	if err != nil {
		return withExitCode(exitUsage, err)
	}
	sum, _, err := sha256File(opts.slingBin)
	if err != nil {
		return withExitCode(exitPrerequisites, fmt.Errorf("reading the Sling executable: %w", err))
	}
	if info, err := os.Stat(opts.template); err != nil || !info.IsDir() {
		return withExitCode(exitPrerequisites, fmt.Errorf("template %s is not a directory", opts.template))
	}
	stateDir, err := filepath.Abs(opts.stateDir)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(stateDir); err == nil && len(entries) > 0 && !opts.resume {
		return withExitCode(exitUsage, fmt.Errorf("state directory %s is not empty; use --resume to reuse it", stateDir))
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil { //nolint:mnd
		return err
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
	if opts.chainID != 0 && opts.chainID != chainID {
		return withExitCode(exitUsage, fmt.Errorf("--chain-id %d, but %s serves chain %d", opts.chainID, opts.rpc, chainID))
	}
	if code, err := c.eth.CodeAt(ctx, app, nil); err != nil || len(code) == 0 {
		return withExitCode(exitPrerequisites, fmt.Errorf("no contract at %s on %s", app, opts.rpc))
	}

	result := &runSlingResult{SlingBinary: opts.slingBin, SHA256: sum, StateDir: stateDir}
	keyFile := opts.keyFile
	if opts.mnemonicIndex >= 0 {
		if err := requireTestChain(chainID); err != nil {
			return withExitCode(exitUsage, err)
		}
		account, err := deriveTestAccount(testMnemonic, uint32(opts.mnemonicIndex)) //nolint:gosec // checked non-negative
		if err != nil {
			return err
		}
		keyFile = filepath.Join(stateDir, "signer.key")
		if err := os.WriteFile(keyFile, []byte("0x"+hex.EncodeToString(account.key)), 0o600); err != nil { //nolint:mnd
			return err
		}
		result.Signer = account.Address.Hex()
	}

	args := slingArgs(app, opts.template, opts.rpc, chainID, stateDir, opts.sleepSeconds, keyFile)
	result.Command = append([]string{opts.slingBin}, args...)
	step("run-sling", "starting the Sling node for %s on %s (state %s, signer %s)", app, opts.rpc, stateDir, result.Signer)
	code, err := runForeground("", slingEnvironment(), opts.slingBin, args...)
	result.ExitCode = code
	if printErr := emit(cmd.OutOrStdout(), result); printErr != nil {
		return printErr
	}
	if err != nil {
		return withExitCode(exitFailure, err)
	}
	if code != 0 {
		return withExitCode(code, fmt.Errorf("the Sling node exited with code %d", code))
	}
	return nil
}

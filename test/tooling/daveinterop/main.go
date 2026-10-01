// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Command daveinterop runs interoperability checks between the rollups node
// (this repository) and the Sling node, the reference PRT node of the Dave
// repository (cartesi-rollups-prt-node).
//
// Every subcommand writes its result to stdout (text, or JSON with --json) and
// a short progress log to stderr.
// See README.md in this directory for the walkthrough.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// Exit codes. A child process exit code is passed through unchanged by run-sling.
const (
	exitFailure       = 1 // a check failed, or an operation failed
	exitUsage         = 2 // invalid arguments
	exitPrerequisites = 3 // a required tool, file, or service is missing
)

// exitError carries a specific process exit code to main.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitError{code: code, err: err}
}

var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

func main() {
	root := &cobra.Command{
		Use:   "daveinterop",
		Short: "Interoperability checks between the rollups node and the Sling node",
		Long: `daveinterop checks the rollups node (this repository) against the Sling node
(the reference PRT node of the Dave repository). It captures Dave harness
chains, replays them into a fresh rollups node, and verifies the rollups node
against the chain and the Sling node's evidence.
It can then continue a replayed chain with the rollups node alone.
It also runs both nodes together on a live chain.

Run "eval $(make env)" first. See test/tooling/daveinterop/README.md.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			configureLogger()
		},
	}
	// An error before a command starts is a usage error; after, it is a
	// failure unless the command says otherwise.
	started := false
	root.PersistentFlags().BoolVar(&outputJSON, "json", false, "print the result as JSON (for tools)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "debug logs, and stream the Dave harness output")
	root.AddCommand(
		newCheckCommand(),
		newCaptureCommand(),
		newReplayCommand(),
		newVerifyCommand(),
		newRunSlingCommand(),
		newLiveCommand(),
		newSuiteCommand(),
	)
	for _, cmd := range root.Commands() {
		run := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			started = true
			return run(cmd, args)
		}
	}

	err := root.Execute()
	if err == nil {
		return
	}
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		fmt.Fprintln(os.Stderr, "error:", exitErr.err)
		os.Exit(exitErr.code)
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	if !started {
		os.Exit(exitUsage)
	}
	os.Exit(exitFailure)
}

// writeJSON writes the structured result of a subcommand.
func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

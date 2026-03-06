// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

const cliBinary = "cartesi-rollups-cli"

// cliCommandTimeout is the maximum time a single CLI command may run before
// being killed. This prevents a hanging command from consuming the entire
// suite timeout.
const cliCommandTimeout = 60 * time.Second

// uniqueSalt generates a random 32-byte hex salt for CREATE2 deployments,
// ensuring tests are idempotent when re-run against the same blockchain state.
func uniqueSalt() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("generate salt: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// uniqueAppName generates a unique application name by appending a random
// 8-char hex suffix. This avoids DB unique-constraint violations when tests
// are re-run against the same postgres without a schema reset.
func uniqueAppName(prefix string) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("generate app name suffix: %v", err))
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b[:]))
}

// CLI execution helpers.

func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// cliError wraps a CLI execution failure, preserving the exit code for
// error discrimination in poll helpers.
type cliError struct {
	Args     []string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *cliError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("cli %v failed (exit %d): %s", e.Args, e.ExitCode, e.Stderr)
	}
	return fmt.Sprintf("cli %v failed: %v", e.Args, e.Err)
}

func (e *cliError) Unwrap() error { return e.Err }

// isCLIExitError returns true if the error is a CLI command that exited
// with a non-zero code (as opposed to a context cancellation, JSON parse
// failure, or other structural error). Poll helpers use this to distinguish
// "not found yet" from genuine failures.
func isCLIExitError(err error) bool {
	var ce *cliError
	return errors.As(err, &ce)
}

// runCLI executes the CLI binary with the given arguments and returns stdout.
// Each command is given an independent timeout (cliCommandTimeout) to prevent
// a single hanging call from consuming the entire suite timeout.
func runCLI(ctx context.Context, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, cliCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, cliBinary, args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", &cliError{
				Args:     args,
				ExitCode: exitErr.ExitCode(),
				Stderr:   string(exitErr.Stderr),
				Err:      err,
			}
		}
		return "", fmt.Errorf("cli %v failed: %w", args, err)
	}
	return string(out), nil
}

// deployApplication deploys a self-hosted application using the CLI and returns
// the application address from the JSON output.
// Extra CLI flags (e.g., "--salt", value, "--prt") can be appended via extraArgs.
func deployApplication(ctx context.Context, appName, dappPath string, extraArgs ...string) (string, error) {
	args := []string{"deploy", "application", appName, dappPath, "--json"}
	args = append(args, extraArgs...)
	out, err := runCLI(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("deploy: %w", err)
	}

	var app struct {
		IApplicationAddress string `json:"iapplication_address"`
		IConsensusAddress   string `json:"iconsensus_address"`
	}
	if err := json.Unmarshal([]byte(out), &app); err != nil {
		return "", fmt.Errorf("parse deploy output: %w", err)
	}
	return app.IApplicationAddress, nil
}

// sendInput sends a payload to the application and returns (inputIndex, blockNumber).
func sendInput(ctx context.Context, appName string, payload string) (uint64, uint64, error) {
	out, err := runCLI(ctx, "send", appName, payload, "--yes", "--json")
	if err != nil {
		return 0, 0, fmt.Errorf("send: %w", err)
	}
	var result cli.SendResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return 0, 0, fmt.Errorf("parse send output: %w", err)
	}
	inputIndex, err := hexutil.DecodeUint64(result.InputIndex)
	if err != nil {
		return 0, 0, fmt.Errorf("parse input_index %q: %w", result.InputIndex, err)
	}
	blockNumber, err := hexutil.DecodeUint64(result.BlockNumber)
	if err != nil {
		return 0, 0, fmt.Errorf("parse block_number %q: %w", result.BlockNumber, err)
	}
	return inputIndex, blockNumber, nil
}

// readOutputs lists all outputs for the application.
func readOutputs(ctx context.Context, appName string) (*api.ListResponse[api.DecodedOutput], error) {
	out, err := runCLI(ctx, "read", "outputs", appName)
	if err != nil {
		return nil, err
	}
	var resp api.ListResponse[api.DecodedOutput]
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse outputs: %w", err)
	}
	return &resp, nil
}

// readOutput reads a single output by index.
func readOutput(ctx context.Context, appName string, index uint64) (*api.DecodedOutput, error) {
	out, err := runCLI(ctx, "read", "outputs", appName, strconv.FormatUint(index, 10))
	if err != nil {
		return nil, err
	}
	var resp api.SingleResponse[api.DecodedOutput]
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse output: %w", err)
	}
	return &resp.Data, nil
}

// readReports lists all reports for the application.
func readReports(ctx context.Context, appName string) (*api.ListResponse[model.Report], error) {
	out, err := runCLI(ctx, "read", "reports", appName)
	if err != nil {
		return nil, err
	}
	var resp api.ListResponse[model.Report]
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse reports: %w", err)
	}
	return &resp, nil
}

// readEpoch reads a single epoch by index.
func readEpoch(ctx context.Context, appName string, epochIndex uint64) (*model.Epoch, error) {
	out, err := runCLI(ctx, "read", "epochs", appName, strconv.FormatUint(epochIndex, 10))
	if err != nil {
		return nil, err
	}
	var resp api.SingleResponse[model.Epoch]
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse epoch: %w", err)
	}
	return &resp.Data, nil
}

// readInput reads a single input by index.
func readInput(ctx context.Context, appName string, inputIndex uint64) (*model.Input, error) {
	out, err := runCLI(ctx, "read", "inputs", appName, strconv.FormatUint(inputIndex, 10))
	if err != nil {
		return nil, err
	}
	var resp api.SingleResponse[model.Input]
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	return &resp.Data, nil
}

// executeOutput executes a voucher on L1 via the CLI.
func executeOutput(ctx context.Context, appName string, index uint64) (string, error) {
	out, err := runCLI(ctx, "execute", appName, strconv.FormatUint(index, 10), "--yes", "--json")
	if err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	var result cli.ExecuteResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return "", fmt.Errorf("parse execute output: %w", err)
	}
	return result.TransactionHash, nil
}

// validateOutput validates a notice on L1 via the CLI.
func validateOutput(ctx context.Context, appName string, index uint64) error {
	_, err := runCLI(ctx, "validate", appName, strconv.FormatUint(index, 10))
	return err
}

// readTournaments lists all tournaments for the application.
func readTournaments(
	ctx context.Context,
	appName string,
) (*api.ListResponse[model.Tournament], error) {
	out, err := runCLI(ctx, "read", "tournaments", appName)
	if err != nil {
		return nil, err
	}
	var resp api.ListResponse[model.Tournament]
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse tournaments: %w", err)
	}
	return &resp, nil
}

// readCommitments lists all commitments for the application.
func readCommitments(
	ctx context.Context,
	appName string,
) (*api.ListResponse[model.Commitment], error) {
	out, err := runCLI(ctx, "read", "commitments", appName)
	if err != nil {
		return nil, err
	}
	var resp api.ListResponse[model.Commitment]
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse commitments: %w", err)
	}
	return &resp, nil
}

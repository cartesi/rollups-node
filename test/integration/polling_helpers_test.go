// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
)

// pollUntil polls fn at the given interval until it returns true or the context
// is cancelled. Returns an error if the context times out.
func pollUntil(ctx context.Context, interval time.Duration, fn func() (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("poll timed out: %w", ctx.Err())
		default:
		}

		done, err := fn()
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("poll timed out: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// waitForInputProcessed polls until the given input has been processed
// (status != NONE) using the CLI.
//
// Error discrimination: CLI exit errors (the input doesn't exist yet) are
// retried. Structural errors (JSON parse failure, context cancellation) fail
// immediately.
func waitForInputProcessed(
	ctx context.Context,
	t testing.TB,
	appName string,
	inputIndex uint64,
) (*model.Input, error) {
	var lastErr error
	var result *model.Input
	err := pollUntil(ctx, 2*time.Second, func() (bool, error) {
		input, err := readInput(ctx, appName, inputIndex)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				t.Logf("poll input %d: %v (retrying)", inputIndex, err)
				return false, nil // Input may not exist yet; keep polling.
			}
			return false, fmt.Errorf("poll input %d: %w", inputIndex, err)
		}
		if input.Status != model.InputCompletionStatus_None {
			result = input
			return true, nil
		}
		return false, nil
	})
	if err != nil && lastErr != nil {
		return nil, fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	return result, err
}

// waitForEpochStatus polls until the epoch at the given index reaches the
// desired status using the CLI.
//
// Error discrimination: CLI exit errors (the epoch doesn't exist yet) are
// retried. Structural errors fail immediately.
func waitForEpochStatus(
	ctx context.Context,
	t testing.TB,
	appName string,
	epochIndex uint64,
	status model.EpochStatus,
) (*model.Epoch, error) {
	var lastErr error
	var result *model.Epoch
	err := pollUntil(ctx, 3*time.Second, func() (bool, error) {
		epoch, err := readEpoch(ctx, appName, epochIndex)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				t.Logf("poll epoch %d: %v (retrying)", epochIndex, err)
				return false, nil // Epoch may not exist yet; keep polling.
			}
			return false, fmt.Errorf("poll epoch %d: %w", epochIndex, err)
		}
		if epoch.Status == status {
			result = epoch
			return true, nil
		}
		return false, nil
	})
	if err != nil && lastErr != nil {
		return nil, fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	return result, err
}

// waitForExecutionRecorded polls until the output's execution transaction hash
// is recorded in the database.
//
// Error discrimination: CLI exit errors are retried. Structural errors
// (JSON parse failure, context cancellation) fail immediately.
func waitForExecutionRecorded(
	ctx context.Context,
	t testing.TB,
	appName string,
	outputIdx uint64,
) error {
	var lastErr error
	err := pollUntil(ctx, 3*time.Second, func() (bool, error) {
		out, err := readOutput(ctx, appName, outputIdx)
		if err != nil {
			if isCLIExitError(err) {
				lastErr = err
				t.Logf("    poll execution tx hash for output %d: %v (retrying)", outputIdx, err)
				return false, nil
			}
			return false, fmt.Errorf("poll execution tx hash: %w", err)
		}
		return out.ExecutionTransactionHash != nil, nil
	})
	if err != nil && lastErr != nil {
		return fmt.Errorf("%w (last poll error: %v)", err, lastErr)
	}
	return err
}

// readOutput is used above and returns *api.DecodedOutput, which embeds
// *model.Output. The ExecutionTransactionHash field is *common.Hash — a nil
// check correctly detects whether the execution event has been recorded.

// timed logs the duration of a test phase. Usage:
//
//	defer timed(t, "deploy echo-dapp")()
func timed(t testing.TB, phase string) func() {
	t.Helper()
	start := time.Now()
	return func() {
		t.Logf("    [timing] %s took %s", phase, time.Since(start).Round(time.Millisecond))
	}
}

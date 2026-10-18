// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	passiveClaimSubmissionField = "ClaimSubmissionEnabled"
	passiveDefaultBlockField    = "DefaultBlock"
	passiveSignerField          = "ResolvedSignerAddress"

	observerCleanupExitEnv     = "CARTESI_TEST_PASSIVE_OBSERVER_CLEANUP_EXIT_AT"
	observerCleanupTestPattern = "^TestPassiveObserverCleanupPreservesRestoreOrder$"
)

// This test-only codec has no integration build tag. Its pure tests must run
// without TestMain starting a node or connecting to the development database.
func patchPassiveObserverConfig(raw []byte, overrides map[string]any) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode passive observer config: %w", err)
	}
	for key, value := range overrides {
		if _, exists := fields[key]; !exists {
			return nil, fmt.Errorf("passive observer config is missing field %s", key)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode passive observer config field %s: %w", key, err)
		}
		fields[key] = encoded
	}
	if string(fields[passiveClaimSubmissionField]) == "false" {
		if _, exists := fields[passiveSignerField]; !exists {
			return nil, fmt.Errorf("passive observer config is missing field %s", passiveSignerField)
		}
		encoded, err := json.Marshal(common.Address{})
		if err != nil {
			return nil, fmt.Errorf("encode observer-only signer: %w", err)
		}
		fields[passiveSignerField] = encoded
	}
	return json.Marshal(fields)
}

func registerPassiveObserverCleanup(t *testing.T, stop, restore, restart func()) {
	t.Helper()
	t.Cleanup(func() {
		restore()
		restart()
	})
	// Cleanup runs in reverse registration order. A fatal shutdown assertion
	// must not skip restoration, so shutdown needs its own cleanup callback.
	t.Cleanup(stop)
}

func TestPassiveObserverCleanupPreservesRestoreOrder(t *testing.T) {
	const (
		noFailure   = "none"
		setupStep   = "setup"
		stopStep    = "stop"
		restoreStep = "restore"
		restartStep = "restart"
		closeStep   = "close repository"
	)
	if exitAt := os.Getenv(observerCleanupExitEnv); exitAt != "" {
		require.Contains(t, []string{noFailure, setupStep, stopStep, restoreStep}, exitAt)
		var calls []string
		// Report the sequence after all cleanup callbacks, including after Fatal.
		t.Cleanup(func() { t.Logf("cleanup calls: %q", calls) })
		t.Cleanup(func() { calls = append(calls, closeStep) })
		step := func(name string) func() {
			return func() {
				calls = append(calls, name)
				if name == exitAt {
					t.Fatalf("injected cleanup failure at %s", name)
				}
			}
		}
		registerPassiveObserverCleanup(t, step(stopStep), step(restoreStep), step(restartStep))
		if exitAt == setupStep {
			// An initial stop can fail before the reader starts. Cleanup must
			// still restore the default fixture.
			t.Fatalf("injected cleanup failure at %s", setupStep)
		}
		return
	}

	executable, err := os.Executable()
	require.NoError(t, err)
	for _, exitAt := range []string{noFailure, setupStep, stopStep, restoreStep} {
		t.Run(exitAt, func(t *testing.T) {
			// Use a child test process so a real Fatal does not fail the parent.
			// TestMain bypasses node startup for this exact child test selection.
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run="+observerCleanupTestPattern, "-test.v")
			cmd.Env = append(os.Environ(), observerCleanupExitEnv+"="+exitAt)
			output, err := cmd.CombinedOutput()
			require.NoError(t, ctx.Err(), "cleanup child did not finish: %s", output)
			if exitAt == noFailure {
				require.NoError(t, err, "%s", output)
			} else {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr, "%s", output)
				require.Equal(t, 1, exitErr.ExitCode(), "%s", output)
				require.Contains(t, string(output), "injected cleanup failure at "+exitAt)
			}
			want := []string{stopStep, restoreStep, restartStep, closeStep}
			if exitAt == restoreStep {
				want = []string{stopStep, restoreStep, closeStep}
			}
			require.Contains(t, string(output), fmt.Sprintf("cleanup calls: %q\n", want))
			require.NotContains(t, string(output), "--- SKIP:")
		})
	}
}

func TestPassiveObserverConfigPreservesOriginalAndUnrelatedFields(t *testing.T) {
	raw := []byte(`{"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":true,` +
		`"ResolvedSignerAddress":"0x0000000000000000000000000000000000000123",` +
		`"ChainID":18446744073709551615,"Other":{"value":18446744073709551616}}`)
	original := bytes.Clone(raw)
	updated, err := patchPassiveObserverConfig(raw, map[string]any{passiveDefaultBlockField: "LATEST", passiveClaimSubmissionField: false})
	require.NoError(t, err)
	require.Equal(t, original, raw, "cleanup must retain the original payload unchanged")
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(updated, &fields))
	require.Equal(t, `"LATEST"`, string(fields[passiveDefaultBlockField]))
	require.Equal(t, "false", string(fields[passiveClaimSubmissionField]))
	require.JSONEq(t, `"0x0000000000000000000000000000000000000000"`, string(fields[passiveSignerField]))
	require.Equal(t, "18446744073709551615", string(fields["ChainID"]))
	require.Equal(t, `{"value":18446744073709551616}`, string(fields["Other"]))
}

func TestPassiveObserverConfigRejectsMissingFieldsAndInvalidJSON(t *testing.T) {
	for _, raw := range []string{"null", "[]", "{", `{"ChainID":31337}`} {
		t.Run(raw, func(t *testing.T) {
			_, err := patchPassiveObserverConfig([]byte(raw), map[string]any{passiveDefaultBlockField: "LATEST"})
			require.Error(t, err)
		})
	}
}

func TestReaderConfigClearsSignerAndCanRestoreExactSubmittingRecord(t *testing.T) {
	original := []byte(`{"DefaultBlock":"FINALIZED","ChainID":31337,"ClaimSubmissionEnabled":true,` +
		`"ResolvedSignerAddress":"0x0000000000000000000000000000000000000123"}`)
	copyBefore := bytes.Clone(original)
	reader, err := patchPassiveObserverConfig(original, map[string]any{passiveClaimSubmissionField: false})
	require.NoError(t, err)
	var parsed config.PersistentSubmitterConfig
	require.NoError(t, json.Unmarshal(reader, &parsed))
	require.NoError(t, parsed.Validate())
	require.False(t, parsed.ClaimSubmissionEnabled)
	require.Equal(t, common.Address{}, parsed.ResolvedSignerAddress)
	require.Equal(t, copyBefore, original, "restore writes the exact saved bytes")
	require.NoError(t, json.Unmarshal(original, &parsed))
	require.True(t, parsed.ClaimSubmissionEnabled)
	require.Equal(t, common.HexToAddress("0x123"), parsed.ResolvedSignerAddress)
}

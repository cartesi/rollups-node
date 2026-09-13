// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	passiveClaimSubmissionField = "ClaimSubmissionEnabled"
	passiveDefaultBlockField    = "DefaultBlock"
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
		setupStep   = "setup"
		stopStep    = "stop"
		restoreStep = "restore"
		restartStep = "restart"
		closeStep   = "close repository"
	)
	for _, exitAt := range []string{"none", setupStep, stopStep, restoreStep} {
		t.Run(exitAt, func(t *testing.T) {
			var calls []string
			t.Run("cleanup", func(t *testing.T) {
				t.Cleanup(func() { calls = append(calls, closeStep) })
				step := func(name string) func() {
					return func() {
						calls = append(calls, name)
						if name == exitAt {
							// SkipNow uses Goexit, like Fatal, but does not fail
							// the parent test that checks the remaining cleanup.
							t.SkipNow()
						}
					}
				}
				registerPassiveObserverCleanup(t, step(stopStep), step(restoreStep), step(restartStep))
				if exitAt == setupStep {
					// An initial stop can fail before the reader starts.
					// Cleanup must still restore the default fixture.
					t.SkipNow()
				}
			})
			want := []string{stopStep, restoreStep, restartStep, closeStep}
			if exitAt == restoreStep {
				want = []string{stopStep, restoreStep, closeStep}
			}
			require.Equal(t, want, calls)
		})
	}
}

func TestPassiveObserverConfigPreservesOriginalAndUnrelatedFields(t *testing.T) {
	raw := []byte(`{"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":true,` +
		`"ChainID":18446744073709551615,"Other":{"value":18446744073709551616}}`)
	original := bytes.Clone(raw)
	updated, err := patchPassiveObserverConfig(raw, map[string]any{passiveDefaultBlockField: "LATEST", passiveClaimSubmissionField: false})
	require.NoError(t, err)
	require.Equal(t, original, raw, "cleanup must retain the original payload unchanged")
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(updated, &fields))
	require.Equal(t, `"LATEST"`, string(fields[passiveDefaultBlockField]))
	require.Equal(t, "false", string(fields[passiveClaimSubmissionField]))
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

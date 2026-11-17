// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

type machineToolReplaySummary struct {
	ProcessedInputs uint64 `json:"processed_inputs"`
	LastInputIndex  string `json:"last_input_index"`
	MachineRoot     string `json:"machine_root"`
}

// TestMachineToolReplayRejectedInput replays a reject-loop application through
// a range that ends on a rejected input and through the accepted input after
// it. Rejected inputs are skipped, so the stored roots must match the last
// accepted input in each range.
func TestMachineToolReplayRejectedInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()

	appName := uniqueAppName("replay-reject")
	dappPath := resolveIntegrationDappPath(t,
		envOrDefault("CARTESI_TEST_REJECT_DAPP_PATH", "applications/reject-loop-dapp"),
	)

	_, err := deployApplication(ctx, appName, dappPath, "--salt", uniqueSalt())
	require.NoError(t, err, "deploy reject-loop application")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		require.NoError(t, disableApplication(cleanupCtx, appName))
	})

	for i := range 3 {
		index, _, err := sendInput(ctx, appName, fmt.Sprintf("replay-rejected-%d", i))
		require.NoError(t, err, "send input %d", i)
		require.Equal(t, uint64(i), index, "input index mismatch")
	}

	inputs := make([]*model.Input, 3)
	for i := range inputs {
		processCtx, processCancel := context.WithTimeout(ctx, inputProcessingTimeout)
		inputs[i], err = waitForInputProcessed(processCtx, t, appName, uint64(i))
		processCancel()
		require.NoError(t, err, "wait for input %d processing", i)
	}

	require.Equal(t, model.InputCompletionStatus_Accepted, inputs[0].Status)
	require.Equal(t, model.InputCompletionStatus_Rejected, inputs[1].Status)
	require.Equal(t, model.InputCompletionStatus_Accepted, inputs[2].Status)
	require.NotNil(t, inputs[0].MachineHash)
	require.NotNil(t, inputs[2].MachineHash)

	tests := []struct {
		name          string
		toInputIndex  uint64
		wantRoot      common.Hash
		wantProcessed uint64
	}{
		{
			name:          "through rejected",
			toInputIndex:  1,
			wantRoot:      *inputs[0].MachineHash,
			wantProcessed: 2,
		},
		{
			name:          "through next accepted",
			toInputIndex:  2,
			wantRoot:      *inputs[2].MachineHash,
			wantProcessed: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := filepath.Join(t.TempDir(), "machine")
			args := []string{
				"replay",
				"--template", dappPath,
				"--application", appName,
				"--to-input-index", strconv.FormatUint(tt.toInputIndex, 10),
				"--store", store,
			}

			out := runMachineTool(ctx, t, args...)
			var summary machineToolReplaySummary
			require.NoError(t, json.Unmarshal([]byte(out), &summary), "parse replay summary: %s", out)
			require.Equal(t, tt.wantProcessed, summary.ProcessedInputs)
			require.Equal(t, fmt.Sprintf("0x%x", tt.toInputIndex), summary.LastInputIndex)
			require.Equal(t, strings.ToLower(tt.wantRoot.Hex()), strings.ToLower(summary.MachineRoot))
			require.FileExists(t, filepath.Join(store, "hash_tree.sht"))
		})
	}
}

func resolveIntegrationDappPath(t testing.TB, path string) string {
	t.Helper()
	if filepath.IsAbs(path) {
		return path
	}
	root, err := repositoryRoot()
	require.NoError(t, err, "resolve repository root")
	return filepath.Join(root, path)
}

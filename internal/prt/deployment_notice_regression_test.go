// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestProtectionRepeatedDeploymentFailureIsThrottled(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	clock := prtprotection.Clock{EpochIndex: 1, Root: common.HexToAddress("0x123"), ClosingBlock: new(uint64(200))}
	providerErr := errors.New("provider unavailable")
	for _, number := range []uint64{100, 101} {
		head := observerFacts(number)
		// deploymentReader.call uses this exact wrapper for every failed pinned call.
		readErr := fmt.Errorf("deployment %s at %s, block %d hash %s: %w",
			"CM_MARCHID", common.HexToAddress("0x456"), head.Number, head.Hash, providerErr)
		f.s.reportProtectionStops(t.Context(), f.app, []prtprotection.Clock{clock}, head, deploymentNone, 0, readErr)
	}
	require.Equal(t, 1, strings.Count(output.String(), "PRT protection stopped"), output.String())
}

func TestDeploymentMissingDependencyWarningIsThrottled(t *testing.T) {
	adapter, backend := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
	backend.absent[backend.factory] = true
	service := newDeploymentObserverService()
	var logs bytes.Buffer
	service.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	app := prtRevertTestApp()
	app.IConsensusAddress = backend.consensus
	for range 2 {
		snapshot, err := service.observeDeployment(t.Context(), app, adapter, backend.head)
		require.NoError(t, err)
		require.Equal(t, deploymentNone, snapshot.Assessment.State)
	}
	require.Equal(t, 1, strings.Count(logs.String(), "PRT deployment evidence is unavailable"), logs.String())
	require.Contains(t, logs.String(), "reason=empty_return")
	require.Contains(t, logs.String(), backend.factory.Hex())
}

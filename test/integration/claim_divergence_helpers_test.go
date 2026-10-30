// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

type claimDivergenceExpectation struct {
	reason, activeDiagnostic, legacyTick, watchTransition *regexp.Regexp
}

// The roots come from the fixture's real executions, independently of whichever
// observer writes the first diagnosis. The source proof is replayed for local's
// covered block; it does not describe the local execution of that interval.
func expectClaimDivergence(t testing.TB, name string, app, consensus common.Address,
	kind model.Consensus, claimed, local *model.Epoch,
) claimDivergenceExpectation {
	t.Helper()
	for _, epoch := range []*model.Epoch{claimed, local} {
		require.NotNil(t, epoch)
		require.NotNil(t, epoch.MachineHash)
		require.NotNil(t, epoch.TxBufferDataBlock)
	}
	require.NotEqual(t, *claimed.MachineHash, *local.MachineHash, "the stale proof must differ from actual target execution")
	observedMachine, observedOutputs := claimed.MachineHash.Hex(), claimed.TxBufferDataBlock.Hex()
	localMachine, localOutputs := local.MachineHash.Hex(), local.TxBufferDataBlock.Hex()
	watchReason := fmt.Sprintf(
		"independent_claim_mismatch: covered block %d, on-chain machine=%s outputs=%s, validated local machine=%s outputs=%s",
		local.LastBlock, observedMachine, observedOutputs, localMachine, localOutputs)

	// Exclude escaped/multiline additions: a joined database failure must not
	// become an allowed Tick merely because it also contains a known mismatch.
	const reasonText = `[^"\\\r\n]*`
	legacyReason := `authority_divergence_at_(?:submission|acceptance):` + reasonText + regexp.QuoteMeta(fmt.Sprintf(
		"on-chain outputsMerkleRoot=%s machineMerkleRoot=%s, our outputsMerkleRoot=%s machineMerkleRoot=%s, epoch %d (lastBlock %d).",
		observedOutputs, observedMachine, localOutputs, localMachine, local.Index, local.LastBlock)) + reasonText
	legacyLabel := `marking application as diverged \(terminal\)`
	if kind == model.Consensus_Quorum {
		legacyReason = `quorum_divergence_at_staging:` + reasonText + regexp.QuoteMeta(fmt.Sprintf(
			"on-chain machineMerkleRoot=%s, our machineMerkleRoot=%s, epoch %d (lastBlock %d).",
			observedMachine, localMachine, local.Index, local.LastBlock)) + reasonText
		legacyLabel = `claim divergence detected`
	} else {
		require.Equal(t, model.Consensus_Authority, kind)
	}
	identity := `.*service=claimer.*application=` + regexp.QuoteMeta(name) + `(?: |$).*address=(?i:` + regexp.QuoteMeta(app.Hex()) + `)`
	watch := `Divergent on-chain claim observed` + identity + `.*consensus=(?i:` + regexp.QuoteMeta(consensus.Hex()) + `)` +
		fmt.Sprintf(`.*covered_block=%d\b.*machine_merkle_root=%s\b.*outputs_merkle_root=%s\b`,
			local.LastBlock, observedMachine, observedOutputs) +
		`.*comparison=mismatch\b.*reason=` + regexp.QuoteMeta(strconv.Quote(watchReason)) + `.*phase=staged\b`
	watchTransition := `marking application as diverged \(terminal\)` + identity +
		`.*reason=` + regexp.QuoteMeta(strconv.Quote(watchReason))
	legacy := legacyLabel + identity + `.*reason="` + legacyReason + `"`
	return claimDivergenceExpectation{
		reason:           regexp.MustCompile(`(?m)^Reason: (` + regexp.QuoteMeta(watchReason) + `|` + legacyReason + `)$`),
		activeDiagnostic: regexp.MustCompile(`(?:` + watch + `|` + legacy + `)`),
		watchTransition:  regexp.MustCompile(watchTransition),
		legacyTick:       regexp.MustCompile(`Tick service=claimer .*error="` + legacyReason + `"$`),
	}
}

func (e claimDivergenceExpectation) requireReason(t testing.TB, status string) string {
	t.Helper()
	match := e.reason.FindStringSubmatch(status)
	require.Len(t, match, 2, "DIVERGED reason must identify the fixture's proven mismatch:\n%s", status)
	return match[1]
}

func (e claimDivergenceExpectation) expectedLogs(active bool) []ExpectedLog {
	return []ExpectedLog{{
		Pattern: e.activeDiagnostic, Level: LevelError, Required: active,
		Reason: "proven mismatch of this application's attacked claim; either observer may diagnose first",
	}, {
		Pattern: e.watchTransition, Level: LevelError,
		Reason: "the observer reports the exact committed status transition",
	}, {
		Pattern: e.legacyTick, Level: LevelError,
		Reason: "legacy observer may also return the exact same claim mismatch to its Tick",
	}}
}

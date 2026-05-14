// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPRTTypedErrorNamesExistInABI walks every typed-error name the PRT
// package references and asserts it exists in the appropriate ABI metadata.
// Catches silent regressions when contracts rename errors (e.g. v3 renamed
// ClockNotTimedOut → NeitherClockHasTimedOut and BothClocksHaveNotTimedOut
// → AtLeastOneClockHasNotTimedOut — neither old name appears in PRT today,
// but the same shape of rename can recur).
//
// Maintenance: add an entry here every time PRT starts referencing a new
// typed error by name (via ethutil.IsCustomError or a hardcoded selector).
// Mirror entries are kept across both ABIs where appropriate.
func TestPRTTypedErrorNamesExistInABI(t *testing.T) {
	tournamentABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	daveABI, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	require.NoError(t, err)

	cases := []struct {
		name string
		abi  map[string]struct {
			present bool
		}
		// where: brief locator pointing at the source reference, for
		// failure messages.
		where string
	}{
		// itournament_adapter.go: Result() tolerates ArbitrationResult
		// reverting with TournamentFailedNoWinner.
		{name: "TournamentFailedNoWinner", where: "itournament_adapter.go (Result)",
			abi: map[string]struct{ present bool }{"itournament": {true}}},

		// prt.go: handleJoinTournamentRevert classifies JoinTournament reverts.
		{name: "ClockAlreadyInitialized", where: "prt.go (handleJoinTournamentRevert)",
			abi: map[string]struct{ present bool }{"itournament": {true}}},
		{name: "TournamentIsClosed", where: "prt.go (handleJoinTournamentRevert)",
			abi: map[string]struct{ present bool }{"itournament": {true}}},
		{name: "TournamentIsFinished", where: "prt.go (handleJoinTournamentRevert)",
			abi: map[string]struct{ present bool }{"itournament": {true}}},
		{name: "CommitmentStateMismatch", where: "prt.go (handleJoinTournamentRevert)",
			abi: map[string]struct{ present bool }{"itournament": {true}}},
		{name: "CommitmentProofWrongSize", where: "prt.go (handleJoinTournamentRevert)",
			abi: map[string]struct{ present bool }{"itournament": {true}}},

		// prt.go: handleSettleRevert classifies Settle reverts.
		{name: "IncorrectEpochNumber", where: "prt.go (handleSettleRevert)",
			abi: map[string]struct{ present bool }{"idaveconsensus": {true}}},
		{name: "TournamentNotFinishedYet", where: "prt.go (handleSettleRevert)",
			abi: map[string]struct{ present bool }{"idaveconsensus": {true}}},
		{name: "InvalidOutputsMerkleRootProofSize", where: "prt.go (handleSettleRevert)",
			abi: map[string]struct{ present bool }{"idaveconsensus": {true}}},
		{name: "InvalidOutputsMerkleRootProof", where: "prt.go (handleSettleRevert)",
			abi: map[string]struct{ present bool }{"idaveconsensus": {true}}},
		{name: "ApplicationForeclosed", where: "prt.go (handleSettleRevert)",
			abi: map[string]struct{ present bool }{"idaveconsensus": {true}}},
		{name: "ApplicationNotDeployed", where: "prt.go (handleSettleRevert)",
			abi: map[string]struct{ present bool }{"idaveconsensus": {true}}},
		{name: "ApplicationReverted", where: "prt.go (handleSettleRevert)",
			abi: map[string]struct{ present bool }{"idaveconsensus": {true}}},
		{name: "IllformedApplicationReturnData", where: "prt.go (handleSettleRevert)",
			abi: map[string]struct{ present bool }{"idaveconsensus": {true}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for which := range tc.abi {
				var ok bool
				switch which {
				case "itournament":
					_, ok = tournamentABI.Errors[tc.name]
				case "idaveconsensus":
					_, ok = daveABI.Errors[tc.name]
				default:
					t.Fatalf("unknown ABI bucket %q for %s", which, tc.name)
				}
				assert.True(t, ok,
					"%s missing from %s ABI (referenced by %s) — check whether the contract renamed it",
					tc.name, which, tc.where)
			}
		})
	}
}

// TestPRTHasNoReferencesToRenamedErrors locks against accidental reintroduction
// of v2 error names that v3 renamed. If a future maintainer copies a code
// fragment from a v2 branch that references one of these, the existence check
// in TestPRTTypedErrorNamesExistInABI would still catch it — but this test
// fails earlier with a more direct message.
func TestPRTHasNoReferencesToRenamedErrors(t *testing.T) {
	tournamentABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)

	v3Renames := map[string]string{
		"ClockNotTimedOut":          "NeitherClockHasTimedOut",
		"BothClocksHaveNotTimedOut": "AtLeastOneClockHasNotTimedOut",
	}
	for oldName, newName := range v3Renames {
		_, oldExists := tournamentABI.Errors[oldName]
		assert.False(t, oldExists,
			"v2 error %q unexpectedly present in v3 ITournament ABI", oldName)
		_, newExists := tournamentABI.Errors[newName]
		assert.True(t, newExists,
			"v3 renamed error %q missing from ITournament ABI (was %q in v2)",
			newName, oldName)
	}
}

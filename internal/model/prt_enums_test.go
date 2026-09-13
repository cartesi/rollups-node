// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTournamentKindValues(t *testing.T) {
	expected := []string{"LEAF", "NON_LEAF"}
	actual := make([]string, len(TournamentKindAllValues))
	for i, value := range TournamentKindAllValues {
		actual[i] = value.String()
	}
	require.Equal(t, expected, actual)
	for _, text := range expected {
		var value TournamentKind
		require.NoError(t, value.Scan(text))
		require.Equal(t, text, value.String())
		require.NoError(t, value.Scan([]byte(text)))
		require.Equal(t, text, value.String())
	}
	for _, invalid := range []any{nil, 1, true, "", []byte(""), enumInvalidValue, []byte(enumInvalidValue)} {
		value := TournamentKindLeaf
		require.Error(t, value.Scan(invalid))
		require.Equal(t, TournamentKindLeaf, value)
	}
}

func TestTournamentStandingStateValues(t *testing.T) {
	expected := []string{
		"MATCHES_ACTIVE", "AWAITING_CLOSURE", "ROOT_WINNER", "ROOT_FAILED",
		"INNER_WINNER", "INNER_ELIMINABLE_NO_WINNER", "INNER_ELIMINABLE_WINNER_EXPIRED",
	}
	actual := make([]string, len(TournamentStandingStateAllValues))
	for i, value := range TournamentStandingStateAllValues {
		actual[i] = value.String()
	}
	require.Equal(t, expected, actual)
	for _, text := range expected {
		var value TournamentStandingState
		require.NoError(t, value.Scan(text))
		require.Equal(t, text, value.String())
		require.NoError(t, value.Scan([]byte(text)))
		require.Equal(t, text, value.String())
	}
	for _, invalid := range []any{nil, 1, true, "", []byte(""), enumInvalidValue, []byte(enumInvalidValue)} {
		value := TournamentStandingMatchesActive
		require.Error(t, value.Scan(invalid))
		require.Equal(t, TournamentStandingMatchesActive, value)
	}
}

func TestMatchPhaseValues(t *testing.T) {
	expected := []string{"UNINITIALIZED", "BISECTING", "READY_TO_SEAL", "SEALED"}
	actual := make([]string, len(MatchPhaseAllValues))
	for i, value := range MatchPhaseAllValues {
		actual[i] = value.String()
	}
	require.Equal(t, expected, actual)
	for _, text := range expected {
		var value MatchPhase
		require.NoError(t, value.Scan(text))
		require.Equal(t, text, value.String())
		require.NoError(t, value.Scan([]byte(text)))
		require.Equal(t, text, value.String())
	}
	for _, invalid := range []any{nil, 1, true, "", []byte(""), enumInvalidValue, []byte(enumInvalidValue)} {
		value := MatchPhaseUninitialized
		require.Error(t, value.Scan(invalid))
		require.Equal(t, MatchPhaseUninitialized, value)
	}
}

func TestCommitmentSideValues(t *testing.T) {
	expected := []string{"ONE", "TWO"}
	actual := make([]string, len(CommitmentSideAllValues))
	for i, value := range CommitmentSideAllValues {
		actual[i] = value.String()
	}
	require.Equal(t, expected, actual)
	for _, text := range expected {
		var value CommitmentSide
		require.NoError(t, value.Scan(text))
		require.Equal(t, text, value.String())
		require.NoError(t, value.Scan([]byte(text)))
		require.Equal(t, text, value.String())
	}
	for _, invalid := range []any{nil, 1, true, "", []byte(""), enumInvalidValue, []byte(enumInvalidValue)} {
		value := CommitmentSideOne
		require.Error(t, value.Scan(invalid))
		require.Equal(t, CommitmentSideOne, value)
	}
}

func TestMatchTimeoutOutcomeValues(t *testing.T) {
	expected := []string{enumGoldenNone, "ONE_WINS", "TWO_WINS", "ELIMINATE_BOTH"}
	actual := make([]string, len(MatchTimeoutOutcomeAllValues))
	for i, value := range MatchTimeoutOutcomeAllValues {
		actual[i] = value.String()
	}
	require.Equal(t, expected, actual)
	for _, text := range expected {
		var value MatchTimeoutOutcome
		require.NoError(t, value.Scan(text))
		require.Equal(t, text, value.String())
		require.NoError(t, value.Scan([]byte(text)))
		require.Equal(t, text, value.String())
	}
	for _, invalid := range []any{nil, 1, true, "", []byte(""), enumInvalidValue, []byte(enumInvalidValue)} {
		value := MatchTimeoutNone
		require.Error(t, value.Scan(invalid))
		require.Equal(t, MatchTimeoutNone, value)
	}
}

func TestInnerTournamentDispositionValues(t *testing.T) {
	expected := []string{"UNSETTLED", "WINNER", "ELIMINABLE"}
	actual := make([]string, len(InnerTournamentDispositionAllValues))
	for i, value := range InnerTournamentDispositionAllValues {
		actual[i] = value.String()
	}
	require.Equal(t, expected, actual)
	for _, text := range expected {
		var value InnerTournamentDisposition
		require.NoError(t, value.Scan(text))
		require.Equal(t, text, value.String())
		require.NoError(t, value.Scan([]byte(text)))
		require.Equal(t, text, value.String())
	}
	for _, invalid := range []any{nil, 1, true, "", []byte(""), enumInvalidValue, []byte(enumInvalidValue)} {
		value := InnerTournamentUnsettled
		require.Error(t, value.Scan(invalid))
		require.Equal(t, InnerTournamentUnsettled, value)
	}
}

func TestBondDispositionValues(t *testing.T) {
	expected := []string{"TOURNAMENT_RUNNING", "NO_WINNER", "RECOVERABLE", "RECOVERED"}
	actual := make([]string, len(BondDispositionAllValues))
	for i, value := range BondDispositionAllValues {
		actual[i] = value.String()
	}
	require.Equal(t, expected, actual)
	for _, text := range expected {
		var value BondDisposition
		require.NoError(t, value.Scan(text))
		require.Equal(t, text, value.String())
		require.NoError(t, value.Scan([]byte(text)))
		require.Equal(t, text, value.String())
	}
	for _, invalid := range []any{nil, 1, true, "", []byte(""), enumInvalidValue, []byte(enumInvalidValue)} {
		value := BondDispositionTournamentRunning
		require.Error(t, value.Scan(invalid))
		require.Equal(t, BondDispositionTournamentRunning, value)
	}
}

func TestBondEventTypeValues(t *testing.T) {
	expected := []string{"PARTIAL_BOND_REFUND", "BOND_RECOVERED"}
	actual := make([]string, len(BondEventTypeAllValues))
	for i, value := range BondEventTypeAllValues {
		actual[i] = value.String()
	}
	require.Equal(t, expected, actual)
	for _, text := range expected {
		var value BondEventType
		require.NoError(t, value.Scan(text))
		require.Equal(t, text, value.String())
		require.NoError(t, value.Scan([]byte(text)))
		require.Equal(t, text, value.String())
	}
	for _, invalid := range []any{nil, 1, true, "", []byte(""), enumInvalidValue, []byte(enumInvalidValue)} {
		value := BondEventPartialRefund
		require.Error(t, value.Scan(invalid))
		require.Equal(t, BondEventPartialRefund, value)
	}
}

func TestContractMatchEnumsRejectUnknownValues(t *testing.T) {
	reasons := []MatchDeletionReason{MatchDeletionReason_STEP, MatchDeletionReason_TIMEOUT, MatchDeletionReason_CHILD_TOURNAMENT}
	winners := []WinnerCommitment{WinnerCommitment_NONE, WinnerCommitment_ONE, WinnerCommitment_TWO}
	for value := 0; value <= 255; value++ {
		reason, reasonErr := MatchDeletionReasonFromUint8(uint8(value))
		winner, winnerErr := WinnerCommitmentFromUint8(uint8(value))
		if value < len(reasons) {
			require.NoError(t, reasonErr)
			require.Equal(t, reasons[value], reason)
			require.NoError(t, winnerErr)
			require.Equal(t, winners[value], winner)
		} else {
			require.Error(t, reasonErr)
			require.Empty(t, reason)
			require.Error(t, winnerErr)
			require.Empty(t, winner)
		}
	}
}

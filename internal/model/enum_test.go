// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	enumGoldenNone   = "NONE"
	enumInvalidValue = "UNKNOWN"
)

func TestEnumScanners(t *testing.T) {
	checkEnumScanner(t, "ApplicationStatus", ApplicationStatusAllValues, (*ApplicationStatus).Scan,
		"invalid value", "ApplicationStatus",
		[]string{"OK", "FAILED", "DIVERGED", "CORRUPTED", "GUEST_EXCEPTION", "MACHINE_HALTED", "MCYCLE_OVERFLOW", "UNEXPECTED_YIELD"})
	checkEnumScanner(t, "Consensus", ConsensusAllValues, (*Consensus).Scan,
		"invalid value", "ConsensusType", []string{"AUTHORITY", "QUORUM", "PRT"})
	checkEnumScanner(t, "SnapshotPolicy", SnapshotPolicyAllValues, (*SnapshotPolicy).Scan,
		"invalid scan value", "SnapshotPolicy", []string{enumGoldenNone, "EVERY_INPUT", "EVERY_EPOCH"})
	checkEnumScanner(t, "EpochStatus", EpochStatusAllValues, (*EpochStatus).Scan,
		"invalid value", "EpochStatus", []string{
			"OPEN", "CLOSED", "INPUTS_PROCESSED", "CLAIM_COMPUTED", "CLAIM_SUBMITTED", "CLAIM_STAGED",
			"CLAIM_ACCEPTED", "CLAIM_REJECTED", "CLAIM_FORECLOSED",
		})
	checkEnumScanner(t, "InputCompletionStatus", InputCompletionStatusAllValues, (*InputCompletionStatus).Scan,
		"invalid value", "InputCompletionStatus",
		[]string{enumGoldenNone, "ACCEPTED", "REJECTED", "EXCEPTION", "MACHINE_HALTED", "OVERFLOW", "UNEXPECTED_YIELD"})
	checkEnumScanner(t, "DefaultBlock", DefaultBlockAllValues, (*DefaultBlock).Scan,
		"invalid value", "DefaultBlock", []string{"FINALIZED", "LATEST", "PENDING", "SAFE"})
	checkEnumScanner(t, "MatchDeletionReason", MatchDeletionReasonAllValues, (*MatchDeletionReason).Scan,
		"invalid value", "MatchDeletionReason", []string{"STEP", "TIMEOUT", "CHILD_TOURNAMENT", "NOT_DELETED"})
	checkEnumScanner(t, "WinnerCommitment", WinnerCommitmentAllValues, (*WinnerCommitment).Scan,
		"invalid value", "WinnerCommitment", []string{enumGoldenNone, "ONE", "TWO"})
}

func checkEnumScanner[T ~string](
	t *testing.T, name string, values []T, scan func(*T, any) error, errorPrefix, typeName string, expected []string,
) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		actual := make([]string, len(values))
		for i, value := range values {
			actual[i] = string(value)
		}
		require.Equal(t, expected, actual, "AllValues must contain the full public enum vocabulary")
		for _, text := range expected {
			value := T(text)
			t.Run(string(value), func(t *testing.T) {
				for _, input := range []any{string(value), []byte(value)} {
					var got T
					require.NoError(t, scan(&got, input))
					require.Equal(t, value, got)
				}
			})
		}
		for _, input := range []any{"INVALID", []byte("INVALID"), "", []byte(nil)} {
			got := values[0]
			text, ok := input.(string)
			if !ok {
				text = string(input.([]byte))
			}
			require.EqualError(t, scan(&got, input), fmt.Sprintf("%s '%s' for %s enum", errorPrefix, text, name))
			require.Equal(t, values[0], got, "invalid enum values must not modify the receiver")
		}
		type namedString string
		for _, input := range []any{nil, 1, true, namedString(values[0])} {
			got := values[0]
			require.EqualError(t, scan(&got, input),
				fmt.Sprintf("%s for %s enum. Enum value has to be of type string or []byte", errorPrefix, typeName))
			require.Equal(t, values[0], got, "unsupported input types must not modify the receiver")
		}
	})
}

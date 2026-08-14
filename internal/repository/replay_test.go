// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplayVerificationLevel(t *testing.T) {
	require.True(t, ReplayVerificationCanonical.IsValid())
	require.True(t, ReplayVerificationFull.IsValid())
	require.False(t, ReplayVerificationLevel(255).IsValid())
	require.Equal(t, "canonical", ReplayVerificationCanonical.String())
	require.Equal(t, "full", ReplayVerificationFull.String())
	require.Equal(t, "unknown", ReplayVerificationLevel(255).String())
}

func TestReplaySourceErrorsPreserveIdentityWithoutPayloads(t *testing.T) {
	inconsistency := &ReplayInconsistentEvidenceError{
		Kind: ReplayEvidenceReport, InputIndex: 7,
	}
	require.True(t, errors.Is(inconsistency, ErrReplayInconsistentEvidence))
	require.NotContains(t, inconsistency.Error(), "payload")

	violation := &ReplayStructureViolationError{
		Kind:                       ReplayStructureProcessedInputCount,
		InputIndex:                 7,
		ApplicationProcessedInputs: 8,
		CompletedInputCount:        9,
	}
	require.True(t, errors.Is(violation, ErrReplayInvalidStructure))
	require.Contains(t, violation.Error(), "application_processed_inputs=8")
	require.Contains(t, violation.Error(), "completed_input_count=9")
	require.NotContains(t, violation.Error(), "payload")
}

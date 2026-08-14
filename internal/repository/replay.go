// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

var (
	// ErrReplayInconsistentEvidence identifies persisted child evidence whose
	// parent is not one of the completed inputs selected for the same page.
	ErrReplayInconsistentEvidence = errors.New("persisted replay evidence is inconsistent")

	// ErrReplayInvalidStructure identifies malformed persisted replay
	// coordinates or relationships. Typed details never contain payload bytes.
	ErrReplayInvalidStructure = errors.New("persisted replay structure is invalid")
)

const unknownReplayValue = "unknown"

// ReplayVerificationLevel selects the persisted evidence read during replay.
type ReplayVerificationLevel uint8

const (
	// ReplayVerificationCanonical reads the completion outcome and canonical
	// roots. It is the inexpensive default for normal machine reconstruction.
	ReplayVerificationCanonical ReplayVerificationLevel = iota

	// ReplayVerificationFull additionally reads accepted outputs and reports.
	// For PRT applications, it also reads the compressed per-input state-hash
	// collections used to reconstruct and verify the epoch computation hash.
	// Outputs and reports are audit evidence and do not participate in that hash.
	// It is intended for explicit audits and the forthcoming
	// machine-tool/libcartesi adapter, not normal reconstruction.
	// ReplayPage evidence is scoped to the requested half-open input range;
	// ReplaySummary's completed-prefix and structural checks are application-wide.
	// A maximal PRT collection is large, so callers must provision memory for
	// all evidence associated with at least one input.
	ReplayVerificationFull
)

// IsValid reports whether level is a supported verification policy.
func (level ReplayVerificationLevel) IsValid() bool {
	return level == ReplayVerificationCanonical || level == ReplayVerificationFull
}

func (level ReplayVerificationLevel) String() string {
	switch level {
	case ReplayVerificationCanonical:
		return "canonical"
	case ReplayVerificationFull:
		return "full"
	default:
		return unknownReplayValue
	}
}

// ReplayPageRequest selects an absolute, application-local, half-open input
// range and the evidence that must accompany it.
type ReplayPageRequest struct {
	ApplicationID    int64
	FromInput        uint64
	ToInputExclusive uint64
	Limit            uint64
	Verification     ReplayVerificationLevel
}

// ReplayRepository provides a stable completed-input prefix and keyset-
// paginated replay evidence. Implementations validate persisted invariants for
// the requested verification level.
type ReplayRepository interface {
	ReplaySummary(
		ctx context.Context,
		applicationAddress common.Address,
		verification ReplayVerificationLevel,
	) (model.ReplaySummary, error)
	ReplayPage(ctx context.Context, request ReplayPageRequest) ([]*model.ReplayRecord, error)
}

// ReplayStructureViolationKind identifies one payload-free replay invariant.
type ReplayStructureViolationKind uint8

const (
	// Zero is reserved so an uninitialized kind cannot identify a real
	// structural violation and instead formats as unknown.
	ReplayStructureCompletedInputSequence ReplayStructureViolationKind = iota + 1
	ReplayStructureStateHashIndexSequence
	ReplayStructureStateHashInputOrder
	ReplayStructureProcessedInputCount
	ReplayStructureUnexpectedStateHash
)

func (kind ReplayStructureViolationKind) String() string {
	switch kind {
	case ReplayStructureCompletedInputSequence:
		return "completed_input.index_sequence"
	case ReplayStructureStateHashIndexSequence:
		return "state_hash.index_sequence"
	case ReplayStructureStateHashInputOrder:
		return "state_hash.input_order"
	case ReplayStructureProcessedInputCount:
		return "application.processed_input_count"
	case ReplayStructureUnexpectedStateHash:
		return "state_hash.unexpected_for_consensus"
	default:
		return unknownReplayValue
	}
}

// ReplayStructureViolationError contains only persisted counts and coordinates.
type ReplayStructureViolationError struct {
	Kind                       ReplayStructureViolationKind
	EpochIndex                 *uint64
	InputIndex                 uint64
	EvidenceIndex              uint64
	ExpectedIndex              uint64
	PreviousInputIndex         uint64
	ApplicationProcessedInputs uint64
	CompletedInputCount        uint64
}

func (e *ReplayStructureViolationError) Error() string {
	epoch := "<unknown>"
	if e.EpochIndex != nil {
		epoch = fmt.Sprint(*e.EpochIndex)
	}
	switch e.Kind {
	case ReplayStructureCompletedInputSequence:
		return fmt.Sprintf(
			"%v: kind=%s expected_last_input=%d actual_last_input=%d",
			ErrReplayInvalidStructure, e.Kind, e.ExpectedIndex, e.InputIndex,
		)
	case ReplayStructureStateHashIndexSequence:
		return fmt.Sprintf(
			"%v: kind=%s epoch=%s input=%d expected_index=%d actual_index=%d",
			ErrReplayInvalidStructure, e.Kind, epoch, e.InputIndex,
			e.ExpectedIndex, e.EvidenceIndex,
		)
	case ReplayStructureStateHashInputOrder:
		return fmt.Sprintf(
			"%v: kind=%s epoch=%s state_hash_index=%d previous_input=%d input=%d",
			ErrReplayInvalidStructure, e.Kind, epoch, e.EvidenceIndex,
			e.PreviousInputIndex, e.InputIndex,
		)
	case ReplayStructureProcessedInputCount:
		return fmt.Sprintf(
			"%v: kind=%s application_processed_inputs=%d completed_input_count=%d",
			ErrReplayInvalidStructure, e.Kind,
			e.ApplicationProcessedInputs, e.CompletedInputCount,
		)
	case ReplayStructureUnexpectedStateHash:
		return fmt.Sprintf(
			"%v: kind=%s epoch=%s input=%d state_hash_index=%d",
			ErrReplayInvalidStructure, e.Kind, epoch, e.InputIndex, e.EvidenceIndex,
		)
	default:
		return fmt.Sprintf("%v: kind=%s", ErrReplayInvalidStructure, unknownReplayValue)
	}
}

func (e *ReplayStructureViolationError) Unwrap() error { return ErrReplayInvalidStructure }

// ReplayEvidenceKind identifies persisted child evidence without exposing
// payloads.
type ReplayEvidenceKind uint8

const (
	// Zero is reserved so an uninitialized kind cannot identify real evidence
	// and instead formats as unknown.
	ReplayEvidenceOutput ReplayEvidenceKind = iota + 1
	ReplayEvidenceReport
	ReplayEvidenceStateHash
)

func (kind ReplayEvidenceKind) String() string {
	switch kind {
	case ReplayEvidenceOutput:
		return "output"
	case ReplayEvidenceReport:
		return "report"
	case ReplayEvidenceStateHash:
		return "state_hash"
	default:
		return unknownReplayValue
	}
}

// ReplayInconsistentEvidenceError reports evidence whose input is not among
// the completed inputs selected for the page.
type ReplayInconsistentEvidenceError struct {
	Kind       ReplayEvidenceKind
	InputIndex uint64
}

func (e *ReplayInconsistentEvidenceError) Error() string {
	return fmt.Sprintf(
		"%v: %s row for input %d has no completed replay input in the selected page",
		ErrReplayInconsistentEvidence,
		e.Kind,
		e.InputIndex,
	)
}

func (e *ReplayInconsistentEvidenceError) Unwrap() error {
	return ErrReplayInconsistentEvidence
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package replay

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

// Executor is the machine behavior required by Run. Machine lifecycle and
// persistence remain the caller's responsibility.
type Executor interface {
	ProcessedInputs() uint64
	Advance(
		ctx context.Context,
		input []byte,
		epochIndex uint64,
		inputIndex uint64,
		computeHashes bool,
	) (*model.AdvanceResult, error)
}

// Options selects an absolute, application-local, half-open replay range.
type Options struct {
	Application      *model.Application
	FromInput        uint64
	ToInputExclusive uint64
	BatchSize        uint64
	Verification     repository.ReplayVerificationLevel
}

// Result summarizes a successfully verified replay.
type Result struct {
	ReplayedInputs uint64
}

// Run reconstructs and verifies a machine over the requested input range. It
// never closes or stores executor; callers discard it after any error.
func Run(
	ctx context.Context,
	source repository.ReplayRepository,
	executor Executor,
	opts Options,
) (Result, error) {
	if err := validateOptions(source, executor, opts); err != nil {
		return Result{}, err
	}
	applicationLabel := opts.Application.Name
	if applicationLabel == "" {
		applicationLabel = opts.Application.IApplicationAddress.String()
	}
	summary, err := source.ReplaySummary(
		ctx,
		opts.Application.IApplicationAddress,
		opts.Verification,
	)
	if err != nil {
		return Result{}, classifySourceError(applicationLabel, err)
	}
	if err := verifySummary(applicationLabel, opts.Application, summary, opts.FromInput); err != nil {
		return Result{}, err
	}

	isPRT := opts.Application.IsDaveConsensus()
	computeHashes := opts.Verification == repository.ReplayVerificationFull && isPRT
	contradiction := func(inputIndex uint64, field string, expected, actual any) error {
		return newContradiction(applicationLabel, nil, inputIndex, field, expected, actual)
	}
	var replayed uint64
	for opts.FromInput+replayed < opts.ToInputExclusive {
		from := opts.FromInput + replayed
		remaining := opts.ToInputExclusive - from
		limit := min(opts.BatchSize, remaining)
		records, err := source.ReplayPage(ctx, repository.ReplayPageRequest{
			ApplicationID:    summary.ApplicationID,
			FromInput:        from,
			ToInputExclusive: opts.ToInputExclusive,
			Limit:            limit,
			Verification:     opts.Verification,
		})
		if err != nil {
			return Result{}, classifySourceError(applicationLabel, err)
		}
		if err := validatePage(applicationLabel, records, from, limit, remaining); err != nil {
			return Result{}, err
		}
		for _, record := range records {
			input := &record.Input
			actual, err := executor.Advance(
				ctx,
				input.RawData,
				input.EpochIndex,
				input.InputIndex,
				computeHashes,
			)
			if err != nil {
				return Result{}, fmt.Errorf(
					"%w: input %d: %w",
					ErrExecution,
					input.InputIndex,
					err,
				)
			}
			if err := compareRecord(
				applicationLabel,
				opts.Application.ID,
				isPRT,
				opts.Verification,
				record,
				actual,
			); err != nil {
				return Result{}, err
			}
			replayed++
			if expected := input.InputIndex + 1; executor.ProcessedInputs() != expected {
				return Result{}, contradiction(
					input.InputIndex,
					"executor.processed_inputs",
					expected,
					executor.ProcessedInputs(),
				)
			}
		}
	}
	return Result{ReplayedInputs: replayed}, nil
}

func verifySummary(
	application string,
	expected *model.Application,
	actual model.ReplaySummary,
	firstInput uint64,
) error {
	contradiction := func(field string, expected, actual any) error {
		return newContradiction(application, nil, firstInput, field, expected, actual)
	}
	if actual.ApplicationID != expected.ID {
		return contradiction("application_id", expected.ID, actual.ApplicationID)
	}
	if actual.Consensus != expected.ConsensusType {
		return contradiction("consensus", expected.ConsensusType, actual.Consensus)
	}
	if actual.ProcessedInputs != expected.ProcessedInputs {
		return contradiction(
			"processed_inputs.count",
			expected.ProcessedInputs,
			actual.ProcessedInputs,
		)
	}
	return nil
}

func validateOptions(source repository.ReplayRepository, executor Executor, opts Options) error {
	switch {
	case source == nil:
		return fmt.Errorf("%w: replay source is nil", ErrInvalidOptions)
	case executor == nil:
		return fmt.Errorf("%w: replay executor is nil", ErrInvalidOptions)
	case opts.Application == nil:
		return fmt.Errorf("%w: application is nil", ErrInvalidOptions)
	case !opts.Verification.IsValid():
		return fmt.Errorf("%w: unsupported verification level %d", ErrInvalidOptions, opts.Verification)
	case opts.BatchSize == 0:
		return fmt.Errorf("%w: replay batch size must be greater than zero", ErrInvalidOptions)
	case opts.FromInput > opts.ToInputExclusive:
		return fmt.Errorf(
			"%w: lower bound %d exceeds upper bound %d",
			ErrInvalidOptions,
			opts.FromInput,
			opts.ToInputExclusive,
		)
	case opts.ToInputExclusive > opts.Application.ProcessedInputs:
		return fmt.Errorf(
			"%w: upper bound %d exceeds application processed input count %d",
			ErrInvalidOptions,
			opts.ToInputExclusive,
			opts.Application.ProcessedInputs,
		)
	case executor.ProcessedInputs() != opts.FromInput:
		return fmt.Errorf(
			"%w: executor has %d processed inputs; replay starts at %d",
			ErrInvalidOptions,
			executor.ProcessedInputs(),
			opts.FromInput,
		)
	default:
		return nil
	}
}

func validatePage(
	application string,
	records []*model.ReplayRecord,
	firstInput uint64,
	limit uint64,
	remaining uint64,
) error {
	contradiction := func(inputIndex uint64, field string, expected, actual any) error {
		return newContradiction(application, nil, inputIndex, field, expected, actual)
	}
	if uint64(len(records)) > limit {
		return contradiction(firstInput, "replay_page.count", limit, len(records))
	}
	if len(records) == 0 {
		return contradiction(firstInput, "replay_sequence.remaining_records", remaining, 0)
	}
	for offset, record := range records {
		expectedInput := firstInput + uint64(offset)
		if record == nil {
			return contradiction(expectedInput, "replay_record", "record", "nil")
		}
		if record.Input.InputIndex != expectedInput {
			return contradiction(
				expectedInput,
				"input_index.sequence",
				expectedInput,
				record.Input.InputIndex,
			)
		}
	}
	return nil
}

func classifySourceError(application string, err error) error {
	var structure *repository.ReplayStructureViolationError
	if errors.As(err, &structure) {
		return newContradiction(
			application,
			structure.EpochIndex,
			structure.InputIndex,
			"source."+structure.Kind.String(),
			"valid persisted replay structure",
			structure.Error(),
		)
	}
	var inconsistency *repository.ReplayInconsistentEvidenceError
	if errors.As(err, &inconsistency) {
		return newContradiction(
			application,
			nil,
			inconsistency.InputIndex,
			"source."+inconsistency.Kind.String()+".completed_input",
			"completed replay input in page",
			"not present",
		)
	}
	return fmt.Errorf("%w: source: %w", ErrExecution, err)
}

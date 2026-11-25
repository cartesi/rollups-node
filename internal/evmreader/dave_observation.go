// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

// observeDaveInputs records every InputAdded log up to endBlock, independent of
// epoch attribution. The counter walk is seeded by the observation frontier,
// not by the stored input count. A missing frontier is a first run (or an
// upgrade from the cursor-only layout): the stored input cursor is authoritative
// for what was already projected, so the walk resumes after it.
//
// Reorg trust boundary: committed observation rows are assumed to come from the
// configured L1 view. The node defaults to FINALIZED blocks, whose history
// cannot be reorged, so attribution trusts rows behind the frontier instead of
// re-reading L1 at seal time. A deployment configured with LATEST or PENDING
// forfeits that guarantee; a reorg that rewrites input history below the
// frontier is stored-state divergence and requires an operator reset.
func (r *Service) observeDaveInputs(
	ctx context.Context,
	app appContracts,
	endBlock uint64,
) (*InputObservation, error) {
	address := app.application.IApplicationAddress.Hex()

	frontier, err := r.repository.GetInputObservation(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed to get input observation: %w", err)
	}

	startBlock := app.application.LastInputCheckBlock + 1
	prevCount := uint64(0)
	if frontier != nil {
		startBlock = frontier.ThroughBlock + 1
		prevCount = frontier.NextInputIndex
	} else {
		storedCount, err := r.repository.GetNumberOfInputs(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("failed to get stored input count: %w", err)
		}
		prevCount = storedCount
	}

	// No input can exist before the InputBox deployment.
	if app.application.IInputBoxBlock > startBlock {
		startBlock = app.application.IInputBoxBlock
	}

	if startBlock > endBlock {
		if frontier != nil {
			return frontier, nil
		}
		// Record the already-scanned prefix so the next tick resumes after it.
		// This is the upgrade path from a database that only had the input
		// cursor.
		throughBlock := app.application.LastInputCheckBlock
		if app.application.IInputBoxBlock > 0 && throughBlock < app.application.IInputBoxBlock-1 {
			throughBlock = app.application.IInputBoxBlock - 1
		}
		observation := &InputObservation{
			ThroughBlock:   throughBlock,
			NextInputIndex: prevCount,
		}
		if err := r.repository.ObserveDaveInputs(ctx, address,
			throughBlock, prevCount, nil); err != nil {
			return nil, fmt.Errorf("failed to initialize input observation: %w", err)
		}
		return observation, nil
	}

	// Observation keeps every log the counter walk sees, including indices
	// beyond the current OPEN epoch's cap. Attribution decides which of them
	// belong to which epoch.
	inputs, endCount, err := r.fetchInputs(ctx, app, startBlock, endBlock,
		new(big.Int).SetUint64(prevCount), 0, math.MaxUint64)
	if err != nil {
		if errors.Is(err, ethutil.ErrMonotonicViolation) {
			return nil, fmt.Errorf("%w: %w", errInconclusiveInputView, err)
		}
		return nil, fmt.Errorf("failed to observe inputs: %w", err)
	}
	if endCount < prevCount || endCount-prevCount != uint64(len(inputs)) {
		return nil, fmt.Errorf(
			"%w: input count mismatch: stored %d, on-chain %d, expected %d new inputs, got %d",
			errInconclusiveInputView, prevCount, endCount, endCount-prevCount, len(inputs))
	}

	if err := r.repository.ObserveDaveInputs(ctx, address, endBlock, endCount, inputs); err != nil {
		return nil, fmt.Errorf("failed to store observed inputs: %w", err)
	}

	return &InputObservation{
		ThroughBlock:   endBlock,
		NextInputIndex: endCount,
	}, nil
}

// collectDaveEpochInputs returns exactly the inputs in [lower, upper), merging
// the observed-but-unassigned rows with rows already projected into the same
// epoch. The latter happens when the OPEN scan stored inputs before the seal
// arrived (or before an earlier projection in the same epoch). Rows committed
// by the observation frontier are trusted as final history; see
// observeDaveInputs for the finalized-block assumption.
func (r *Service) collectDaveEpochInputs(
	ctx context.Context,
	app appContracts,
	epoch *Epoch,
) ([]*Input, error) {
	return r.collectDaveEpochInputRange(ctx, app, epoch,
		epoch.InputIndexLowerBound, epoch.InputIndexUpperBound)
}

// collectDaveEpochInputRange returns exactly the inputs in [lower, upper) of an
// epoch. The OPEN projection passes only its newly grown prefix; sealed
// attribution passes the full sealed range.
func (r *Service) collectDaveEpochInputRange(
	ctx context.Context,
	app appContracts,
	epoch *Epoch,
	lower, upper uint64,
) ([]*Input, error) {
	if upper <= lower {
		return nil, nil
	}
	if lower < epoch.InputIndexLowerBound || upper > epoch.InputIndexUpperBound {
		return nil, fmt.Errorf("%w: epoch %d range [%d, %d) is outside [%d, %d)",
			repository.ErrEpochInputRangeConflict, epoch.Index, lower, upper,
			epoch.InputIndexLowerBound, epoch.InputIndexUpperBound)
	}
	address := app.application.IApplicationAddress.Hex()

	observed, err := r.repository.ListObservedInputs(ctx, address, lower, upper)
	if err != nil {
		return nil, fmt.Errorf("failed to list observed inputs for epoch %d: %w", epoch.Index, err)
	}

	epochIndex := epoch.Index
	// Bound the attributed lookup too: a projection passes only its newly
	// grown prefix, and listing the whole epoch here would restore the
	// per-tick O(epoch size) read even though only the delta is inserted.
	attributed, _, err := r.repository.ListInputs(ctx, address,
		repository.InputFilter{
			EpochIndex: &epochIndex,
			IndexRange: &repository.Range{Start: lower, End: upper - 1},
		},
		repository.Pagination{}, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list attributed inputs for epoch %d: %w", epoch.Index, err)
	}

	byIndex := make(map[uint64]*Input, len(observed))
	for _, input := range observed {
		if input.Index < lower || input.Index >= upper {
			continue
		}
		byIndex[input.Index] = input
	}
	for _, input := range attributed {
		if input.Index < lower || input.Index >= upper {
			continue
		}
		if previous, ok := byIndex[input.Index]; ok &&
			(previous.TransactionHash != input.TransactionHash ||
				previous.LogIndex != input.LogIndex) {
			return nil, fmt.Errorf(
				"%w: epoch %d input %d has conflicting L1 identity",
				repository.ErrInputLogIdentityConflict, epoch.Index, input.Index)
		}
		byIndex[input.Index] = input
	}

	if uint64(len(byIndex)) != upper-lower {
		return nil, fmt.Errorf(
			"%w: epoch %d input count mismatch: expected %d (indices %d to %d), got %d",
			errInconclusiveEpochView, epoch.Index, upper-lower, lower, upper, len(byIndex))
	}

	inputs := make([]*Input, 0, upper-lower)
	for index := lower; index < upper; index++ {
		input, ok := byIndex[index]
		if !ok {
			return nil, fmt.Errorf(
				"%w: epoch %d is missing input index %d",
				errInconclusiveEpochView, epoch.Index, index)
		}
		if input.BlockNumber > epoch.LastBlock {
			return nil, fmt.Errorf(
				"%w: epoch %d input %d is at block %d, after seal block %d",
				errInconclusiveEpochView, epoch.Index, input.Index,
				input.BlockNumber, epoch.LastBlock)
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

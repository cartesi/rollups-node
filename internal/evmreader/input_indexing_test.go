// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math"
	"testing"
	"testing/quick"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
)

// makeInput creates a test input with the given index and block number.
func makeInput(index, blockNumber uint64) *Input {
	return &Input{
		Index:       index,
		BlockNumber: blockNumber,
		Status:      InputCompletionStatus_None,
	}
}

// collectEpochs flattens the epoch→inputs map into sorted slices for assertions.
// Returns epochs sorted by Index and a parallel slice of input slices.
func collectEpochs(m map[*Epoch][]*Input) (epochs []*Epoch, inputs [][]*Input) {
	for e, ins := range m {
		epochs = append(epochs, e)
		inputs = append(inputs, ins)
	}
	// Sort by epoch index for deterministic assertions
	for i := 0; i < len(epochs); i++ {
		for j := i + 1; j < len(epochs); j++ {
			if epochs[j].Index < epochs[i].Index {
				epochs[i], epochs[j] = epochs[j], epochs[i]
				inputs[i], inputs[j] = inputs[j], inputs[i]
			}
		}
	}
	return
}

func TestIndexInputsIntoEpochs(t *testing.T) {
	const epochLength uint64 = 10

	t.Run("InputsSpanningMultipleEpochs", func(t *testing.T) {
		// Inputs in epochs 0, 1, 2 (blocks 5, 15, 25) with mostRecent=29 (epoch 2 not closed)
		inputs := []*Input{
			makeInput(0, 5),  // epoch 0
			makeInput(1, 15), // epoch 1
			makeInput(2, 25), // epoch 2
		}

		result, err := indexInputsIntoEpochs(epochLength, nil, inputs, 29)
		require.NoError(t, err)
		require.Len(t, result, 3)

		epochs, epochInputs := collectEpochs(result)

		// Epoch 0: closed (input 1 at block 15 moved past it)
		require.Equal(t, uint64(0), epochs[0].Index)
		require.Equal(t, EpochStatus_Closed, epochs[0].Status)
		require.Equal(t, uint64(0), epochs[0].FirstBlock)
		require.Equal(t, uint64(9), epochs[0].LastBlock)
		require.Equal(t, uint64(0), epochs[0].InputIndexLowerBound)
		require.Equal(t, uint64(1), epochs[0].InputIndexUpperBound) // set when closing: next input index
		require.Len(t, epochInputs[0], 1)
		require.Equal(t, uint64(0), epochInputs[0][0].Index)

		// Epoch 1: closed (input 2 at block 25 moved past it)
		require.Equal(t, uint64(1), epochs[1].Index)
		require.Equal(t, EpochStatus_Closed, epochs[1].Status)
		require.Equal(t, uint64(10), epochs[1].FirstBlock)
		require.Equal(t, uint64(19), epochs[1].LastBlock)
		require.Equal(t, uint64(1), epochs[1].InputIndexLowerBound)
		require.Equal(t, uint64(2), epochs[1].InputIndexUpperBound)
		require.Len(t, epochInputs[1], 1)

		// Epoch 2: still open (mostRecent=29 < LastBlock=29, i.e. 29 >= 29 → closed!)
		// Actually 29 >= 29, so epoch 2 IS closed.
		require.Equal(t, uint64(2), epochs[2].Index)
		require.Equal(t, EpochStatus_Closed, epochs[2].Status)
		require.Equal(t, uint64(20), epochs[2].FirstBlock)
		require.Equal(t, uint64(29), epochs[2].LastBlock)
		require.Len(t, epochInputs[2], 1)
	})

	t.Run("InputsSpanningMultipleEpochsLastOpen", func(t *testing.T) {
		// Same inputs but mostRecent=28, so epoch 2 stays open
		inputs := []*Input{
			makeInput(0, 5),  // epoch 0
			makeInput(1, 15), // epoch 1
			makeInput(2, 25), // epoch 2
		}

		result, err := indexInputsIntoEpochs(epochLength, nil, inputs, 28)
		require.NoError(t, err)

		epochs, _ := collectEpochs(result)
		require.Len(t, epochs, 3)

		require.Equal(t, EpochStatus_Closed, epochs[0].Status)
		require.Equal(t, EpochStatus_Closed, epochs[1].Status)
		require.Equal(t, EpochStatus_Open, epochs[2].Status) // 28 < 29
	})

	t.Run("NoInputsNoCurrentEpoch", func(t *testing.T) {
		result, err := indexInputsIntoEpochs(epochLength, nil, nil, 100)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("NoInputsCurrentEpochStaysOpen", func(t *testing.T) {
		// Current epoch at index 1 (blocks 10-19), mostRecent=15: should stay open
		currentEpoch := &Epoch{
			Index:                1,
			FirstBlock:           10,
			LastBlock:            19,
			InputIndexLowerBound: 0,
			InputIndexUpperBound: 1,
			Status:               EpochStatus_Open,
		}

		result, err := indexInputsIntoEpochs(epochLength, currentEpoch, nil, 15)
		require.NoError(t, err)
		require.Empty(t, result) // no inputs means no entries
		require.Equal(t, EpochStatus_Open, currentEpoch.Status)
	})

	t.Run("NoInputsCurrentEpochClosedByBlockAdvance", func(t *testing.T) {
		// Current epoch at index 1 (blocks 10-19), mostRecent=19: should close
		currentEpoch := &Epoch{
			Index:                1,
			FirstBlock:           10,
			LastBlock:            19,
			InputIndexLowerBound: 0,
			InputIndexUpperBound: 1,
			Status:               EpochStatus_Open,
		}

		result, err := indexInputsIntoEpochs(epochLength, currentEpoch, nil, 19)
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, EpochStatus_Closed, currentEpoch.Status)
		// The epoch should be in the map with empty inputs
		inputs, ok := result[currentEpoch]
		require.True(t, ok)
		require.Empty(t, inputs)
	})

	t.Run("InputForAlreadyClosedEpochReturnsError", func(t *testing.T) {
		currentEpoch := &Epoch{
			Index:      1,
			FirstBlock: 10,
			LastBlock:  19,
			Status:     EpochStatus_Closed, // already closed
		}
		inputs := []*Input{
			makeInput(5, 15), // targets epoch 1 which is closed
		}

		result, err := indexInputsIntoEpochs(epochLength, currentEpoch, inputs, 25)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInputForNonOpenEpoch)
		require.Nil(t, result)
	})

	t.Run("InputForInputsProcessedEpochReturnsError", func(t *testing.T) {
		currentEpoch := &Epoch{
			Index:      1,
			FirstBlock: 10,
			LastBlock:  19,
			Status:     EpochStatus_InputsProcessed,
		}
		inputs := []*Input{
			makeInput(5, 15),
		}

		result, err := indexInputsIntoEpochs(epochLength, currentEpoch, inputs, 25)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInputForNonOpenEpoch)
		require.Nil(t, result)
	})

	t.Run("MultipleInputsSameEpoch", func(t *testing.T) {
		inputs := []*Input{
			makeInput(0, 10),
			makeInput(1, 12),
			makeInput(2, 18),
		}

		result, err := indexInputsIntoEpochs(epochLength, nil, inputs, 18)
		require.NoError(t, err)
		require.Len(t, result, 1)

		epochs, epochInputs := collectEpochs(result)
		require.Equal(t, uint64(1), epochs[0].Index)
		require.Equal(t, EpochStatus_Open, epochs[0].Status) // 18 < 19
		require.Equal(t, uint64(0), epochs[0].InputIndexLowerBound)
		require.Equal(t, uint64(3), epochs[0].InputIndexUpperBound) // last input index + 1
		require.Len(t, epochInputs[0], 3)
	})

	t.Run("ContinuesExistingOpenEpoch", func(t *testing.T) {
		currentEpoch := &Epoch{
			Index:                1,
			FirstBlock:           10,
			LastBlock:            19,
			InputIndexLowerBound: 0,
			InputIndexUpperBound: 2, // already has inputs 0,1
			Status:               EpochStatus_Open,
		}
		inputs := []*Input{
			makeInput(2, 14), // same epoch, new input
			makeInput(3, 17),
		}

		result, err := indexInputsIntoEpochs(epochLength, currentEpoch, inputs, 17)
		require.NoError(t, err)

		// The existing epoch pointer should be in the map
		epochInputs, ok := result[currentEpoch]
		require.True(t, ok)
		require.Len(t, epochInputs, 2)
		require.Equal(t, uint64(4), currentEpoch.InputIndexUpperBound) // updated to 3+1
		require.Equal(t, EpochStatus_Open, currentEpoch.Status)
	})

	t.Run("ExistingEpochClosedByNewEpochInput", func(t *testing.T) {
		currentEpoch := &Epoch{
			Index:                0,
			FirstBlock:           0,
			LastBlock:            9,
			InputIndexLowerBound: 0,
			InputIndexUpperBound: 1,
			Status:               EpochStatus_Open,
		}
		inputs := []*Input{
			makeInput(1, 10), // epoch 1 → closes epoch 0
		}

		result, err := indexInputsIntoEpochs(epochLength, currentEpoch, inputs, 15)
		require.NoError(t, err)
		require.Len(t, result, 2) // old epoch (closed) + new epoch

		// Verify old epoch was closed
		require.Equal(t, EpochStatus_Closed, currentEpoch.Status)
		require.Equal(t, uint64(1), currentEpoch.InputIndexUpperBound) // set to input.Index

		epochs, epochInputs := collectEpochs(result)

		// Epoch 0: closed, no new inputs in this batch (but was in map)
		require.Equal(t, uint64(0), epochs[0].Index)
		require.Equal(t, EpochStatus_Closed, epochs[0].Status)
		require.Empty(t, epochInputs[0])

		// Epoch 1: open with the new input
		require.Equal(t, uint64(1), epochs[1].Index)
		require.Equal(t, EpochStatus_Open, epochs[1].Status) // 15 < 19
		require.Len(t, epochInputs[1], 1)
	})

	t.Run("EpochBoundaryExact", func(t *testing.T) {
		// Input at exact epoch boundary (block 10 = first block of epoch 1)
		inputs := []*Input{
			makeInput(0, 9),  // last block of epoch 0
			makeInput(1, 10), // first block of epoch 1
		}

		result, err := indexInputsIntoEpochs(epochLength, nil, inputs, 10)
		require.NoError(t, err)
		require.Len(t, result, 2)

		epochs, epochInputs := collectEpochs(result)
		require.Equal(t, uint64(0), epochs[0].Index)
		require.Equal(t, EpochStatus_Closed, epochs[0].Status)
		require.Len(t, epochInputs[0], 1)

		require.Equal(t, uint64(1), epochs[1].Index)
		require.Equal(t, EpochStatus_Open, epochs[1].Status)
		require.Len(t, epochInputs[1], 1)
	})

	t.Run("SkippedEpochs", func(t *testing.T) {
		// Inputs in epoch 0 and epoch 5, skipping 1-4 (no inputs in between)
		inputs := []*Input{
			makeInput(0, 5),  // epoch 0
			makeInput(1, 55), // epoch 5
		}

		result, err := indexInputsIntoEpochs(epochLength, nil, inputs, 59)
		require.NoError(t, err)
		require.Len(t, result, 2) // only epochs with inputs + closed prev

		epochs, epochInputs := collectEpochs(result)

		// Epoch 0: closed when input at block 55 arrived
		require.Equal(t, uint64(0), epochs[0].Index)
		require.Equal(t, EpochStatus_Closed, epochs[0].Status)
		require.Equal(t, uint64(1), epochs[0].InputIndexUpperBound) // next input index
		require.Len(t, epochInputs[0], 1)

		// Epoch 5: closed by mostRecentBlockNumber (59 >= 59)
		require.Equal(t, uint64(5), epochs[1].Index)
		require.Equal(t, EpochStatus_Closed, epochs[1].Status)
		require.Equal(t, uint64(50), epochs[1].FirstBlock)
		require.Equal(t, uint64(59), epochs[1].LastBlock)
		require.Len(t, epochInputs[1], 1)
	})
}

func TestCalculateEpochIndex(t *testing.T) {
	t.Run("BasicCases", func(t *testing.T) {
		require.Equal(t, uint64(0), calculateEpochIndex(10, 0))
		require.Equal(t, uint64(0), calculateEpochIndex(10, 9))
		require.Equal(t, uint64(1), calculateEpochIndex(10, 10))
		require.Equal(t, uint64(1), calculateEpochIndex(10, 19))
		require.Equal(t, uint64(2), calculateEpochIndex(10, 20))
	})

	t.Run("EpochLengthOne", func(t *testing.T) {
		require.Equal(t, uint64(0), calculateEpochIndex(1, 0))
		require.Equal(t, uint64(42), calculateEpochIndex(1, 42))
	})

	t.Run("LargeEpochLength", func(t *testing.T) {
		require.Equal(t, uint64(0), calculateEpochIndex(1000, 999))
		require.Equal(t, uint64(1), calculateEpochIndex(1000, 1000))
	})
}

func TestCalculateEpochIndexProperty(t *testing.T) {
	t.Run("BlockBelongsToExactlyOneEpoch", func(t *testing.T) {
		f := func(epochLength uint64, blockNumber uint64) bool {
			if epochLength == 0 {
				return true // skip division by zero
			}
			idx := calculateEpochIndex(epochLength, blockNumber)
			firstBlock := idx * epochLength
			lastBlock := firstBlock + epochLength - 1

			// Guard against overflow
			if lastBlock < firstBlock {
				return true // overflow, skip
			}

			return blockNumber >= firstBlock && blockNumber <= lastBlock
		}
		require.NoError(t, quick.Check(f, nil))
	})

	t.Run("ConsecutiveBlocksNeverSkipEpochs", func(t *testing.T) {
		f := func(epochLength uint64, blockNumber uint64) bool {
			if epochLength == 0 || blockNumber == math.MaxUint64 {
				return true
			}
			idx1 := calculateEpochIndex(epochLength, blockNumber)
			idx2 := calculateEpochIndex(epochLength, blockNumber+1)
			// Consecutive blocks are either in the same epoch or adjacent epochs
			return idx2 == idx1 || idx2 == idx1+1
		}
		require.NoError(t, quick.Check(f, nil))
	})
}

func TestIndexInputsIntoEpochsProperty(t *testing.T) {
	t.Run("MonotonicInputsProduceContiguousNonOverlappingEpochs", func(t *testing.T) {
		const epochLength uint64 = 10

		f := func(numInputs uint8) bool {
			n := int(numInputs) % 20 // limit to 0-19 inputs
			if n == 0 {
				return true
			}

			inputs := make([]*Input, n)
			for i := range n {
				inputs[i] = makeInput(uint64(i), uint64(i*3+1)) // monotonic blocks: 1,4,7,10,...
			}
			mostRecent := inputs[n-1].BlockNumber + epochLength // ensure past last epoch

			result, err := indexInputsIntoEpochs(epochLength, nil, inputs, mostRecent)
			if err != nil {
				return false
			}

			epochs, epochInputs := collectEpochs(result)

			// 1. Every input must be assigned to exactly one epoch
			totalInputs := 0
			for _, ins := range epochInputs {
				totalInputs += len(ins)
			}
			if totalInputs != n {
				return false
			}

			// 2. Epochs must be non-overlapping (sorted by index, no duplicates)
			for i := 1; i < len(epochs); i++ {
				if epochs[i].Index <= epochs[i-1].Index {
					return false
				}
			}

			// 3. Each input's block must fall within its epoch's block range
			for i, ep := range epochs {
				for _, inp := range epochInputs[i] {
					if inp.BlockNumber < ep.FirstBlock || inp.BlockNumber > ep.LastBlock {
						return false
					}
				}
			}

			// 4. Input indices within each epoch must be monotonically increasing
			for _, ins := range epochInputs {
				for j := 1; j < len(ins); j++ {
					if ins[j].Index <= ins[j-1].Index {
						return false
					}
				}
			}

			return true
		}
		require.NoError(t, quick.Check(f, nil))
	})
}

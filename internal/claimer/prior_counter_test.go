// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeStepCounter returns an oracle whose value at block b is the count of
// transitions in `transitions` that are <= b. The transition list must be
// sorted ascending. This is a faithful stand-in for the on-chain
// GetNumberOfSubmittedClaims / GetNumberOfAcceptedClaims counters: monotonic,
// integer, increments at the event block.
func makeStepCounter(transitions []uint64, calls *[]uint64) ethutil.TransitionQueryFn {
	return func(_ context.Context, block uint64) (*big.Int, error) {
		if calls != nil {
			*calls = append(*calls, block)
		}
		var n int64
		for _, t := range transitions {
			if t <= block {
				n++
			}
		}
		return big.NewInt(n), nil
	}
}

func TestPriorCounter_QueriesFromBlockMinusOne(t *testing.T) {
	// fromBlock = 70 should hit oracle exactly once, at block 69. Counter
	// at block 69 is 0 (the only acceptance fires at block 80), so
	// priorCounter must return *big.Int(0), not nil and not the value
	// at block 70 itself (which is also 0 here, indistinguishable) and
	// definitely not the value at epoch.LastBlock=90 (which is 1).
	calls := []uint64{}
	oracle := makeStepCounter([]uint64{80}, &calls)

	got, err := priorCounter(context.Background(), oracle, 70)
	require.NoError(t, err)
	require.NotNil(t, got, "priorCounter must return a value (non-nil *big.Int) when fromBlock > 0")
	assert.Equal(t, int64(0), got.Int64())
	require.Len(t, calls, 1, "priorCounter must make exactly one oracle call")
	assert.Equal(t, uint64(69), calls[0],
		"priorCounter must query block fromBlock-1, not fromBlock and not epoch.LastBlock")
}

func TestPriorCounter_FromBlockOne(t *testing.T) {
	// fromBlock = 1 is the smallest non-zero value; oracle must be called
	// at block 0 (not block 1, not block -1).
	calls := []uint64{}
	oracle := makeStepCounter([]uint64{0}, &calls)

	got, err := priorCounter(context.Background(), oracle, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	// Counter at block 0 with a transition AT block 0 is 1 (the step is
	// "count of transitions <= block"). This pins that priorCounter does
	// NOT off-by-one in the other direction (block 0 - 1 wrap-around).
	assert.Equal(t, int64(1), got.Int64(),
		"priorCounter(1) must query oracle(0); a counter that fired at block 0 must be visible")
	require.Len(t, calls, 1)
	assert.Equal(t, uint64(0), calls[0])
}

func TestPriorCounter_FromBlockZero(t *testing.T) {
	// fromBlock = 0 has no "block before"; querying oracle(uint64(0)-1)
	// would wrap to math.MaxUint64 and either error at the RPC layer or
	// return a misleading head-of-chain counter. priorCounter must
	// short-circuit and return (nil, nil) without calling the oracle.
	calls := []uint64{}
	oracle := makeStepCounter([]uint64{0, 5, 10}, &calls)

	got, err := priorCounter(context.Background(), oracle, 0)
	require.NoError(t, err)
	assert.Nil(t, got,
		"priorCounter(fromBlock=0) must return nil (signaling FindTransitions to skip the boundary monotonic check)")
	assert.Empty(t, calls,
		"priorCounter(fromBlock=0) must NOT call the oracle — there is no fromBlock-1 to query")
}

func TestPriorCounter_PropagatesOracleError(t *testing.T) {
	// Oracle errors must surface unchanged so the caller can fail the
	// claim cycle rather than silently treat a transient RPC failure as
	// "no prior counter".
	sentinel := errors.New("rpc unavailable")
	oracle := func(_ context.Context, _ uint64) (*big.Int, error) {
		return nil, sentinel
	}

	got, err := priorCounter(context.Background(), oracle, 70)
	assert.Nil(t, got)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestFindTransitions_PrevValueRegression pins the prevValue contract of
// ethutil.FindTransitions: the caller must pass oracle(fromBlock-1), not
// oracle(epoch.LastBlock). Using the counter at any block past the scan
// window violates FindTransitions' monotonic invariant
// (prevValue <= oracle(fromBlock)) as soon as a transition fires inside
// the window, aborting the whole scan.
//
// Setup mirrors the multi-epoch foreclosure-replay scenario:
//
//	fromBlock         = 70   (prevEpoch.LastBlock + 1; scan starts here)
//	currEpoch.LastBlock = 90
//	transitions at blocks 75, 85         (two acceptance events inside [70, 90])
//	oracle(69) = 0    (priorCounter — correct prevValue)
//	oracle(70) = 0    (startValue — same block the scan begins from)
//	oracle(90) = 2    (the buggy prevValue: counter at currEpoch.LastBlock)
//
// With prevValue = 2 (the bug) FindTransitions returns
// "monotonic assumption violated: prevValue 2 > startValue 0 at block 70"
// and never scans the interior. With prevValue = priorCounter(...) = 0 the
// scan completes and surfaces both transition blocks in chronological order.
func TestFindTransitions_PrevValueRegression(t *testing.T) {
	ctx := context.Background()
	const (
		fromBlock        uint64 = 70
		currEpochLastBlk uint64 = 90
	)
	transitions := []uint64{75, 85}

	t.Run("BuggyOracleAtEpochLastBlockTripsMonotonicCheck", func(t *testing.T) {
		oracle := makeStepCounter(transitions, nil)

		// The buggy pattern: pass the counter at the CURRENT epoch's
		// LastBlock as prevValue. This is "the counter at some unrelated
		// block" — specifically a block past several in-scan-window
		// transitions.
		buggyPrevValue, err := oracle(ctx, currEpochLastBlk)
		require.NoError(t, err)
		require.Equal(t, int64(2), buggyPrevValue.Int64(),
			"sanity: oracle(currEpoch.LastBlock=90) must observe both transitions")

		hits := []uint64{}
		onHit := func(block uint64) error {
			hits = append(hits, block)
			return nil
		}

		_, err = ethutil.FindTransitions(ctx, fromBlock, currEpochLastBlk,
			buggyPrevValue, oracle, onHit)
		require.Error(t, err, "the buggy prevValue MUST trip the monotonic-assumption check")
		assert.Contains(t, err.Error(), "monotonic assumption violated",
			"the specific error string is the reason this bug went undetected for so long; pin it")
		assert.Empty(t, hits,
			"on monotonic violation the scan aborts before any interior split; no onHit call must fire")
	})

	t.Run("PriorCounterFixCompletesScan", func(t *testing.T) {
		oracle := makeStepCounter(transitions, nil)

		fixedPrevValue, err := priorCounter(ctx, oracle, fromBlock)
		require.NoError(t, err)
		require.NotNil(t, fixedPrevValue)
		require.Equal(t, int64(0), fixedPrevValue.Int64(),
			"sanity: priorCounter at fromBlock=70 must read oracle(69)=0 (no transitions yet)")

		hits := []uint64{}
		onHit := func(block uint64) error {
			hits = append(hits, block)
			return nil
		}

		count, err := ethutil.FindTransitions(ctx, fromBlock, currEpochLastBlk,
			fixedPrevValue, oracle, onHit)
		require.NoError(t, err, "priorCounter MUST satisfy FindTransitions' monotonic invariant")
		assert.Equal(t, uint64(len(transitions)), count)
		assert.Equal(t, transitions, hits,
			"every transition block in [fromBlock, currEpoch.LastBlock] must be reported in chronological order")
	})
}

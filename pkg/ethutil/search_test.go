// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/suite"
)

type SearchSuite struct {
	suite.Suite
	ctx context.Context
}

func TestSearchSuite(t *testing.T) {
	suite.Run(t, new(SearchSuite))
}

func (s *SearchSuite) SetupTest() {
	s.ctx = context.Background()
}

// Helper to create a mock transitionQuery from a map
func mockTransitionQuery(values map[uint64]*big.Int) TransitionQueryFn {
	return func(_ context.Context, block uint64) (*big.Int, error) {
		if val, ok := values[block]; ok {
			return val, nil
		}
		return big.NewInt(0), nil // default
	}
}

// Helper to create an onHit that collects blocks
func collectOnHit(blocks *[]uint64) OnHitFn {
	return func(block uint64, _, _ *big.Int) error {
		*blocks = append(*blocks, block)
		return nil
	}
}

// Helper to create an onHit that returns error
func errorOnHit(err error) OnHitFn {
	return func(_ uint64, _, _ *big.Int) error {
		return err
	}
}

// Helper to create a transitionQuery that returns error
func errorTransitionQuery(err error) TransitionQueryFn {
	return func(_ context.Context, _ uint64) (*big.Int, error) {
		return nil, err
	}
}

func (s *SearchSuite) TestNoTransitionsWhenValuesEqual() {
	values := map[uint64]*big.Int{
		0:  big.NewInt(1),
		10: big.NewInt(1),
	}
	var blocks []uint64
	_, _, err := FindTransitions(s.ctx, 0, 10, nil, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Empty(blocks)
}

func (s *SearchSuite) TestSingleTransition() {
	values := map[uint64]*big.Int{
		0: big.NewInt(1),
		1: big.NewInt(2),
	}
	var blocks []uint64
	_, _, err := FindTransitions(s.ctx, 0, 1, nil, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal([]uint64{1}, blocks)
}

func (s *SearchSuite) TestMultipleTransitions() {
	values := map[uint64]*big.Int{
		1: big.NewInt(1),
		2: big.NewInt(1),
		3: big.NewInt(2),
		4: big.NewInt(2),
		5: big.NewInt(3),
		6: big.NewInt(3),
	}
	var blocks []uint64
	count, _, err := FindTransitions(s.ctx, 1, 6, nil, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(2))
	s.Equal([]uint64{3, 5}, blocks)

	blocks = []uint64{}
	previousValue := big.NewInt(0)
	count, _, err = FindTransitions(s.ctx, 1, 6, previousValue, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(3))
	s.Equal([]uint64{1, 3, 5}, blocks)
}

func (s *SearchSuite) TestTransitionAtBoundary() {
	values := map[uint64]*big.Int{
		0: big.NewInt(1),
		1: big.NewInt(2),
	}
	var blocks []uint64
	count, _, err := FindTransitions(s.ctx, 0, 1, nil, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(1))
	s.Equal([]uint64{1}, blocks)

	blocks = []uint64{}
	previousValue := big.NewInt(0)
	count, _, err = FindTransitions(s.ctx, 0, 1, previousValue, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(2))
	s.Equal([]uint64{0, 1}, blocks)

}

func (s *SearchSuite) TestStartEqualsEnd() {
	values := map[uint64]*big.Int{
		5: big.NewInt(1),
	}
	var blocks []uint64
	count, _, err := FindTransitions(s.ctx, 5, 5, nil, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(0))
	s.Empty(blocks)

	blocks = []uint64{}
	previousValue := big.NewInt(0)
	count, _, err = FindTransitions(s.ctx, 5, 5, previousValue, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(1))
	s.Equal([]uint64{5}, blocks)
}

func (s *SearchSuite) TestContextCancellation() {
	values := map[uint64]*big.Int{
		0:  big.NewInt(1),
		10: big.NewInt(2),
	}
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	var blocks []uint64
	_, _, err := FindTransitions(ctx, 0, 10, nil, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.Error(err)
	s.Equal(context.Canceled, err)
}

func (s *SearchSuite) TestTransitionQueryErrorAtStart() {
	var blocks []uint64
	_, _, err := FindTransitions(s.ctx, 0, 10, nil, nil, errorTransitionQuery(fmt.Errorf("query error")), collectOnHit(&blocks))
	s.Error(err)
	s.Contains(err.Error(), "transitionQuery(startBlock=0): query error")
	s.Empty(blocks)
}

func (s *SearchSuite) TestTransitionQueryErrorAtEnd() {
	values := map[uint64]*big.Int{
		0: big.NewInt(1),
	}
	var blocks []uint64
	_, _, err := FindTransitions(s.ctx, 0, 10, nil, nil, func(_ context.Context, block uint64) (*big.Int, error) {
		if block == 10 {
			return nil, fmt.Errorf("query error at end")
		}
		return values[block], nil
	}, collectOnHit(&blocks))
	s.Error(err)
	s.Contains(err.Error(), "transitionQuery(endBlock=10): query error at end")
	s.Empty(blocks)
}

func (s *SearchSuite) TestTransitionQueryErrorAtMid() {
	values := map[uint64]*big.Int{
		0:  big.NewInt(1),
		10: big.NewInt(2),
	}
	var blocks []uint64
	_, _, err := FindTransitions(s.ctx, 0, 10, nil, nil, func(_ context.Context, block uint64) (*big.Int, error) {
		if block == 5 {
			return nil, fmt.Errorf("query error at mid")
		}
		if val, ok := values[block]; ok {
			return val, nil
		}
		return big.NewInt(0), nil
	}, collectOnHit(&blocks))
	s.Error(err)
	s.Contains(err.Error(), "transitionQuery(midBlock=5): query error at mid")
	s.Empty(blocks)
}

func (s *SearchSuite) TestOnHitError() {
	values := map[uint64]*big.Int{
		0: big.NewInt(1),
		1: big.NewInt(2),
	}
	var blocks []uint64
	_, _, err := FindTransitions(s.ctx, 0, 1, nil, nil, mockTransitionQuery(values), errorOnHit(fmt.Errorf("onHit error")))
	s.Error(err)
	s.Contains(err.Error(), "onHit error")
	s.Empty(blocks)
}

func (s *SearchSuite) TestLargeRange() {
	values := make(map[uint64]*big.Int)
	for i := uint64(0); i <= 100; i++ {
		if i < 50 {
			values[i] = big.NewInt(1)
		} else {
			values[i] = big.NewInt(2)
		}
	}
	var blocks []uint64
	_, _, err := FindTransitions(s.ctx, 0, 100, nil, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal([]uint64{50}, blocks)
}

func (s *SearchSuite) TestNoValuesDefined() {
	var blocks []uint64
	_, _, err := FindTransitions(s.ctx, 0, 10, nil, nil, mockTransitionQuery(map[uint64]*big.Int{}), collectOnHit(&blocks))
	s.NoError(err)
	s.Empty(blocks)
}

func (s *SearchSuite) TestMonotonicViolation() {
	values := map[uint64]*big.Int{
		0: big.NewInt(2),
		1: big.NewInt(1),
	}
	var blocks []uint64
	previousValue := big.NewInt(3) // 3 > 2, violation
	_, _, err := FindTransitions(s.ctx, 0, 1, previousValue, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.Error(err)
	s.ErrorIs(err, ErrMonotonicViolation)
	s.Contains(err.Error(), "monotonic assumption violated")
	s.Empty(blocks)
}

func (s *SearchSuite) TestUpperBoundStopsAtReachingTransition() {
	// Counters can jump by more than one in a block. The block reaching or
	// crossing the bound must be processed, but no later transition may be.
	values := map[uint64]*big.Int{
		10: big.NewInt(2), 11: big.NewInt(2),
		12: big.NewInt(5), 13: big.NewInt(5),
		14: big.NewInt(9), 15: big.NewInt(9),
		16: big.NewInt(12), 17: big.NewInt(12),
	}
	for _, test := range []struct {
		name  string
		lower *big.Int
		upper *big.Int
		want  []uint64
		last  uint64
	}{
		{name: "unbounded", want: []uint64{12, 14, 16}, last: 17},
		{name: "exact counter", upper: big.NewInt(5), want: []uint64{12}, last: 13},
		{name: "counter jumps past bound", upper: big.NewInt(6), want: []uint64{12, 14}, last: 15},
		{name: "bound above final counter", upper: big.NewInt(20), want: []uint64{12, 14, 16}, last: 17},
		{name: "bound already reached at start", upper: big.NewInt(2), last: 10},
		{name: "bound already exceeded at start", upper: big.NewInt(1), last: 10},
		{name: "initial transition reaches bound", lower: big.NewInt(0), upper: big.NewInt(2), want: []uint64{10}, last: 10},
		{name: "initial and later transitions", lower: big.NewInt(0), upper: big.NewInt(9), want: []uint64{10, 12, 14}, last: 15},
	} {
		s.Run(test.name, func() {
			var blocks []uint64
			count, last, err := FindTransitions(s.ctx, 10, 17, test.lower, test.upper,
				mockTransitionQuery(values), collectOnHit(&blocks))
			s.Require().NoError(err)
			s.Equal(test.want, blocks)
			s.Equal(uint64(len(test.want)), count)
			// Progress includes the unchanged blocks following a transition.
			s.Equal(test.last, last)
		})
	}
}

func (s *SearchSuite) TestLastProcessedBlockOnEmptyAndSingleBlockRanges() {
	for _, test := range []struct {
		name       string
		start, end uint64
		lower      *big.Int
		want       []uint64
		last       uint64
	}{
		{name: "empty range", start: 11, end: 10, last: 10},
		{name: "single block without transition", start: 10, end: 10, last: 10},
		{name: "single block with transition", start: 10, end: 10, lower: big.NewInt(0), want: []uint64{10}, last: 10},
	} {
		s.Run(test.name, func() {
			var blocks []uint64
			count, last, err := FindTransitions(s.ctx, test.start, test.end, test.lower, nil,
				mockTransitionQuery(map[uint64]*big.Int{10: big.NewInt(1)}), collectOnHit(&blocks))
			s.Require().NoError(err)
			s.Equal(test.want, blocks)
			s.Equal(uint64(len(test.want)), count)
			s.Equal(test.last, last)
		})
	}
}

func (s *SearchSuite) TestLastProcessedBlockExcludesFailedCallback() {
	// Include unchanged blocks to verify that progress can extend beyond the
	// last successful callback, while excluding the failed transition.
	for _, test := range []struct {
		failBlock uint64
		last      uint64
		completed []uint64
	}{
		{failBlock: 10, last: 9},
		{failBlock: 12, last: 10, completed: []uint64{10}},
		{failBlock: 14, last: 13, completed: []uint64{10, 12}},
		{failBlock: 16, last: 15, completed: []uint64{10, 12, 14}},
	} {
		failBlock := test.failBlock
		s.Run(fmt.Sprintf("failure_at_%d", failBlock), func() {
			callbackErr := fmt.Errorf("callback failed at %d", failBlock)
			var completed []uint64
			count, last, err := FindTransitions(s.ctx, 10, 17, big.NewInt(0), nil,
				func(_ context.Context, block uint64) (*big.Int, error) {
					return new(big.Int).SetUint64((block-10)/2 + 1), nil
				}, func(block uint64, _, _ *big.Int) error {
					if block == failBlock {
						return callbackErr
					}
					completed = append(completed, block)
					return nil
				})
			s.ErrorIs(err, callbackErr)
			s.Equal(test.last, last)
			s.Equal(test.completed, completed)
			s.Equal(uint64(len(completed)), count)
		})
	}
}

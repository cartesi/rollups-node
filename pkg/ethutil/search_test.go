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
	return func(block uint64) error {
		*blocks = append(*blocks, block)
		return nil
	}
}

// Helper to create an onHit that returns error
func errorOnHit(err error) OnHitFn {
	return func(_ uint64) error {
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
	_, err := FindTransitions(s.ctx, 0, 10, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Empty(blocks)
}

func (s *SearchSuite) TestSingleTransition() {
	values := map[uint64]*big.Int{
		0: big.NewInt(1),
		1: big.NewInt(2),
	}
	var blocks []uint64
	_, err := FindTransitions(s.ctx, 0, 1, nil, mockTransitionQuery(values), collectOnHit(&blocks))
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
	count, err := FindTransitions(s.ctx, 1, 6, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(2))
	s.Equal([]uint64{3, 5}, blocks)

	blocks = []uint64{}
	previousValue := big.NewInt(0)
	count, err = FindTransitions(s.ctx, 1, 6, previousValue, mockTransitionQuery(values), collectOnHit(&blocks))
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
	count, err := FindTransitions(s.ctx, 0, 1, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(1))
	s.Equal([]uint64{1}, blocks)

	blocks = []uint64{}
	previousValue := big.NewInt(0)
	count, err = FindTransitions(s.ctx, 0, 1, previousValue, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(2))
	s.Equal([]uint64{0, 1}, blocks)

}

func (s *SearchSuite) TestStartEqualsEnd() {
	values := map[uint64]*big.Int{
		5: big.NewInt(1),
	}
	var blocks []uint64
	count, err := FindTransitions(s.ctx, 5, 5, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal(count, uint64(0))
	s.Empty(blocks)

	blocks = []uint64{}
	previousValue := big.NewInt(0)
	count, err = FindTransitions(s.ctx, 5, 5, previousValue, mockTransitionQuery(values), collectOnHit(&blocks))
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
	_, err := FindTransitions(ctx, 0, 10, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.Error(err)
	s.Equal(context.Canceled, err)
}

func (s *SearchSuite) TestTransitionQueryErrorAtStart() {
	var blocks []uint64
	_, err := FindTransitions(s.ctx, 0, 10, nil, errorTransitionQuery(fmt.Errorf("query error")), collectOnHit(&blocks))
	s.Error(err)
	s.Contains(err.Error(), "transitionQuery(startBlock=0): query error")
	s.Empty(blocks)
}

func (s *SearchSuite) TestTransitionQueryErrorAtEnd() {
	values := map[uint64]*big.Int{
		0: big.NewInt(1),
	}
	var blocks []uint64
	_, err := FindTransitions(s.ctx, 0, 10, nil, func(_ context.Context, block uint64) (*big.Int, error) {
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
	_, err := FindTransitions(s.ctx, 0, 10, nil, func(_ context.Context, block uint64) (*big.Int, error) {
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
	_, err := FindTransitions(s.ctx, 0, 1, nil, mockTransitionQuery(values), errorOnHit(fmt.Errorf("onHit error")))
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
	_, err := FindTransitions(s.ctx, 0, 100, nil, mockTransitionQuery(values), collectOnHit(&blocks))
	s.NoError(err)
	s.Equal([]uint64{50}, blocks)
}

func (s *SearchSuite) TestNoValuesDefined() {
	var blocks []uint64
	_, err := FindTransitions(s.ctx, 0, 10, nil, mockTransitionQuery(map[uint64]*big.Int{}), collectOnHit(&blocks))
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
	_, err := FindTransitions(s.ctx, 0, 1, previousValue, mockTransitionQuery(values), collectOnHit(&blocks))
	s.Error(err)
	s.ErrorIs(err, ErrMonotonicViolation)
	s.Contains(err.Error(), "monotonic assumption violated")
	s.Empty(blocks)
}

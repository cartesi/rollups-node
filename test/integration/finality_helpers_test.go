// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

// confirmedBlockAtLeast waits for fixture foreclosure at the observation policy.
// A latest-mode test must not wait for a separate finalized head.
func confirmedBlockAtLeast(
	ctx context.Context, headerByNumber func(context.Context, *big.Int) (*types.Header, error),
	policy model.DefaultBlock, block uint64,
) (bool, error) {
	var tag *big.Int
	switch policy {
	case model.DefaultBlock_Latest:
	case model.DefaultBlock_Safe:
		tag = big.NewInt(rpc.SafeBlockNumber.Int64())
	case model.DefaultBlock_Finalized:
		tag = big.NewInt(rpc.FinalizedBlockNumber.Int64())
	case model.DefaultBlock_Pending:
		return false, fmt.Errorf("unsupported confirmation policy %s", policy)
	default:
		return false, fmt.Errorf("unsupported confirmation policy %s", policy)
	}
	header, err := headerByNumber(ctx, tag)
	if err != nil {
		return false, fmt.Errorf("read %s block: %w", policy, err)
	}
	if header == nil || header.Number == nil || !header.Number.IsUint64() {
		return false, fmt.Errorf("%s head has no valid block number", policy)
	}
	return header.Number.Uint64() >= block, nil
}

func testConfirmedBlockAtLeastUsesSelectedPolicy(t *testing.T) {
	const foreclosureBlock uint64 = 100
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe, model.DefaultBlock_Finalized} {
		for _, confirmed := range []uint64{foreclosureBlock - 1, foreclosureBlock} {
			t.Run(fmt.Sprintf("%s-%d", policy, confirmed), func(t *testing.T) {
				read := func(_ context.Context, number *big.Int) (*types.Header, error) {
					switch policy {
					case model.DefaultBlock_Latest:
						require.Nil(t, number)
					case model.DefaultBlock_Safe:
						require.Equal(t, big.NewInt(rpc.SafeBlockNumber.Int64()), number)
					case model.DefaultBlock_Finalized:
						require.Equal(t, big.NewInt(rpc.FinalizedBlockNumber.Int64()), number)
					case model.DefaultBlock_Pending:
						t.Fatal("pending is not a confirmation policy")
					}
					return &types.Header{Number: new(big.Int).SetUint64(confirmed)}, nil
				}
				ready, err := confirmedBlockAtLeast(t.Context(), read, policy, foreclosureBlock)
				require.NoError(t, err)
				require.Equal(t, confirmed == foreclosureBlock, ready)
			})
		}
	}
}

func testConfirmedBlockAtLeastCannotProceedAfterAReadFailure(t *testing.T) {
	cause := errors.New("confirmation read failed")
	read := func(context.Context, *big.Int) (*types.Header, error) { return nil, cause }
	ready, err := confirmedBlockAtLeast(t.Context(), read, model.DefaultBlock_Finalized, 100)
	require.False(t, ready)
	require.ErrorIs(t, err, cause)
}

// fixtureFinalityReader separates bounded fixture progress from a live RPC.
type fixtureFinalityReader interface {
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	BlockNumber(context.Context) (uint64, error)
	BlockByNumber(context.Context, *big.Int) (*types.Block, error)
}

func advanceFixtureFinalityWith(
	ctx context.Context,
	client fixtureFinalityReader,
	policy model.DefaultBlock,
	mine func(context.Context, int) error,
) (uint64, error) {
	const maxFinalityBlocks = 128
	var tag rpc.BlockNumber
	switch policy {
	case model.DefaultBlock_Latest:
		return client.BlockNumber(ctx)
	case model.DefaultBlock_Finalized:
		tag = rpc.FinalizedBlockNumber
	case model.DefaultBlock_Safe:
		tag = rpc.SafeBlockNumber
	case model.DefaultBlock_Pending:
		return 0, fmt.Errorf("pending is not a fixture observation policy")
	default:
		return 0, fmt.Errorf("unsupported fixture block policy %s", policy)
	}
	selected, err := client.HeaderByNumber(ctx, big.NewInt(tag.Int64()))
	if err != nil {
		return 0, fmt.Errorf("read fixture %s head: %w", policy, err)
	}
	if selected == nil || selected.Number == nil || !selected.Number.IsUint64() {
		return 0, fmt.Errorf("fixture %s head has no valid block number", policy)
	}
	head, err := client.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	observed := selected.Number.Uint64()
	if head < observed || head-observed > maxFinalityBlocks {
		return 0, fmt.Errorf("fixture head %d and %s head %d exceed bounded finality gap", head, policy, observed)
	}
	target := observed
	for height := head; height > observed; height-- {
		block, readErr := client.BlockByNumber(ctx, new(big.Int).SetUint64(height))
		if readErr != nil {
			return 0, readErr
		}
		if block == nil {
			return 0, fmt.Errorf("fixture block %d is missing", height)
		}
		if len(block.Transactions()) > 0 {
			target = height
			break
		}
	}
	for mined := 0; observed < target; mined++ {
		if mined == maxFinalityBlocks {
			return 0, fmt.Errorf("fixture %s head did not reach transaction block %d", policy, target)
		}
		if err = mine(ctx, 1); err != nil {
			return 0, err
		}
		selected, err = client.HeaderByNumber(ctx, big.NewInt(tag.Int64()))
		if err != nil {
			return 0, err
		}
		if selected == nil || selected.Number == nil || !selected.Number.IsUint64() {
			return 0, fmt.Errorf("fixture %s head has no valid block number", policy)
		}
		observed = selected.Number.Uint64()
	}
	return observed, nil
}

type fixtureFinalityChain struct {
	head             uint64
	lag              uint64
	transactionBlock uint64
	mined            uint64
	selectedTags     []int64
}

func (chain *fixtureFinalityChain) HeaderByNumber(_ context.Context, tag *big.Int) (*types.Header, error) {
	chain.selectedTags = append(chain.selectedTags, tag.Int64())
	return &types.Header{Number: new(big.Int).SetUint64(chain.head - chain.lag)}, nil
}

func (chain *fixtureFinalityChain) BlockNumber(context.Context) (uint64, error) {
	return chain.head, nil
}

func (chain *fixtureFinalityChain) BlockByNumber(_ context.Context, height *big.Int) (*types.Block, error) {
	block := types.NewBlockWithHeader(&types.Header{Number: height})
	if height.Uint64() == chain.transactionBlock {
		block = block.WithBody(types.Body{Transactions: []*types.Transaction{types.NewTx(&types.LegacyTx{})}})
	}
	return block, nil
}

func (chain *fixtureFinalityChain) mine(_ context.Context, count int) error {
	for range count {
		chain.head++
		chain.mined++
	}
	return nil
}

func testFixtureFinalitySuppliesLongLagBeforeObservation(t *testing.T) {
	for _, test := range []struct {
		policy model.DefaultBlock
		lag    uint64
		tag    rpc.BlockNumber
	}{
		{model.DefaultBlock_Finalized, 64, rpc.FinalizedBlockNumber},
		{model.DefaultBlock_Safe, 32, rpc.SafeBlockNumber},
	} {
		t.Run(test.policy.String(), func(t *testing.T) {
			chain := &fixtureFinalityChain{head: 100, lag: test.lag, transactionBlock: 100}
			observed, err := advanceFixtureFinalityWith(t.Context(), chain, test.policy, chain.mine)
			require.NoError(t, err)
			require.Equal(t, uint64(100), observed, "the mined foreclosure must be observable before its short CLI poll")
			require.Equal(t, test.lag, chain.mined, "supply only the missing empty successors")
			for _, tag := range chain.selectedTags {
				require.Equal(t, test.tag.Int64(), tag, "never substitute latest for the selected observation policy")
			}
		})
	}
}

func testFixtureFinalityBoundsProgressAndReadFailures(t *testing.T) {
	t.Run("latest needs no successors", func(t *testing.T) {
		chain := &fixtureFinalityChain{head: 100, lag: 64, transactionBlock: 100}
		observed, err := advanceFixtureFinalityWith(t.Context(), chain, model.DefaultBlock_Latest, chain.mine)
		require.NoError(t, err)
		require.Equal(t, uint64(100), observed)
		require.Zero(t, chain.mined)
		require.Empty(t, chain.selectedTags)
	})
	t.Run("already observable needs no successors", func(t *testing.T) {
		chain := &fixtureFinalityChain{head: 100, lag: 64, transactionBlock: 36}
		observed, err := advanceFixtureFinalityWith(t.Context(), chain, model.DefaultBlock_Finalized, chain.mine)
		require.NoError(t, err)
		require.Equal(t, uint64(36), observed)
		require.Zero(t, chain.mined)
	})
	t.Run("unsupported gap stays bounded", func(t *testing.T) {
		chain := &fixtureFinalityChain{head: 200, lag: 129, transactionBlock: 200}
		_, err := advanceFixtureFinalityWith(t.Context(), chain, model.DefaultBlock_Finalized, chain.mine)
		require.ErrorContains(t, err, "bounded finality gap")
		require.Zero(t, chain.mined)
	})
	t.Run("mining error stops progress", func(t *testing.T) {
		chain := &fixtureFinalityChain{head: 100, lag: 64, transactionBlock: 100}
		cause := errors.New("private fixture mining failed")
		_, err := advanceFixtureFinalityWith(t.Context(), chain, model.DefaultBlock_Finalized,
			func(context.Context, int) error { return cause })
		require.ErrorIs(t, err, cause)
		require.Zero(t, chain.mined)
	})
}

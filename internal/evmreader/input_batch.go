// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// Exhausting the tick budget is successful catch-up work, not a scan failure.
var errInputBudgetExhausted = errors.New("input budget exhausted")

// fetchInputBatch uses indexed topics to bound the RPC response even when a
// single block contains more inputs than the budget. The repository count is
// the durable index cursor; the block cursor covers only complete blocks.
func (r *Service) fetchInputBatch(
	ctx context.Context,
	app appContracts,
	startBlock, endBlock, first, upper uint64) ([]*Input, uint64, uint64, error) {
	limit := r.maxInputsPerStep
	if r.inputBudget != nil {
		limit = *r.inputBudget
	}
	if limit == 0 {
		return nil, first, startBlock - 1, errInputBudgetExhausted
	}
	count, err := app.inputSource.GetNumberOfInputs(&bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(endBlock),
	}, app.application.IApplicationAddress)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("get input count at block %d: %w", endBlock, err)
	}
	if count == nil || !count.IsUint64() || count.Uint64() < first {
		return nil, 0, 0, fmt.Errorf("invalid input count at block %d: %v (stored %d)", endBlock, count, first)
	}
	if count.Uint64() < upper && upper != ^uint64(0) {
		return nil, 0, 0, fmt.Errorf("sealed input range exceeds on-chain count: upper %d, count %d", upper, count.Uint64())
	}
	target := min(count.Uint64(), upper)
	if target < first {
		return nil, 0, 0, fmt.Errorf("input range ends before stored count")
	}
	next := first + min(limit, target-first)
	indices := make([]*big.Int, 0, next-first)
	for i := first; i < next; i++ {
		indices = append(indices, new(big.Int).SetUint64(i))
	}
	var inputs []*Input
	if len(indices) > 0 {
		events, err := app.inputSource.RetrieveInputs(&bind.FilterOpts{
			Context: ctx,
			Start:   startBlock,
			End:     &endBlock,
		}, []common.Address{app.application.IApplicationAddress}, indices)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("retrieve input batch: %w", err)
		}
		for _, event := range events {
			if event.Index == nil || !event.Index.IsUint64() || event.Index.Uint64() < first || event.Index.Uint64() >= next || event.Raw.BlockNumber < startBlock || event.Raw.BlockNumber > endBlock {
				return nil, 0, 0, fmt.Errorf("input log outside requested batch")
			}
			input := &Input{Index: event.Index.Uint64(), Status: InputCompletionStatus_None, RawData: event.Input, BlockNumber: event.Raw.BlockNumber, TransactionHash: event.Raw.TxHash, LogIndex: uint64(event.Raw.Index)}
			var duplicate bool
			inputs, duplicate = insertSorted(sortByInputIndex, inputs, input)
			if duplicate {
				return nil, 0, 0, fmt.Errorf("duplicate input index %d", input.Index)
			}
		}
		if uint64(len(inputs)) != next-first {
			return nil, 0, 0, fmt.Errorf("input count mismatch: expected %d, got %d", next-first, len(inputs))
		}
		for i, input := range inputs {
			if input.Index != first+uint64(i) || (i > 0 && input.BlockNumber < inputs[i-1].BlockNumber) {
				return nil, 0, 0, fmt.Errorf("invalid input sequence")
			}
		}
	}
	cursor := endBlock
	if next < target {
		cursor = inputs[len(inputs)-1].BlockNumber - 1
	}
	if r.inputBudget != nil {
		*r.inputBudget -= uint64(len(inputs))
	}
	return inputs, next, cursor, nil
}

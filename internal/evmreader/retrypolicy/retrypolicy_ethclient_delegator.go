// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retrypolicy

import (
	"context"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/services/retry"
	"github.com/ethereum/go-ethereum/core/types"
)

// A EthClient Delegator that
// calls HeaderByNumber with the retry
// policy defined by util.RetryFunction
type EthClientRetryPolicyDelegator struct {
	delegate          evmreader.EthClient
	maxRetries        uint64
	delayBetweenCalls time.Duration
}

func NewEhtClientWithRetryPolicy(
	delegate evmreader.EthClient,
	maxRetries uint64,
	delayBetweenCalls time.Duration,
) *EthClientRetryPolicyDelegator {
	return &EthClientRetryPolicyDelegator{
		delegate:          delegate,
		maxRetries:        maxRetries,
		delayBetweenCalls: delayBetweenCalls,
	}
}

type headerByNumberArgs struct {
	ctx    context.Context
	number *big.Int
}

func (d *EthClientRetryPolicyDelegator) HeaderByNumber(
	ctx context.Context,
	number *big.Int,
) (*types.Header, error) {

	return retry.CallFunctionWithRetryPolicy(d.headerByNumber,
		headerByNumberArgs{
			ctx:    ctx,
			number: number,
		},
		d.maxRetries,
		d.delayBetweenCalls,
		"EthClient::HeaderByNumber",
	)

}

func (d *EthClientRetryPolicyDelegator) headerByNumber(
	args headerByNumberArgs,
) (*types.Header, error) {
	return d.delegate.HeaderByNumber(args.ctx, args.number)
}

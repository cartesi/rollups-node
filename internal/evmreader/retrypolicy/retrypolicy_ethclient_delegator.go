// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retrypolicy

import (
	"context"
	"math/big"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/util/retrypolicy"
	"github.com/ethereum/go-ethereum/core/types"
)

type (
	EthClient = evmreader.EthClient
	Header    = types.Header
	Context   = context.Context
)

// A EthClient Delegator that
// calls HeaderByNumber with the retry
// policy defined by util.RetryFunction
type EthClientRetryPolicyDelegator struct {
	delegate          EthClient
	maxRetries        uint64
	delayBetweenCalls Duration
}

func NewEhtClientWithRetryPolicy(
	delegate EthClient,
	maxRetries uint64,
	delayBetweenCalls Duration,
) *EthClientRetryPolicyDelegator {
	return &EthClientRetryPolicyDelegator{
		delegate:          delegate,
		maxRetries:        maxRetries,
		delayBetweenCalls: delayBetweenCalls,
	}
}

type headerByNumberArgs struct {
	ctx    Context
	number *big.Int
}

func (d *EthClientRetryPolicyDelegator) HeaderByNumber(
	ctx Context,
	number *big.Int,
) (*Header, error) {

	return retrypolicy.CallFunctionWithRetryPolicy(d.headerByNumber,
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
) (*Header, error) {
	return d.delegate.HeaderByNumber(args.ctx, args.number)
}

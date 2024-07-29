// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retrypolicy

import (
	"context"
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/util/retrypolicy"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

type (
	EthWsClient = evmreader.EthWsClient
)

type EthWsClientRetryPolicyDelegator struct {
	delegate          EthWsClient
	maxRetries        uint64
	delayBetweenCalls time.Duration
}

func NewEthWsClientWithRetryPolicy(
	delegate EthWsClient,
	maxRetries uint64,
	delayBetweenCalls time.Duration,
) *EthWsClientRetryPolicyDelegator {
	return &EthWsClientRetryPolicyDelegator{
		delegate:          delegate,
		maxRetries:        maxRetries,
		delayBetweenCalls: delayBetweenCalls,
	}
}

type subscribeNewHeadArgs struct {
	ctx context.Context
	ch  chan<- *types.Header
}

func (d *EthWsClientRetryPolicyDelegator) SubscribeNewHead(
	ctx context.Context,
	ch chan<- *types.Header,
) (ethereum.Subscription, error) {

	return retrypolicy.CallFunctionWithRetryPolicy(
		d.subscribeNewHead,
		subscribeNewHeadArgs{
			ctx: ctx,
			ch:  ch,
		},
		d.maxRetries,
		d.delayBetweenCalls,
		"EthWSClient::SubscribeNewHead",
	)
}

func (d *EthWsClientRetryPolicyDelegator) subscribeNewHead(
	args subscribeNewHeadArgs,
) (ethereum.Subscription, error) {
	return d.delegate.SubscribeNewHead(args.ctx, args.ch)
}
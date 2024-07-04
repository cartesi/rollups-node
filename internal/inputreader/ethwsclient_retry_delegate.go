// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inputreader

import (
	"context"
	"time"

	"github.com/cartesi/rollups-node/internal/util"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

type EthWsClientRetryPolicyDelegator struct {
	delegate          EthWsClient
	maxRetries        uint
	delayBetweenCalls time.Duration
}

func NewEthWsClientWithRetryPolicy(
	delegate EthWsClient,
	maxRetries uint,
	delayBetweenCalls time.Duration,
) *EthWsClientRetryPolicyDelegator {
	return &EthWsClientRetryPolicyDelegator{
		delegate:          delegate,
		maxRetries:        maxRetries,
		delayBetweenCalls: delayBetweenCalls,
	}
}

type subcribeNewHeadArgs struct {
	ctx context.Context
	ch  chan<- *types.Header
}

func (d *EthWsClientRetryPolicyDelegator) SubscribeNewHead(
	ctx context.Context,
	ch chan<- *types.Header,
) (ethereum.Subscription, error) {

	return util.CallFunctionWithRetryPolicy(
		d.subscribeNewHead,
		subcribeNewHeadArgs{
			ctx: ctx,
			ch:  ch,
		},
		d.maxRetries,
		d.delayBetweenCalls,
	)
}

func (d *EthWsClientRetryPolicyDelegator) subscribeNewHead(
	args subcribeNewHeadArgs,
) (ethereum.Subscription, error) {
	return d.delegate.SubscribeNewHead(args.ctx, args.ch)
}

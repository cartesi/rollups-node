// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retrypolicy

import (
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/retry"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

// A Consensus Delegator that
// calls GetEpochLength with the retry
// policy defined by util.RetryFunction
type ConsensusRetryPolicyDelegator struct {
	delegate          evmreader.ConsensusContract
	maxRetries        uint64
	delayBetweenCalls time.Duration
}

func NewConsensusWithRetryPolicy(
	delegate evmreader.ConsensusContract,
	maxRetries uint64,
	delayBetweenCalls time.Duration,
) *ConsensusRetryPolicyDelegator {
	return &ConsensusRetryPolicyDelegator{
		delegate:          delegate,
		maxRetries:        maxRetries,
		delayBetweenCalls: delayBetweenCalls,
	}
}

type getEpochLengthArgs struct {
	opts *bind.CallOpts
}

func (d *ConsensusRetryPolicyDelegator) GetEpochLength(
	opts *bind.CallOpts,
) (*big.Int, error) {

	return retry.CallFunctionWithRetryPolicy(d.getEpochLength,
		getEpochLengthArgs{
			opts: opts,
		},
		d.maxRetries,
		d.delayBetweenCalls,
		"Consensus::GetEpochLength",
	)

}

func (d *ConsensusRetryPolicyDelegator) getEpochLength(
	args getEpochLengthArgs,
) (*big.Int, error) {
	return d.delegate.GetEpochLength(args.opts)
}

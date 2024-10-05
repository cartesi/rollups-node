// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retrypolicy

import (
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/retry"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type ApplicationRetryPolicyDelegator struct {
	delegate          evmreader.ApplicationContract
	maxRetries        uint64
	delayBetweenCalls time.Duration
}

func NewApplicationWithRetryPolicy(
	delegate evmreader.ApplicationContract,
	maxRetries uint64,
	delayBetweenCalls time.Duration,
) *ApplicationRetryPolicyDelegator {
	return &ApplicationRetryPolicyDelegator{
		delegate:          delegate,
		maxRetries:        maxRetries,
		delayBetweenCalls: delayBetweenCalls,
	}
}

func (d *ApplicationRetryPolicyDelegator) GetConsensus(opts *bind.CallOpts,
) (common.Address, error) {
	return retry.CallFunctionWithRetryPolicy(d.delegate.GetConsensus,
		opts,
		d.maxRetries,
		d.delayBetweenCalls,
		"Application::GetConsensus",
	)
}

func (d *ApplicationRetryPolicyDelegator) RetrieveOutputExecutionEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationOutputExecuted, error) {
	return retry.CallFunctionWithRetryPolicy(d.delegate.RetrieveOutputExecutionEvents,
		opts,
		d.maxRetries,
		d.delayBetweenCalls,
		"Application::RetrieveOutputExecutionEvents",
	)
}

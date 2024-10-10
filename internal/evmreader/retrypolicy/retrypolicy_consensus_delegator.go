// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retrypolicy

import (
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/services/retry"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
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

func (d *ConsensusRetryPolicyDelegator) GetEpochLength(
	opts *bind.CallOpts,
) (*big.Int, error) {

	return retry.CallFunctionWithRetryPolicy(d.delegate.GetEpochLength,
		opts,
		d.maxRetries,
		d.delayBetweenCalls,
		"Consensus::GetEpochLength",
	)

}

type retrieveClaimAcceptedEventsArgs struct {
	opts         *bind.FilterOpts
	appAddresses []common.Address
}

func (d *ConsensusRetryPolicyDelegator) RetrieveClaimAcceptanceEvents(
	opts *bind.FilterOpts,
	appAddresses []common.Address,
) ([]*iconsensus.IConsensusClaimAcceptance, error) {
	return retry.CallFunctionWithRetryPolicy(d.retrieveClaimAcceptanceEvents,
		retrieveClaimAcceptedEventsArgs{
			opts:         opts,
			appAddresses: appAddresses,
		}, d.maxRetries,
		d.delayBetweenCalls,
		"Consensus::RetrieveClaimAcceptedEvents")
}

func (d *ConsensusRetryPolicyDelegator) retrieveClaimAcceptanceEvents(
	args retrieveClaimAcceptedEventsArgs) ([]*iconsensus.IConsensusClaimAcceptance, error) {
	return d.delegate.RetrieveClaimAcceptanceEvents(args.opts, args.appAddresses)
}

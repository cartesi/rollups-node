// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retrypolicy

import (
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/services/retry"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type InputSourceWithRetryPolicyDelegator struct {
	delegate   evmreader.InputSource
	maxRetries uint64
	delay      time.Duration
}

func NewInputSourceWithRetryPolicy(
	delegate evmreader.InputSource,
	maxRetries uint64,
	delay time.Duration,
) *InputSourceWithRetryPolicyDelegator {
	return &InputSourceWithRetryPolicyDelegator{
		delegate:   delegate,
		maxRetries: maxRetries,
		delay:      delay,
	}
}

type retrieveInputsArgs struct {
	opts        *bind.FilterOpts
	appContract []common.Address
	index       []*big.Int
}

func (d *InputSourceWithRetryPolicyDelegator) RetrieveInputs(
	opts *bind.FilterOpts,
	appContract []common.Address,
	index []*big.Int,
) ([]iinputbox.IInputBoxInputAdded, error) {
	return retry.CallFunctionWithRetryPolicy(d.retrieveInputs,
		retrieveInputsArgs{
			opts:        opts,
			appContract: appContract,
			index:       index,
		},
		d.maxRetries,
		d.delay,
		"InputSource::RetrieveInputs",
	)
}

func (d *InputSourceWithRetryPolicyDelegator) retrieveInputs(
	args retrieveInputsArgs,
) ([]iinputbox.IInputBoxInputAdded, error) {
	return d.delegate.RetrieveInputs(
		args.opts,
		args.appContract,
		args.index,
	)
}

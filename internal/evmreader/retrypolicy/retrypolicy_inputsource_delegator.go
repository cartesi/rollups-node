// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package retrypolicy

import (
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/util/retrypolicy"
	"github.com/cartesi/rollups-node/pkg/contracts/inputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type (
	InputSource        = evmreader.InputSource
	Duration           = time.Duration
	Address            = common.Address
	FilterOpts         = bind.FilterOpts
	InputBoxInputAdded = inputbox.InputBoxInputAdded
)

type InputSourceWithRetryPolicyDelegator struct {
	delegate   InputSource
	maxRetries uint64
	delay      time.Duration
}

func NewInputSourceWithRetryPolicy(
	delegate InputSource,
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
	opts        *FilterOpts
	appContract []Address
	index       []*big.Int
}

func (d *InputSourceWithRetryPolicyDelegator) RetrieveInputs(
	opts *bind.FilterOpts,
	appContract []common.Address,
	index []*big.Int,
) ([]InputBoxInputAdded, error) {
	return retrypolicy.CallFunctionWithRetryPolicy(d.retrieveInputs,
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
) ([]InputBoxInputAdded, error) {
	return d.delegate.RetrieveInputs(
		args.opts,
		args.appContract,
		args.index,
	)
}
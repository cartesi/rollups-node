// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inputreader

import (
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/util"
	"github.com/cartesi/rollups-node/pkg/contracts/inputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type InputSourceWithRetryPolicyDelegator struct {
	delegate   InputSource
	maxRetries uint
	delay      time.Duration
}

func NewInputSourceWithRetryPolicy(
	delegate InputSource,
	masxRetries uint,
	delay time.Duration,
) *InputSourceWithRetryPolicyDelegator {
	return &InputSourceWithRetryPolicyDelegator{
		delegate:   delegate,
		maxRetries: masxRetries,
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
) ([]*inputbox.InputBoxInputAdded, error) {
	return util.CallFunctionWithRetryPolicy(d.retrieveInputs,
		retrieveInputsArgs{
			opts:        opts,
			appContract: appContract,
			index:       index,
		},
		d.maxRetries,
		d.delay,
	)
}

func (d *InputSourceWithRetryPolicyDelegator) retrieveInputs(
	args retrieveInputsArgs,
) ([]*inputbox.InputBoxInputAdded, error) {
	return d.delegate.RetrieveInputs(
		args.opts,
		args.appContract,
		args.index,
	)
}

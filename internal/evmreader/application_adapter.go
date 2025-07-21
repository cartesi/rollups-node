// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// IApplication Wrapper
type ApplicationContractAdapterImpl struct {
	application        *iapplication.IApplication
	client             *ethclient.Client
	applicationAddress common.Address
	filter             ethutil.Filter
}

func NewApplicationContractAdapter(
	appAddress common.Address,
	client *ethclient.Client,
	filter ethutil.Filter,
) (ApplicationContractAdapter, error) {
	applicationContract, err := iapplication.NewIApplication(appAddress, client)
	if err != nil {
		return nil, err
	}
	return &ApplicationContractAdapterImpl{
		application:        applicationContract,
		applicationAddress: appAddress,
		client:             client,
		filter:             filter,
	}, nil
}

func buildOutputExecutedFilterQuery(
	opts *bind.FilterOpts,
	applicationAddress common.Address,
) (q ethereum.FilterQuery, err error) {
	c, err := iapplication.IApplicationMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[MonitoredEvent_OutputExecuted.String()].ID},
	)
	if err != nil {
		return q, err
	}

	q = ethereum.FilterQuery{
		Addresses: []common.Address{applicationAddress},
		FromBlock: new(big.Int).SetUint64(opts.Start),
		Topics:    topics,
	}
	if opts.End != nil {
		q.ToBlock = new(big.Int).SetUint64(*opts.End)
	}
	return q, err
}

func (a *ApplicationContractAdapterImpl) RetrieveOutputExecutionEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationOutputExecuted, error) {
	q, err := buildOutputExecutedFilterQuery(opts, a.applicationAddress)
	if err != nil {
		return nil, err
	}

	itr, err := a.filter.ChunkedFilterLogs(opts.Context, a.client, q)
	if err != nil {
		return nil, err
	}

	var events []*iapplication.IApplicationOutputExecuted
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := a.application.ParseOutputExecuted(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func (a *ApplicationContractAdapterImpl) GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error) {
	return a.application.GetDeploymentBlockNumber(opts)
}

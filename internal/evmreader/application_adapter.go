// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type ApplicationContractAdapter interface {
	RetrieveOutputExecutionEvents(
		opts *bind.FilterOpts,
	) ([]*iapplication.IApplicationOutputExecuted, error)
	RetrieveForeclosureEvents(
		opts *bind.FilterOpts,
	) ([]*iapplication.IApplicationForeclosure, error)
	RetrieveWithdrawalEvents(
		opts *bind.FilterOpts,
	) ([]*iapplication.IApplicationWithdrawal, error)
	RetrieveAccountsDriveProvedEvents(
		opts *bind.FilterOpts,
	) ([]*iapplication.IApplicationAccountsDriveMerkleRootProved, error)
	GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error)
	GetAccountsDriveMerkleRoot(opts *bind.CallOpts) (bool, common.Hash, error)
	GetNumberOfExecutedOutputs(opts *bind.CallOpts) (*big.Int, error)
	GetNumberOfWithdrawals(opts *bind.CallOpts) (*big.Int, error)
	IsForeclosed(opts *bind.CallOpts) (bool, error)
}

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

func buildApplicationEventFilterQuery(
	opts *bind.FilterOpts,
	applicationAddress common.Address,
	eventName MonitoredEvent,
) (q ethereum.FilterQuery, err error) {
	c, err := iapplication.IApplicationMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	event, ok := c.Events[eventName.String()]
	if !ok {
		return q, fmt.Errorf("event %q not found in IApplication ABI", eventName)
	}
	topics, err := abi.MakeTopics(
		[]any{event.ID},
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

func retrieveApplicationEvents[T any](
	opts *bind.FilterOpts,
	client *ethclient.Client,
	filter *ethutil.Filter,
	applicationAddress common.Address,
	eventName MonitoredEvent,
	parse func(types.Log) (*T, error),
) ([]*T, error) {
	q, err := buildApplicationEventFilterQuery(opts, applicationAddress, eventName)
	if err != nil {
		return nil, err
	}

	itr, err := filter.ChunkedFilterLogs(opts.Context, client, q)
	if err != nil {
		return nil, err
	}

	var events []*T
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := parse(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func (a *ApplicationContractAdapterImpl) RetrieveOutputExecutionEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationOutputExecuted, error) {
	return retrieveApplicationEvents(
		opts,
		a.client,
		&a.filter,
		a.applicationAddress,
		MonitoredEvent_OutputExecuted,
		a.application.ParseOutputExecuted,
	)
}

func (a *ApplicationContractAdapterImpl) GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error) {
	return a.application.GetDeploymentBlockNumber(opts)
}

func (a *ApplicationContractAdapterImpl) GetAccountsDriveMerkleRoot(opts *bind.CallOpts) (bool, common.Hash, error) {
	result, err := a.application.GetAccountsDriveMerkleRoot(opts)
	if err != nil {
		return false, common.Hash{}, err
	}
	return result.WasAccountsDriveMerkleRootProved, common.Hash(result.AccountsDriveMerkleRoot), nil
}

func (a *ApplicationContractAdapterImpl) GetNumberOfExecutedOutputs(opts *bind.CallOpts) (*big.Int, error) {
	return a.application.GetNumberOfExecutedOutputs(opts)
}

func (a *ApplicationContractAdapterImpl) IsForeclosed(opts *bind.CallOpts) (bool, error) {
	return a.application.IsForeclosed(opts)
}

func (a *ApplicationContractAdapterImpl) GetNumberOfWithdrawals(opts *bind.CallOpts) (*big.Int, error) {
	return a.application.GetNumberOfWithdrawals(opts)
}

func (a *ApplicationContractAdapterImpl) RetrieveForeclosureEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationForeclosure, error) {
	return retrieveApplicationEvents(
		opts,
		a.client,
		&a.filter,
		a.applicationAddress,
		MonitoredEvent_Foreclosure,
		a.application.ParseForeclosure,
	)
}

func (a *ApplicationContractAdapterImpl) RetrieveWithdrawalEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationWithdrawal, error) {
	return retrieveApplicationEvents(
		opts,
		a.client,
		&a.filter,
		a.applicationAddress,
		MonitoredEvent_Withdrawal,
		a.application.ParseWithdrawal,
	)
}

func (a *ApplicationContractAdapterImpl) RetrieveAccountsDriveProvedEvents(
	opts *bind.FilterOpts,
) ([]*iapplication.IApplicationAccountsDriveMerkleRootProved, error) {
	return retrieveApplicationEvents(
		opts,
		a.client,
		&a.filter,
		a.applicationAddress,
		MonitoredEvent_AccountsDriveMerkleRootProved,
		a.application.ParseAccountsDriveMerkleRootProved,
	)
}

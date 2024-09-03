// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	appcontract "github.com/cartesi/rollups-node/pkg/contracts/application"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// IConsensus Wrapper
type ApplicationContractAdapter struct {
	application *appcontract.Application
}

func NewApplicationContractAdapter(
	appAddress common.Address,
	client *ethclient.Client,
) (*ApplicationContractAdapter, error) {
	applicationContract, err := appcontract.NewApplication(appAddress, client)
	if err != nil {
		return nil, err
	}
	return &ApplicationContractAdapter{
		application: applicationContract,
	}, nil
}

func (a *ApplicationContractAdapter) GetConsensus(opts *bind.CallOpts) (common.Address, error) {
	return a.application.GetConsensus(opts)
}

func (a *ApplicationContractAdapter) RetrieveOutputExecutionEvents(
	opts *bind.FilterOpts,
) ([]*appcontract.ApplicationOutputExecuted, error) {

	itr, err := a.application.FilterOutputExecuted(opts)
	if err != nil {
		return nil, err
	}
	defer itr.Close()

	var events []*appcontract.ApplicationOutputExecuted
	for itr.Next() {
		outputExecutedEvent := itr.Event
		events = append(events, outputExecutedEvent)
	}
	if err = itr.Error(); err != nil {
		return nil, err
	}
	return events, nil
}

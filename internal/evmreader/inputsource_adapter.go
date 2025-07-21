// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// InputBox Wrapper
type InputSourceAdapterImpl struct {
	inputbox        *iinputbox.IInputBox
	client          *ethclient.Client
	inputBoxAddress common.Address
	filter          ethutil.Filter
}

func NewInputSourceAdapter(
	inputBoxAddress common.Address,
	client *ethclient.Client,
	filter ethutil.Filter,
) (InputSourceAdapter, error) {
	inputbox, err := iinputbox.NewIInputBox(inputBoxAddress, client)
	if err != nil {
		return nil, err
	}
	return &InputSourceAdapterImpl{
		inputbox:        inputbox,
		client:          client,
		inputBoxAddress: inputBoxAddress,
		filter:          filter,
	}, nil
}

func buildInputAddedFilterQuery(
	opts *bind.FilterOpts,
	inputBoxAddress common.Address,
	appContract []common.Address,
	index []*big.Int,
) (q ethereum.FilterQuery, err error) {
	c, err := iinputbox.IInputBoxMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	var appContractRule []any
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}
	var indexRule []any
	for _, indexItem := range index {
		indexRule = append(indexRule, indexItem)
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[MonitoredEvent_InputAdded.String()].ID},
		appContractRule,
		indexRule,
	)
	if err != nil {
		return q, err
	}

	q = ethereum.FilterQuery{
		Addresses: []common.Address{inputBoxAddress},
		FromBlock: new(big.Int).SetUint64(opts.Start),
		Topics:    topics,
	}
	if opts.End != nil {
		q.ToBlock = new(big.Int).SetUint64(*opts.End)
	}
	return q, err
}

func (i *InputSourceAdapterImpl) RetrieveInputs(
	opts *bind.FilterOpts,
	appContract []common.Address,
	index []*big.Int,
) ([]iinputbox.IInputBoxInputAdded, error) {
	q, err := buildInputAddedFilterQuery(opts, i.inputBoxAddress, appContract, index)
	if err != nil {
		return nil, err
	}

	itr, err := i.filter.ChunkedFilterLogs(opts.Context, i.client, q)
	if err != nil {
		return nil, err
	}

	var events []iinputbox.IInputBoxInputAdded
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := i.inputbox.ParseInputAdded(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, *ev)
	}
	return events, nil
}

func (i *InputSourceAdapterImpl) GetNumberOfInputs(opts *bind.CallOpts, addr common.Address) (*big.Int, error) {
	return i.inputbox.GetNumberOfInputs(opts, addr)
}

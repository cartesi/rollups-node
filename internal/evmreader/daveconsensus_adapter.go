// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Interface for DaveConsensus reading
type DaveConsensusAdapter interface {
	GetInputBox(opts *bind.CallOpts) (common.Address, error)
	GetCurrentSealedEpoch(opts *bind.CallOpts) (struct {
		EpochNumber          *big.Int
		InputIndexLowerBound *big.Int
		InputIndexUpperBound *big.Int
		Tournament           common.Address
	}, error)
	GetApplicationContract(opts *bind.CallOpts) (common.Address, error)
	GetTournamentFactory(opts *bind.CallOpts) (common.Address, error)
	GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error)
	RetrieveSealedEpochs(opts *bind.FilterOpts) ([]*idaveconsensus.IDaveConsensusEpochSealed, error)
}

// DaveConsensus Wrapper
type DaveConsensusAdapterImpl struct {
	daveConsensus        *idaveconsensus.IDaveConsensus
	client               *ethclient.Client
	daveConsensusAddress common.Address
	filter               ethutil.Filter
}

func NewDaveConsensusAdapter(
	daveConsensusAddress common.Address,
	client *ethclient.Client,
	filter ethutil.Filter,
) (DaveConsensusAdapter, error) {
	daveConsensusContract, err := idaveconsensus.NewIDaveConsensus(daveConsensusAddress, client)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusAdapterImpl{
		daveConsensus:        daveConsensusContract,
		daveConsensusAddress: daveConsensusAddress,
		client:               client,
		filter:               filter,
	}, nil
}

func buildEpochSealedFilterQuery(
	opts *bind.FilterOpts,
	daveConsensusAddress common.Address,
) (q ethereum.FilterQuery, err error) {
	c, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
	if err != nil {
		return q, err
	}

	topics, err := abi.MakeTopics(
		[]any{c.Events[MonitoredEvent_EpochSealed.String()].ID},
	)
	if err != nil {
		return q, err
	}

	q = ethereum.FilterQuery{
		Addresses: []common.Address{daveConsensusAddress},
		FromBlock: new(big.Int).SetUint64(opts.Start),
		Topics:    topics,
	}
	if opts.End != nil {
		q.ToBlock = new(big.Int).SetUint64(*opts.End)
	}
	return q, err
}

func (d *DaveConsensusAdapterImpl) GetInputBox(opts *bind.CallOpts) (common.Address, error) {
	return d.daveConsensus.GetInputBox(opts)
}

func (d *DaveConsensusAdapterImpl) GetCurrentSealedEpoch(opts *bind.CallOpts) (struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
}, error) {
	return d.daveConsensus.GetCurrentSealedEpoch(opts)
}

func (d *DaveConsensusAdapterImpl) GetApplicationContract(opts *bind.CallOpts) (common.Address, error) {
	return d.daveConsensus.GetApplicationContract(opts)
}

func (d *DaveConsensusAdapterImpl) GetTournamentFactory(opts *bind.CallOpts) (common.Address, error) {
	return d.daveConsensus.GetTournamentFactory(opts)
}

func (d *DaveConsensusAdapterImpl) GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error) {
	return d.daveConsensus.GetDeploymentBlockNumber(opts)
}

func (d *DaveConsensusAdapterImpl) RetrieveSealedEpochs(
	opts *bind.FilterOpts,
) ([]*idaveconsensus.IDaveConsensusEpochSealed, error) {
	q, err := buildEpochSealedFilterQuery(opts, d.daveConsensusAddress)
	if err != nil {
		return nil, err
	}

	itr, err := d.filter.ChunkedFilterLogs(opts.Context, d.client, q)
	if err != nil {
		return nil, err
	}

	var events []*idaveconsensus.IDaveConsensusEpochSealed
	for log, err := range itr {
		if err != nil {
			return nil, err
		}
		ev, err := d.daveConsensus.ParseEpochSealed(*log)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

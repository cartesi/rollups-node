// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// IConsensus Wrapper
type ConsensusContractAdapter struct {
	consensus *iconsensus.IConsensus
}

func NewConsensusContractAdapter(
	iconsensusAddress common.Address,
	client *ethclient.Client,
) (*ConsensusContractAdapter, error) {
	consensus, err := iconsensus.NewIConsensus(iconsensusAddress, client)
	if err != nil {
		return nil, err
	}
	return &ConsensusContractAdapter{
		consensus: consensus,
	}, nil
}

func (c *ConsensusContractAdapter) GetEpochLength(opts *bind.CallOpts) (*big.Int, error) {
	return c.consensus.GetEpochLength(opts)
}

func (c *ConsensusContractAdapter) RetrieveClaimAcceptanceEvents(
	opts *bind.FilterOpts,
	appAddresses []common.Address,
) ([]*iconsensus.IConsensusClaimAcceptance, error) {

	itr, err := c.consensus.FilterClaimAcceptance(opts, appAddresses)
	if err != nil {
		return nil, err
	}
	defer itr.Close()

	var events []*iconsensus.IConsensusClaimAcceptance
	for itr.Next() {
		claimAcceptanceEvent := itr.Event
		events = append(events, claimAcceptanceEvent)
	}
	if err = itr.Error(); err != nil {
		return nil, err
	}
	return events, nil
}

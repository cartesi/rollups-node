// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iquorumfactory"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type QuorumDeployment struct {
	Address            common.Address   `json:"address"`
	FactoryAddress     common.Address   `json:"factory"`
	Validators         []common.Address `json:"validators"`
	EpochLength        uint64           `json:"epoch_length"`
	ClaimStagingPeriod uint64           `json:"claim_staging_period"`
	Salt               SaltBytes        `json:"salt"`
	Verbose            bool             `json:"-"`
}

func (me *QuorumDeployment) String() string {
	result := ""
	result += fmt.Sprintf("quorum deployment:\n")
	result += fmt.Sprintf("\tvalidators:           %v\n", me.Validators)
	if me.Verbose {
		result += fmt.Sprintf("\tfactory address:       %v\n", me.FactoryAddress)
		result += fmt.Sprintf("\tsalt:                  %v\n", me.Salt)
		result += fmt.Sprintf("\tepoch length:          %v\n", me.EpochLength)
		result += fmt.Sprintf("\tclaim staging period:  %v\n", me.ClaimStagingPeriod)
	}
	return result
}

func (me *QuorumDeployment) Deploy(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (common.Address, error) {
	zero := common.Address{}
	factory, err := iquorumfactory.NewIQuorumFactory(me.FactoryAddress, client)
	if err != nil {
		return zero, fmt.Errorf("failed to instantiate contract: %v", err)
	}

	epochLength := new(big.Int).SetUint64(me.EpochLength)
	claimStagingPeriod := new(big.Int).SetUint64(me.ClaimStagingPeriod)
	quorumAddress, err := factory.CalculateQuorumAddress(
		nil,
		me.Validators,
		epochLength,
		claimStagingPeriod,
		me.Salt,
	)
	if err != nil {
		return zero, err
	}

	quorumCode, err := client.CodeAt(ctx, quorumAddress, nil)
	if err != nil {
		return zero, err
	}
	if len(quorumCode) != 0 {
		return zero, fmt.Errorf("quorum with address: %v already exists. Try a different salt.", quorumAddress)
	}

	tx, err := factory.NewQuorum(txOpts, me.Validators, epochLength, claimStagingPeriod, me.Salt)
	if err != nil {
		return zero, fmt.Errorf("failed to create new quorum: %v", err)
	}

	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return zero, fmt.Errorf("failed to mine new quorum transaction: %v", err)
	}

	if receipt.Status != 1 {
		return zero, fmt.Errorf("transaction failed")
	}

	for _, vLog := range receipt.Logs {
		event, err := factory.ParseQuorumCreated(*vLog)
		if err != nil {
			continue
		}
		return event.Quorum, nil
	}
	return zero, fmt.Errorf("failed to find event in receipt logs")
}

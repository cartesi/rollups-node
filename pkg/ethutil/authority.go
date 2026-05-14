// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iauthorityfactory"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type AuthorityDeployment struct {
	Address            common.Address `json:"address"`
	FactoryAddress     common.Address `json:"factory"`
	OwnerAddress       common.Address `json:"owner"`
	EpochLength        uint64         `json:"epoch_length"`
	ClaimStagingPeriod uint64         `json:"claim_staging_period"`
	Salt               SaltBytes      `json:"salt"`
	Verbose            bool           `json:"-"`
}

func (me *AuthorityDeployment) String() string {
	result := ""
	result += fmt.Sprintf("authority deployment:\n")
	result += fmt.Sprintf("\tauthority owner:       %v\n", me.OwnerAddress)
	if me.Verbose {
		result += fmt.Sprintf("\tfactory address:       %v\n", me.FactoryAddress)
		result += fmt.Sprintf("\tsalt:                  %v\n", me.Salt)
		result += fmt.Sprintf("\tepoch length:          %v\n", me.EpochLength)
		result += fmt.Sprintf("\tclaim staging period:  %v\n", me.ClaimStagingPeriod)
	}
	return result
}

func (me *AuthorityDeployment) Deploy(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (common.Address, error) {
	zero := common.Address{}
	factory, err := iauthorityfactory.NewIAuthorityFactory(me.FactoryAddress, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to instantiate contract: %v", err)
	}

	// check if addresses are available (have no code)
	authorityAddress, err := factory.CalculateAuthorityAddress(nil, me.OwnerAddress, new(big.Int).SetUint64(me.EpochLength), new(big.Int).SetUint64(me.ClaimStagingPeriod), me.Salt)
	if err != nil {
		return zero, err
	}

	authorityCode, err := client.CodeAt(ctx, authorityAddress, nil)
	if err != nil {
		return zero, err
	}
	if len(authorityCode) != 0 {
		return zero, fmt.Errorf("authority with address: %v already exists. Try a different salt.", authorityAddress)
	}

	// deploy the contracts
	tx, err := factory.NewAuthority0(txOpts, me.OwnerAddress, new(big.Int).SetUint64(me.EpochLength), new(big.Int).SetUint64(me.ClaimStagingPeriod), me.Salt)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to create new authority: %v", err)
	}

	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to mine new authority transaction: %v", err)
	}

	if receipt.Status != 1 {
		return common.Address{}, fmt.Errorf("transaction failed")
	}

	// search for the matching event
	for _, vLog := range receipt.Logs {
		event, err := factory.ParseAuthorityCreated(*vLog)
		if err != nil {
			continue // Skip logs that don't match
		}
		return event.Authority, nil
	}
	return common.Address{}, fmt.Errorf("failed to find event in receipt logs")
}

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveappfactory"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type PRTApplicationDeployment struct {
	ApplicationDeployment
}

type PRTApplicationDeploymentResult struct {
	Deployment *PRTApplicationDeployment

	ApplicationAddress   common.Address `json:"application_address"`
	DaveConsensusAddress common.Address `json:"dave_consensus_address"`
	DataAvailability     []byte         `json:"data_availability"`

	InputBoxAddress common.Address `json:"inputbox_address"`
	IInputBoxBlock  uint64         `json:"inputbox_block"`
}

func (me *PRTApplicationDeployment) String() string {
	result := ""
	result += fmt.Sprintf("PRT application deployment:\n")
	if me.Verbose {
		result += fmt.Sprintf("\tPRT application factory address:   %v\n", me.FactoryAddress)
		result += fmt.Sprintf("\ttemplate hash:         %v\n", me.TemplateHash)
		result += fmt.Sprintf("\tsalt:                  %v\n", me.Salt)
	}
	return result
}

func (me *PRTApplicationDeploymentResult) String() string {
	result := ""
	result += fmt.Sprintf("\tapplication address:   %v\n", me.ApplicationAddress)
	result += fmt.Sprintf("\tconsensus address:     %v\n", me.DaveConsensusAddress)
	result += fmt.Sprintf("\tdata availability:     0x%v\n", hex.EncodeToString(me.DataAvailability))
	result += fmt.Sprintf("\tinputbox address:   %v\n", me.InputBoxAddress)
	result += fmt.Sprintf("\tinputbox block address:   %d\n", me.IInputBoxBlock)
	return result
}

func (me *PRTApplicationDeployment) deployPRT(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (common.Address, common.Address, error) {
	zero := common.Address{}

	factory, err := idaveappfactory.NewIDaveAppFactory(me.FactoryAddress, client)
	if err != nil {
		return zero, zero, fmt.Errorf("failed to instantiate contract binding: %v", err)
	}
	tx, err := factory.NewDaveApp(txOpts, me.TemplateHash, me.Salt)
	if err != nil {
		return zero, zero, fmt.Errorf("transaction failed: %v", err)
	}

	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return zero, zero, fmt.Errorf("failed to wait for transaction mining: %v", err)
	}

	if receipt.Status != 1 {
		return zero, zero, fmt.Errorf("transaction failed")
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		// Parse log for DaveConsensusCreated event
		event, err := factory.ParseDaveAppCreated(*vLog)
		if err != nil {
			continue // Skip logs that don't match
		}
		return event.AppContract, event.DaveConsensus, nil
	}
	return zero, zero, fmt.Errorf("failed to find DaveAppCreated event in receipt logs")
}

func (me *PRTApplicationDeployment) Deploy(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (common.Address, IApplicationDeploymentResult, error) {
	zero := common.Address{}
	result := &PRTApplicationDeploymentResult{}
	result.Deployment = me

	var err error
	appAddress, consensusAddress, err := me.deployPRT(ctx, client, txOpts)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to deploy Dave Application and consensus contracts: %w", err)
	}

	result.ApplicationAddress = appAddress
	result.DaveConsensusAddress = consensusAddress

	application, err := iapplication.NewIApplication(appAddress, client)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to instantiate application: %v", err)
	}

	da, err := application.GetDataAvailability(nil)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to retrieve data availability: %v", err)
	}
	result.DataAvailability = da

	result.InputBoxAddress, result.IInputBoxBlock, err = DecodeDA(client, da)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to decode data availability: %v", err)
	}

	return appAddress, result, nil
}

func (me *PRTApplicationDeployment) GetFactoryAddress() common.Address {
	return me.FactoryAddress
}

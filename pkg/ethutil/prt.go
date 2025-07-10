// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/daveconsensusfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type PRTApplicationDeployment struct {
	FactoryAddress common.Address
	TemplateHash   common.Hash
	Salt           SaltBytes

	Application *ApplicationDeployment
}

type PRTApplicationDeploymentResult struct {
	Deployment        *PRTApplicationDeployment
	ApplicationResult *ApplicationDeploymentResult
}

func (me *PRTApplicationDeployment) String() string {
	result := ""
	result += fmt.Sprintf("PRT application deployment:\n")
	result += fmt.Sprintf("\tapplication owner:     %v\n", me.Application.OwnerAddress)
	if me.Application.Verbose {
		result += fmt.Sprintf("\tPRT factory address:   %v\n", me.FactoryAddress)
		result += fmt.Sprintf("\tAPP factory address:   %v\n", me.Application.FactoryAddress)
		result += fmt.Sprintf("\ttemplate hash:         %v\n", me.Application.TemplateHash)
		result += fmt.Sprintf("\tdata availability:     0x%v\n", hex.EncodeToString(me.Application.DataAvailability))
		result += fmt.Sprintf("\tsalt:                  %v\n", me.Application.Salt)
	}
	return result
}

func (me *PRTApplicationDeploymentResult) String() string {
	result := ""
	result += fmt.Sprintf("\tapplication address:   %v\n", me.ApplicationResult.ApplicationAddress)
	result += fmt.Sprintf("\tconsensus address:     %v\n", me.ApplicationResult.Deployment.Consensus)
	return result
}

func (me *PRTApplicationDeployment) deployPRT(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
	applicationAddress common.Address,
) (common.Address, error) {
	zero := common.Address{}

	factory, err := daveconsensusfactory.NewDaveConsensusFactory(me.FactoryAddress, client)
	if err != nil {
		return zero, fmt.Errorf("failed to instantiate contract: %v", err)
	}
	tx, err := factory.NewDaveConsensus(txOpts, applicationAddress, me.TemplateHash, me.Salt)
	if err != nil {
		return zero, fmt.Errorf("transaction failed: %v", err)
	}

	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return zero, fmt.Errorf("failed to wait for transaction mining: %v", err)
	}

	if receipt.Status != 1 {
		return zero, fmt.Errorf("transaction failed")
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		// Parse log for DaveConsensusCreated event
		event, err := factory.ParseDaveConsensusCreated(*vLog)
		if err != nil {
			continue // Skip logs that don't match
		}
		return event.DaveConsensus, nil
	}
	return zero, fmt.Errorf("failed to find DaveConsensusCreated event in receipt logs")
}

// Do the consensus/application dance based on: https://github.com/cartesi/dave/blob/v1.0.0/cartesi-rollups/contracts/cannonfile.prod-instance.toml
func (me *PRTApplicationDeployment) Deploy(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (common.Address, IApplicationDeploymentResult, error) {
	zero := common.Address{}
	result := &PRTApplicationDeploymentResult{}
	result.Deployment = me

	var err error
	applicationAddress, appResult, err := me.Application.Deploy(ctx, client, txOpts)
	if err != nil {
		return zero, nil, err
	}

	switch appRes := appResult.(type) {
	case *ApplicationDeploymentResult:
		result.ApplicationResult = appRes
	default:
		panic("Application deployment returned an impossible type.")
	}

	consensus, err := me.deployPRT(ctx, client, txOpts, applicationAddress)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to deploy PRT contract: %w", err)
	}

	application, err := iapplication.NewIApplication(applicationAddress, client)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to instantiate application: %v", err)
	}

	_, err = sendTransaction(ctx, client, txOpts, big.NewInt(0), GasLimit,
		func(txOpts *bind.TransactOpts) (*types.Transaction, error) {
			return application.MigrateToOutputsMerkleRootValidator(txOpts, consensus)
		},
	)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to create a self hosted application: execution reverted")
	}

	_, err = sendTransaction(ctx, client, txOpts, big.NewInt(0), GasLimit,
		func(txOpts *bind.TransactOpts) (*types.Transaction, error) {
			return application.RenounceOwnership(txOpts)
		},
	)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to create a self hosted application: execution reverted")
	}

	result.ApplicationResult.Deployment.Consensus = consensus
	return applicationAddress, result, nil
}

func (me *PRTApplicationDeployment) GetFactoryAddress() common.Address {
	return me.FactoryAddress
}

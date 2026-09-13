// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveappfactory"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type PRTApplicationDeployment struct {
	ApplicationDeployment

	// A zero manager disables rotation. Sentry IDs follow the supplied order.
	SentryManager common.Address
	Sentries      []common.Address
}

type PRTApplicationDeploymentResult struct {
	Deployment *PRTApplicationDeployment

	ApplicationAddress   common.Address `json:"application_address"`
	DaveConsensusAddress common.Address `json:"dave_consensus_address"`

	InputBoxAddress common.Address `json:"inputbox_address"`
	IInputBoxBlock  uint64         `json:"inputbox_block"`
}

func (me *PRTApplicationDeployment) String() string {
	result := ""
	result += "PRT application deployment:\n"
	if me.Verbose {
		result += fmt.Sprintf("\tPRT application factory address:   %v\n", me.FactoryAddress)
		result += fmt.Sprintf("\ttemplate hash:         %v\n", me.TemplateHash)
		result += fmt.Sprintf("\tsalt:                  %v\n", me.Salt)
		result += fmt.Sprintf("\tsentry manager:        %v\n", me.SentryManager)
		result += fmt.Sprintf("\tsentries:              %v\n", me.Sentries)
	}
	return result
}

func (me *PRTApplicationDeploymentResult) String() string {
	result := ""
	result += fmt.Sprintf("\tapplication address:   %v\n", me.ApplicationAddress)
	result += fmt.Sprintf("\tconsensus address:     %v\n", me.DaveConsensusAddress)
	result += fmt.Sprintf("\tinput box address:     %v\n", me.InputBoxAddress)
	result += fmt.Sprintf("\tinput box block:       %d\n", me.IInputBoxBlock)
	return result
}

func (me *PRTApplicationDeployment) Deploy(
	ctx context.Context,
	client *ethclient.Client,
	txOptsFactory TransactOptsFactory,
) (common.Address, IApplicationDeploymentResult, error) {
	return me.DeployWithTransaction(ctx, client, txOptsFactory, nil)
}

// DeployWithTransaction returns the predicted application address and no confirmed
// deployment result when runner does not wait for a receipt. The prediction does
// not prove that the deployment transaction succeeded.
func (me *PRTApplicationDeployment) DeployWithTransaction(
	ctx context.Context,
	client *ethclient.Client,
	txOptsFactory TransactOptsFactory,
	runner TransactionRunner,
) (common.Address, IApplicationDeploymentResult, error) {
	zero := common.Address{}
	result := &PRTApplicationDeploymentResult{Deployment: me}
	if err := ValidateSentryAddresses(me.Sentries); err != nil {
		return zero, nil, err
	}

	factory, err := idaveappfactory.NewIDaveAppFactory(me.FactoryAddress, client)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to instantiate contract binding: %w", err)
	}

	if err := ValidateWithdrawalConfig(me.WithdrawalConfig); err != nil {
		return zero, nil, err
	}
	if err := CheckWithdrawalOutputBuilderCode(ctx, client, me.WithdrawalConfig); err != nil {
		return zero, nil, err
	}

	claimStagingPeriod := new(big.Int).SetUint64(me.ClaimStagingPeriod)
	daveWC := idaveappfactory.WithdrawalConfig(me.WithdrawalConfig)

	// check if addresses are available (have no code)
	addresses, err := factory.CalculateDaveAppAddress(
		&bind.CallOpts{Context: ctx},
		me.TemplateHash,
		claimStagingPeriod,
		me.SentryManager,
		me.Sentries,
		daveWC,
		me.Salt,
	)
	if err != nil {
		return zero, nil, err
	}
	applicationCode, err := client.CodeAt(ctx, addresses.AppContractAddress, nil)
	if err != nil {
		return zero, nil, err
	}
	if len(applicationCode) != 0 {
		return zero, nil, fmt.Errorf("application with address %v already exists; use a different salt", addresses.AppContractAddress)
	}

	daveConsensusCode, err := client.CodeAt(ctx, addresses.DaveConsensusAddress, nil)
	if err != nil {
		return zero, nil, err
	}
	if len(daveConsensusCode) != 0 {
		return zero, nil, fmt.Errorf("dave consensus with address %v already exists; use a different salt", addresses.DaveConsensusAddress)
	}

	txOpts, err := txOptsFactory.NewTransactOpts(ctx)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to create transaction options: %w", err)
	}
	receipt, err := runDeploymentTransaction(ctx, client, txOpts, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return factory.NewDaveApp(
			opts,
			me.TemplateHash,
			claimStagingPeriod,
			me.SentryManager,
			me.Sentries,
			daveWC,
			me.Salt,
		)
	}, runner)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to deploy Dave application and consensus contracts: %w", err)
	}
	if receipt == nil {
		return addresses.AppContractAddress, nil, nil
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		// Parse log for DaveConsensusCreated event
		event, err := factory.ParseDaveAppCreated(*vLog)
		if err != nil {
			continue // Skip logs that don't match
		}
		result.ApplicationAddress = event.AppContract
		result.DaveConsensusAddress = event.DaveConsensus
		break
	}
	if result.ApplicationAddress == zero {
		return zero, nil, fmt.Errorf("failed to find DaveAppCreated event in receipt logs")
	}
	appAddress := result.ApplicationAddress
	application, err := iapplication.NewIApplication(appAddress, client)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to instantiate application: %w", err)
	}

	inputBoxAddress, err := application.GetInputBox(&bind.CallOpts{Context: ctx})
	if err != nil {
		return zero, nil, fmt.Errorf("failed to retrieve input box: %w", err)
	}
	if me.InputBoxAddress != (common.Address{}) && inputBoxAddress != me.InputBoxAddress {
		return zero, nil, fmt.Errorf(
			"deployed application uses input box %v instead of %v",
			inputBoxAddress,
			me.InputBoxAddress,
		)
	}
	result.InputBoxAddress = inputBoxAddress

	inputBoxBlock, err := GetInputBoxDeploymentBlock(ctx, client, inputBoxAddress)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to retrieve input box deployment block: %w", err)
	}
	if !inputBoxBlock.IsUint64() {
		return zero, nil, fmt.Errorf("input box deployment block does not fit uint64: %v", inputBoxBlock)
	}
	result.IInputBoxBlock = inputBoxBlock.Uint64()

	if err := VerifyDeployedWithdrawalConfig(ctx, client, appAddress, me.WithdrawalConfig); err != nil {
		return zero, nil, err
	}

	return appAddress, result, nil
}

func (me *PRTApplicationDeployment) GetFactoryAddress() common.Address {
	return me.FactoryAddress
}

// ValidateSentryAddresses checks the constructor's address rules without changing
// slot order. An empty list is valid and requires the full claim staging period.
func ValidateSentryAddresses(sentries []common.Address) error {
	seen := make(map[common.Address]struct{}, len(sentries))
	for i, address := range sentries {
		if address == (common.Address{}) {
			return fmt.Errorf("sentry %d must not be the zero address", i+1)
		}
		if _, duplicate := seen[address]; duplicate {
			return fmt.Errorf("sentry %d duplicates address %s", i+1, address)
		}
		seen[address] = struct{}{}
	}
	return nil
}

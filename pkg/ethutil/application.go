// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package ethutil

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type IApplicationDeployment interface {
	Deploy(ctx context.Context, client *ethclient.Client, txOpts *bind.TransactOpts) (common.Address, IApplicationDeploymentResult, error)
	GetFactoryAddress() common.Address
}
type IApplicationDeploymentResult interface{}

type ApplicationDeployment struct {
	FactoryAddress   common.Address                       `json:"factory"`
	Consensus        common.Address                       `json:"consensus"`
	OwnerAddress     common.Address                       `json:"owner"`
	DataAvailability []byte                               `json:"-"`
	TemplateHash     common.Hash                          `json:"template_hash"`
	WithdrawalConfig iapplicationfactory.WithdrawalConfig `json:"withdrawal_config"`
	Salt             SaltBytes                            `json:"salt"`

	// needed by model.Application
	InputBoxAddress    common.Address `json:"inputbox_address"`
	IInputBoxBlock     uint64         `json:"inputbox_block"`
	EpochLength        uint64         `json:"epoch_length"`
	ClaimStagingPeriod uint64         `json:"claim_staging_period"`
	ConsensusType      string         `json:"consensus_type,omitempty"`

	Verbose bool
}

type ApplicationDeploymentResult struct {
	Deployment *ApplicationDeployment `json:"deployment"`

	ApplicationAddress common.Address `json:"address"`
}

func (me *ApplicationDeployment) String() string {
	result := ""
	result += fmt.Sprintf("application deployment:\n")
	result += fmt.Sprintf("\tapplication owner:     %v\n", me.OwnerAddress)
	result += fmt.Sprintf("\tconsensus address:     %v\n", me.Consensus)
	if me.Verbose {
		result += fmt.Sprintf("\tfactory address:       %v\n", me.FactoryAddress)
		result += fmt.Sprintf("\ttemplate hash:         %v\n", me.TemplateHash)
		result += fmt.Sprintf("\tdata availability:     0x%v\n", hex.EncodeToString(me.DataAvailability))
		result += fmt.Sprintf("\tsalt:                  %v\n", me.Salt)
		result += fmt.Sprintf("\tepoch length:          %v\n", me.EpochLength)
		if me.ConsensusType != "" {
			result += fmt.Sprintf("\tconsensus type:        %v\n", me.ConsensusType)
		}
	}
	return result
}

func (me *ApplicationDeploymentResult) String() string {
	result := ""
	result += fmt.Sprintf("\tapplication address:   %v\n", me.ApplicationAddress)
	return result
}

func (me *ApplicationDeployment) Deploy(
	ctx context.Context,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (common.Address, IApplicationDeploymentResult, error) {
	zero := common.Address{}
	result := &ApplicationDeploymentResult{}
	result.Deployment = me
	factory, err := iapplicationfactory.NewIApplicationFactory(me.FactoryAddress, client)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to instantiate contract: %v", err)
	}

	if err := ValidateWithdrawalConfig(me.WithdrawalConfig); err != nil {
		return zero, nil, err
	}
	if err := CheckWithdrawalOutputBuilderCode(ctx, client, me.WithdrawalConfig); err != nil {
		return zero, nil, err
	}

	// check if addresses are available (have no code)
	applicationAddress, err := factory.CalculateApplicationAddress(nil, me.Consensus, me.OwnerAddress, me.TemplateHash, me.DataAvailability, me.WithdrawalConfig, me.Salt)
	if err != nil {
		return zero, nil, err
	}

	applicationCode, err := client.CodeAt(ctx, applicationAddress, nil)
	if err != nil {
		return zero, nil, err
	}
	if len(applicationCode) != 0 {
		return zero, nil, fmt.Errorf("application with address: %v already exists. Try a different salt.", applicationAddress)
	}

	// deploy the contracts
	tx, err := factory.NewApplication0(txOpts, me.Consensus, me.OwnerAddress, me.TemplateHash, me.DataAvailability, me.WithdrawalConfig, me.Salt)
	if err != nil {
		return zero, nil, fmt.Errorf("transaction failed: %v", err)
	}

	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return zero, nil, fmt.Errorf("failed to wait for transaction mining: %v", err)
	}

	if receipt.Status != 1 {
		return zero, nil, fmt.Errorf("transaction failed")
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		// Parse log for ApplicationCreated event
		event, err := factory.ParseApplicationCreated(*vLog)
		if err != nil {
			continue // Skip logs that don't match
		}
		result.ApplicationAddress = event.AppContract
		if err := VerifyDeployedWithdrawalConfig(ctx, client, result.ApplicationAddress, me.WithdrawalConfig); err != nil {
			return zero, nil, err
		}
		return result.ApplicationAddress, result, nil
	}
	return zero, nil, fmt.Errorf("failed to find ApplicationCreated event in receipt logs")
}

func (me *ApplicationDeployment) GetFactoryAddress() common.Address {
	return me.FactoryAddress
}

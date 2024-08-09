// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

const (
	CONTRACT_OWNER_ADDRESS = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	SALT                   = "0x0000000000000000000000000000000000000000000000000000000000000000"
)

func deploy(ctx context.Context,
	rollupsContractsPath string,
	hash string,
	epochLength uint64,
) (DeploymentInfo, error) {
	var depInfo DeploymentInfo

	fmt.Printf("deployer: deploying %v\n", rollupsContractsPath)
	err := deployRollupsContracts(ctx, rollupsContractsPath)
	if err != nil {
		return depInfo, fmt.Errorf("could not deploy rollups-contracts: %v", err)
	}

	depInfo, err = createApplication(ctx, hash, epochLength)
	if err != nil {
		return depInfo, fmt.Errorf("could not create Application: %v", err)
	}

	return depInfo, nil
}

// Create a Rollups Application by calling the necessary factories
func createApplication(
	ctx context.Context,
	hash string,
	epochLength uint64,
) (DeploymentInfo, error) {
	var depInfo DeploymentInfo
	addressBook := addresses.GetTestBook()

	// Create the Authority contract
	contractAddresses, err := createContracts(ctx,
		addressBook.AuthorityFactory.Hex(),
		"newAuthority(address,uint256,bytes32)(address)",
		CONTRACT_OWNER_ADDRESS,
		strconv.FormatUint(epochLength, 10),
		SALT)
	if err != nil {
		return DeploymentInfo{}, fmt.Errorf("could not create authority: %v", err)
	}
	depInfo.AuthorityAddress = contractAddresses[0]

	// Create the Application, passing the address of the newly created Authority
	contractAddresses, err = createContracts(ctx,
		addressBook.ApplicationFactory.Hex(),
		"newApplication(address,address,bytes32,bytes32)(address)",
		depInfo.AuthorityAddress,
		CONTRACT_OWNER_ADDRESS,
		hash,
		SALT)
	if err != nil {
		return DeploymentInfo{}, fmt.Errorf("could not create application: %v", err)
	}
	depInfo.ApplicationAddress = contractAddresses[0]

	return depInfo, nil
}

// Call a contract factory, passing a factory function to be executed.
// Returns the resulting contract address(es) and the corresponding
// block number.
//
// Warning: a second call to a contract with the same arguments will fail.
func createContracts(ctx context.Context,
	args ...string) ([]string, error) {
	commonArgs := []string{"--rpc-url", RPC_URL}
	commonArgs = append(commonArgs, args...)

	var contractAddresses []string
	// Calculate the resulting deterministic address(es)
	castCall := exec.CommandContext(ctx,
		"cast",
		"call")
	castCall.Args = append(castCall.Args, commonArgs...)
	var outStrBuilder strings.Builder
	castCall.Stdout = &outStrBuilder
	err := castCall.Run()
	if err != nil {
		return contractAddresses, fmt.Errorf("command failed %v: %v", castCall.Args, err)
	}
	contractAddresses = strings.Fields(outStrBuilder.String())

	// Perform actual transaction on the contract
	castSend := exec.CommandContext(ctx,
		"cast",
		"send",
		"--json",
		"--mnemonic",
		ethutil.FoundryMnemonic)
	castSend.Args = append(castSend.Args, commonArgs...)
	outStrBuilder.Reset()
	castSend.Stdout = &outStrBuilder
	err = castSend.Run()
	if err != nil {
		return contractAddresses, fmt.Errorf("command failed %v: %v", castSend.Args, err)
	}

	if VerboseLog {
		fmt.Printf("deployer: command: %s\n", castSend.Args)
		fmt.Printf("deployer: output: %s\n", outStrBuilder.String())
	}

	return contractAddresses, nil
}

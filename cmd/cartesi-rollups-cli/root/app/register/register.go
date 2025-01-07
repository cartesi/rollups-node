// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package register

import (
	"encoding/json"
	"fmt"
	"os"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/advancer/snapshot"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "register",
	Short:   "Register an existing application on the node",
	Example: examples,
	Run:     run,
}

const examples = `# Adds an application to Rollups Node:
cartesi-rollups-cli app register -n echo-dapp -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF` //nolint:lll

var (
	name                string
	applicationAddress  string
	consensusAddress    string
	templatePath        string
	templateHash        string
	epochLength         uint64
	inputBoxBlockNumber uint64
	rpcURL              string
	disabled            bool
	printAsJSON         bool
)

func init() {
	Cmd.Flags().StringVarP(
		&name,
		"name",
		"n",
		"",
		"Application name",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("name"))

	Cmd.Flags().StringVarP(
		&applicationAddress,
		"address",
		"a",
		"",
		"Application contract address",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("address"))

	Cmd.Flags().StringVarP(
		&consensusAddress,
		"consensus",
		"c",
		"",
		"Application IConsensus Address. If not provided the value will be read from the contract",
	)

	Cmd.Flags().StringVarP(
		&templatePath,
		"template-path",
		"t",
		"",
		"Application template URI",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("template-path"))

	Cmd.Flags().StringVarP(
		&templateHash,
		"template-hash",
		"H",
		"",
		"Application template hash. If not provided, it will be read from the template URI",
	)

	Cmd.Flags().Uint64VarP(
		&inputBoxBlockNumber,
		"inputbox-block-number",
		"i",
		0,
		"InputBox deployment block number",
	)

	Cmd.Flags().Uint64VarP(
		&epochLength,
		"epoch-length",
		"e",
		10,
		"Consensus Epoch length. If not provided the value will be read from the contract",
	)

	Cmd.Flags().BoolVarP(
		&disabled,
		"disabled",
		"d",
		false,
		"Sets the application state to disabled",
	)

	Cmd.Flags().BoolVarP(
		&printAsJSON,
		"print-json",
		"j",
		false,
		"Prints the application data as JSON",
	)

	Cmd.Flags().StringVarP(&rpcURL, "rpc-url", "r", "http://localhost:8545", "Ethereum RPC URL")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Repository == nil {
		panic("Database was not initialized")
	}

	applicationState := model.ApplicationState_Enabled
	if disabled {
		applicationState = model.ApplicationState_Disabled
	}

	if templateHash == "" {
		var err error
		templateHash, err = snapshot.ReadHash(templatePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Read machine template hash failed: %v\n", err)
			os.Exit(1)
		}
	}

	address := common.HexToAddress(applicationAddress)
	var consensus common.Address
	var err error
	if cmd.Flags().Changed("consensus") {
		consensus = common.HexToAddress(consensusAddress)
	} else {
		consensus, err = getConsensus(address)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get consensus address from application: %v\n", err)
			os.Exit(1)
		}

	}

	if !cmd.Flags().Changed("epochLength") {
		epochLength, err = getEpochLength(consensus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get epoch length from consensus: %v\n", err)
			os.Exit(1)
		}
	}

	application := model.Application{
		Name:                 name,
		IApplicationAddress:  address,
		IConsensusAddress:    consensus,
		TemplateURI:          templatePath,
		TemplateHash:         common.HexToHash(templateHash),
		EpochLength:          epochLength,
		State:                applicationState,
		LastProcessedBlock:   inputBoxBlockNumber,
		LastOutputCheckBlock: inputBoxBlockNumber,
		LastClaimCheckBlock:  inputBoxBlockNumber,
	}

	_, err = cmdcommon.Repository.CreateApplication(ctx, &application)
	cobra.CheckErr(err)

	if printAsJSON {
		jsonData, err := json.Marshal(application)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshalling application to JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Printf("Application %v successfully registered\n", application.IApplicationAddress)
	}
}

func getConsensus(
	appAddress common.Address,
) (common.Address, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to connect to the Ethereum client: %v", err)
	}

	app, err := iapplication.NewIApplication(appAddress, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to instantiate contract: %v", err)
	}

	consensus, err := app.GetConsensus(nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("error retrieving application epoch length: %v", err)
	}
	return consensus, nil
}

func getEpochLength(
	consensusAddr common.Address,
) (uint64, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return 0, fmt.Errorf("Failed to connect to the Ethereum client: %v", err)
	}

	consensus, err := iconsensus.NewIConsensus(consensusAddr, client)
	if err != nil {
		return 0, fmt.Errorf("Failed to instantiate contract: %v", err)
	}

	epochLengthRaw, err := consensus.GetEpochLength(nil)
	if err != nil {
		return 0, fmt.Errorf("error retrieving application epoch length: %v", err)
	}
	return epochLengthRaw.Uint64(), nil
}

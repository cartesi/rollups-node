// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package register

import (
	"encoding/json"
	"fmt"
	"os"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "register",
	Short:   "Register an existing application on the node",
	Example: examples,
	Run:     run,
}

const examples = `# Adds an application to Rollups Node:
cartesi-rollups-cli app register -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF -i 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` //nolint:lll

const (
	statusRunning    = "running"
	statusNotRunning = "not-running"
)

var (
	applicationAddress            string
	templatePath                  string
	templateHash                  string
	inputBoxDeploymentBlockNumber uint64
	status                        string
	iConsensusAddress             string
	printAsJSON                   bool
)

func init() {

	Cmd.Flags().StringVarP(
		&applicationAddress,
		"address",
		"a",
		"",
		"Application contract address",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("address"))

	Cmd.Flags().StringVarP(
		&iConsensusAddress,
		"iconsensus",
		"i",
		"",
		"Application IConsensus Address",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("iconsensus"))

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
		&inputBoxDeploymentBlockNumber,
		"inputbox-block-number",
		"n",
		0,
		"InputBox deployment block number",
	)

	Cmd.Flags().StringVarP(
		&status,
		"status",
		"s",
		statusRunning,
		"Sets the application status",
	)

	Cmd.Flags().BoolVarP(
		&printAsJSON,
		"print-json",
		"j",
		false,
		"Prints the application data as JSON",
	)
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Database == nil {
		panic("Database was not initialized")
	}

	var applicationStatus model.ApplicationStatus
	switch status {
	case statusRunning:
		applicationStatus = model.ApplicationStatusRunning
	case statusNotRunning:
		applicationStatus = model.ApplicationStatusNotRunning
	default:
		fmt.Fprintf(os.Stderr, "Invalid application status: %s\n", status)
		os.Exit(1)
	}

	application := model.Application{
		ContractAddress:    common.HexToAddress(applicationAddress),
		TemplateUri:        templatePath,
		TemplateHash:       common.HexToHash(templateHash),
		LastProcessedBlock: inputBoxDeploymentBlockNumber,
		Status:             applicationStatus,
		IConsensusAddress:  common.HexToAddress(iConsensusAddress),
	}

	_, err := cmdcommon.Database.InsertApplication(ctx, &application)
	cobra.CheckErr(err)

	if printAsJSON {
		jsonData, err := json.Marshal(application)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshalling application to JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Printf("Application %v successfully registered\n", application.ContractAddress)
	}
}

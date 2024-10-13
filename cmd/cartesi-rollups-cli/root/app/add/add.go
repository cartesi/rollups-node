// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package add

import (
	"fmt"
	"log/slog"
	"os"

	cmdcommom "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "add",
	Short:   "Add an existing application to the node",
	Example: examples,
	Run:     run,
}

const examples = `# Adds an application to Rollups Node:
cartesi-rollups-cli app add -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF -i 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` //nolint:lll

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
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommom.Database == nil {
		panic("Database was not initialized")
	}

	var applicationStatus model.ApplicationStatus
	switch status {
	case statusRunning:
		applicationStatus = model.ApplicationStatusRunning
	case statusNotRunning:
		applicationStatus = model.ApplicationStatusNotRunning
	default:
		slog.Error("Invalid application status", "status", status)
		os.Exit(1)
	}

	application := model.Application{
		ContractAddress:    common.HexToAddress(applicationAddress),
		TemplateHash:       common.HexToHash(templateHash),
		LastProcessedBlock: inputBoxDeploymentBlockNumber,
		Status:             applicationStatus,
		IConsensusAddress:  common.HexToAddress(iConsensusAddress),
	}

	_, err := cmdcommom.Database.InsertApplication(ctx, &application)
	cobra.CheckErr(err)
	fmt.Printf("Application %v successfully added\n", application.ContractAddress)
}
